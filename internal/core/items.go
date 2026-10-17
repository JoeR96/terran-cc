// Package core holds Terran CC's rules. The API, CLI and every other client
// go through it; none of them make decisions of their own.
package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"terrancc/internal/model"
	"terrancc/internal/store"
)

var ErrInvalid = errors.New("invalid")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

type Service struct {
	Store store.Store
	Clock Clock
	// OnChange is called after every successful write with "item", "project" or "habit".
	OnChange func(kind string, id int64)
}

func (s *Service) changed(kind string, id int64) {
	if s.OnChange != nil {
		s.OnChange(kind, id)
	}
}

// --- projects ---

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func (s *Service) AddProject(ctx context.Context, name, slug, desc, repo string) (model.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Project{}, invalid("project name is required")
	}
	if slug == "" {
		slug = name
	}
	slug = Slugify(slug)
	if slug == "" || isDigits(slug) {
		return model.Project{}, invalid("project slug must contain a letter")
	}
	p := model.Project{Slug: slug, Name: name, Description: desc, RepoPath: repo, CreatedAt: s.Clock.Now()}
	if err := s.Store.CreateProject(ctx, &p); err != nil {
		return p, err
	}
	s.changed("project", p.ID)
	return p, nil
}

type ProjectPatch struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	RepoPath    *string `json:"repo_path,omitempty"`
	Archived    *bool   `json:"archived,omitempty"`
}

func (s *Service) EditProject(ctx context.Context, id int64, pp ProjectPatch) (model.Project, error) {
	p, err := s.Store.GetProject(ctx, id)
	if err != nil {
		return p, err
	}
	if pp.Name != nil {
		if strings.TrimSpace(*pp.Name) == "" {
			return p, invalid("project name is required")
		}
		p.Name = strings.TrimSpace(*pp.Name)
	}
	if pp.Slug != nil {
		p.Slug = Slugify(*pp.Slug)
		if p.Slug == "" || isDigits(p.Slug) {
			return p, invalid("project slug must contain a letter")
		}
	}
	if pp.Description != nil {
		p.Description = *pp.Description
	}
	if pp.RepoPath != nil {
		p.RepoPath = *pp.RepoPath
	}
	if pp.Archived != nil {
		p.Archived = *pp.Archived
	}
	if err := s.Store.UpdateProject(ctx, p); err != nil {
		return p, err
	}
	s.changed("project", p.ID)
	return p, nil
}

func isDigits(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// ResolveProject accepts a numeric id or a slug.
func (s *Service) ResolveProject(ctx context.Context, ref string) (model.Project, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return s.Store.GetProject(ctx, id)
	}
	return s.Store.GetProjectBySlug(ctx, Slugify(ref))
}

// --- items ---

type NewItem struct {
	Title     string        `json:"title"`
	Notes     string        `json:"notes,omitempty"`
	Kind      model.Kind    `json:"kind,omitempty"`
	ProjectID *int64        `json:"project_id,omitempty"`
	ParentID  *int64        `json:"parent_id,omitempty"`
	Status    model.Status  `json:"status,omitempty"`
	When      model.When    `json:"when,omitempty"`
	Context   model.Context `json:"context,omitempty"`
	Priority  int           `json:"priority,omitempty"`
	Inbox     bool          `json:"inbox,omitempty"`
}

func cleanTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", invalid("title is required")
	}
	if len([]rune(t)) > 300 {
		return "", invalid("title is longer than 300 characters")
	}
	return t, nil
}

func validateEnums(it *model.Item) error {
	if !it.Kind.Valid() {
		return invalid("kind must be todo|bug|task|feature|idea")
	}
	if !it.Status.Valid() {
		return invalid("status must be inbox|backlog|up_next|in_progress|review|done|someday")
	}
	if !it.When.Valid() {
		return invalid("when must be today|week|month|anytime")
	}
	if !it.Context.Valid() {
		return invalid("context must be work|irl|osrs")
	}
	return nil
}

func (s *Service) checkRefs(ctx context.Context, st store.Store, it *model.Item) error {
	if it.ProjectID != nil {
		if _, err := st.GetProject(ctx, *it.ProjectID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return invalid("project %d does not exist", *it.ProjectID)
			}
			return err
		}
	}
	if it.ParentID != nil {
		if *it.ParentID == it.ID {
			return invalid("an item cannot be its own parent")
		}
		parent, err := st.GetItem(ctx, *it.ParentID)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("parent #%d does not exist", *it.ParentID)
		} else if err != nil {
			return err
		}
		if !sameProject(parent.ProjectID, it.ProjectID) {
			return invalid("a sub-task must be in the same project as its parent")
		}
		// Walk up from the new parent; reaching this item again would loop.
		for seen, p := map[int64]bool{}, parent; p.ParentID != nil && it.ID != 0; {
			if *p.ParentID == it.ID {
				return invalid("#%d is already a sub-task of #%d", *it.ParentID, it.ID)
			}
			if seen[p.ID] {
				break
			}
			seen[p.ID] = true
			if p, err = st.GetItem(ctx, *p.ParentID); err != nil {
				break
			}
		}
	}
	return nil
}

func sameProject(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Service) AddItem(ctx context.Context, n NewItem) (model.Item, error) {
	title, err := cleanTitle(n.Title)
	if err != nil {
		return model.Item{}, err
	}
	now := s.Clock.Now()
	it := model.Item{
		Title: title, Notes: n.Notes, Kind: n.Kind, ProjectID: n.ProjectID, ParentID: n.ParentID,
		Status: n.Status, When: n.When, Context: n.Context, Priority: n.Priority, CreatedAt: now, UpdatedAt: now,
		DependsOn: []int64{}, Evidence: []model.Evidence{}, Attachments: []string{},
	}
	// A personal sub to-do goes in its parent's section unless told otherwise.
	if n.ParentID != nil && n.ProjectID == nil {
		if parent, err := s.Store.GetItem(ctx, *n.ParentID); err == nil && parent.ProjectID == nil {
			if it.When == "" {
				it.When = parent.When
			}
			if it.Context == "" {
				it.Context = parent.Context
			}
		}
	}
	if it.Kind == "" {
		it.Kind = model.KindTodo
		if it.ProjectID != nil {
			it.Kind = model.KindTask
		}
	}
	if it.Status == "" {
		it.Status = model.StatusBacklog
		if n.Inbox {
			it.Status = model.StatusInbox
		}
	}
	if it.When == "" {
		it.When = model.WhenAnytime
	}
	if it.Context == "" {
		it.Context = model.ContextIRL
		if it.ProjectID != nil {
			it.Context = model.ContextWork
		}
	}
	if it.Status == model.StatusDone {
		it.DoneAt = &now
	}
	if err := validateEnums(&it); err != nil {
		return it, err
	}
	if err := s.checkRefs(ctx, s.Store, &it); err != nil {
		return it, err
	}
	if err := s.Store.CreateItem(ctx, &it); err != nil {
		return it, err
	}
	s.changed("item", it.ID)
	return it, nil
}

// ItemPatch changes only the fields that are non-nil. ProjectID and ParentID
// are double pointers so a patch can set them to nil.
type ItemPatch struct {
	Title, Notes, WhyFirst, Branch, Worktree, Model, Effort *string
	Kind                                                    *model.Kind
	Status                                                  *model.Status
	When                                                    *model.When
	Context                                                 *model.Context
	Priority, Position                                      *int
	ProjectID, ParentID                                     **int64
	DependsOn                                               *[]int64
}

// update loads an item, applies fn, validates and saves it in one transaction.
func (s *Service) update(ctx context.Context, id int64, fn func(st store.Store, it *model.Item) error) (model.Item, error) {
	var out model.Item
	err := s.Store.WithTx(ctx, func(st store.Store) error {
		it, err := st.GetItem(ctx, id)
		if err != nil {
			return err
		}
		if err := fn(st, &it); err != nil {
			return err
		}
		if err := validateEnums(&it); err != nil {
			return err
		}
		it.UpdatedAt = s.Clock.Now()
		if err := st.UpdateItem(ctx, it); err != nil {
			return err
		}
		out = it
		return nil
	})
	if err != nil {
		return out, err
	}
	s.changed("item", out.ID)
	return out, nil
}

func (s *Service) setStatus(it *model.Item, st model.Status) {
	if st == model.StatusDone && it.Status != model.StatusDone {
		now := s.Clock.Now()
		it.DoneAt = &now
	}
	if st != model.StatusDone {
		it.DoneAt = nil
	}
	it.Status = st
}

func (s *Service) EditItem(ctx context.Context, id int64, p ItemPatch) (model.Item, error) {
	return s.update(ctx, id, func(st store.Store, it *model.Item) error {
		if p.Title != nil {
			t, err := cleanTitle(*p.Title)
			if err != nil {
				return err
			}
			it.Title = t
		}
		for dst, src := range map[*string]*string{&it.Notes: p.Notes, &it.WhyFirst: p.WhyFirst, &it.Branch: p.Branch,
			&it.Worktree: p.Worktree, &it.Model: p.Model, &it.Effort: p.Effort} {
			if src != nil {
				*dst = *src
			}
		}
		if p.Kind != nil {
			it.Kind = *p.Kind
		}
		if p.When != nil {
			it.When = *p.When
		}
		if p.Context != nil {
			it.Context = *p.Context
		}
		if p.Priority != nil {
			it.Priority = *p.Priority
		}
		if p.Position != nil {
			it.Position = *p.Position
		}
		if p.Status != nil {
			if !p.Status.Valid() {
				return invalid("status must be inbox|backlog|up_next|in_progress|review|done|someday")
			}
			s.setStatus(it, *p.Status)
		}
		if p.ProjectID != nil {
			if it.ProjectID == nil && *p.ProjectID != nil && it.Kind == model.KindTodo {
				it.Kind = model.KindTask // promoting a to-do makes it a ticket
			}
			if !sameProject(it.ProjectID, *p.ProjectID) && p.ParentID == nil {
				it.ParentID = nil // a sub-task can't follow its parent into another project
			}
			it.ProjectID = *p.ProjectID
		}
		if p.ParentID != nil {
			it.ParentID = *p.ParentID
		}
		if p.ProjectID != nil || p.ParentID != nil {
			if err := s.checkRefs(ctx, st, it); err != nil {
				return err
			}
		}
		if p.DependsOn != nil {
			deps := slices.Compact(slices.Sorted(slices.Values(*p.DependsOn)))
			if err := checkDeps(ctx, st, it.ID, deps); err != nil {
				return err
			}
			it.DependsOn = deps
		}
		return nil
	})
}

// checkDeps rejects missing ids, self-dependency and cycles.
func checkDeps(ctx context.Context, st store.Store, id int64, deps []int64) error {
	seen := map[int64]bool{}
	var walk func(n int64) error
	walk = func(n int64) error {
		if n == id {
			return invalid("dependency on #%d would create a cycle", n)
		}
		if seen[n] {
			return nil
		}
		seen[n] = true
		it, err := st.GetItem(ctx, n)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("#%d does not exist", n)
		} else if err != nil {
			return err
		}
		for _, d := range it.DependsOn {
			if err := walk(d); err != nil {
				return err
			}
		}
		return nil
	}
	for _, d := range deps {
		if d == id {
			return invalid("an item cannot depend on itself")
		}
		if err := walk(d); err != nil {
			return err
		}
	}
	return nil
}

type StartInfo struct {
	Branch       string `json:"branch,omitempty"`
	Worktree     string `json:"worktree,omitempty"`
	Session      string `json:"session,omitempty"`
	SessionModel string `json:"session_model,omitempty"`
}

func (s *Service) StartItem(ctx context.Context, id int64, info StartInfo) (model.Item, error) {
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		if it.Status == model.StatusDone {
			return invalid("#%d is done; reopen it first", it.ID)
		}
		s.setStatus(it, model.StatusInProgress)
		if info.Branch != "" {
			it.Branch = info.Branch
		}
		if info.Worktree != "" {
			it.Worktree = info.Worktree
		}
		if info.SessionModel != "" {
			it.SessionModel = info.SessionModel
		}
		if info.Session != "" {
			it.Session = info.Session
			now := s.Clock.Now()
			it.SessionStartedAt = &now
		}
		return nil
	})
}

func (s *Service) ReviewItem(ctx context.Context, id int64) (model.Item, error) {
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		if it.Status == model.StatusDone {
			return invalid("#%d is already done", it.ID)
		}
		s.setStatus(it, model.StatusReview)
		return nil
	})
}

func (s *Service) DoneItem(ctx context.Context, id int64) (model.Item, error) {
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		s.setStatus(it, model.StatusDone)
		return nil
	})
}

func (s *Service) ReopenItem(ctx context.Context, id int64) (model.Item, error) {
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		if it.Status == model.StatusDone {
			s.setStatus(it, model.StatusBacklog)
		}
		return nil
	})
}

func (s *Service) AppendNote(ctx context.Context, id int64, text string) (model.Item, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return model.Item{}, invalid("note is empty")
	}
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		entry := fmt.Sprintf("**%s** %s", s.Clock.Now().Format("2006-01-02 15:04"), text)
		if strings.TrimSpace(it.Notes) == "" {
			it.Notes = entry
		} else {
			it.Notes = strings.TrimRight(it.Notes, "\n") + "\n\n" + entry
		}
		return nil
	})
}

func (s *Service) AddEvidence(ctx context.Context, id int64, ev []model.Evidence) (model.Item, error) {
	if len(ev) == 0 {
		return model.Item{}, invalid("no evidence given")
	}
	for i := range ev {
		if !ev[i].Valid() {
			return model.Item{}, invalid("evidence type must be commit|pr|screenshot|test|note")
		}
		if strings.TrimSpace(ev[i].Value) == "" {
			return model.Item{}, invalid("evidence value is empty")
		}
		if ev[i].At.IsZero() {
			ev[i].At = s.Clock.Now()
		}
	}
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		it.Evidence = append(it.Evidence, ev...)
		return nil
	})
}

func (s *Service) AddAttachment(ctx context.Context, id int64, path string) (model.Item, error) {
	return s.update(ctx, id, func(_ store.Store, it *model.Item) error {
		if !slices.Contains(it.Attachments, path) {
			it.Attachments = append(it.Attachments, path)
		}
		return nil
	})
}

// DeleteItem removes an item, drops it from other items' depends_on and
// detaches its sub-tasks.
func (s *Service) DeleteItem(ctx context.Context, id int64) error {
	err := s.Store.WithTx(ctx, func(st store.Store) error {
		if _, err := st.GetItem(ctx, id); err != nil {
			return err
		}
		all, err := st.ListItems(ctx, store.ItemFilter{})
		if err != nil {
			return err
		}
		for _, it := range all {
			dirty := false
			if i := slices.Index(it.DependsOn, id); i >= 0 {
				it.DependsOn = slices.Delete(it.DependsOn, i, i+1)
				dirty = true
			}
			if it.ParentID != nil && *it.ParentID == id {
				it.ParentID = nil
				dirty = true
			}
			if dirty {
				it.UpdatedAt = s.Clock.Now()
				if err := st.UpdateItem(ctx, it); err != nil {
					return err
				}
			}
		}
		return st.DeleteItem(ctx, id)
	})
	if err == nil {
		s.changed("item", id)
	}
	return err
}
