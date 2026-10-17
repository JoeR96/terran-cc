package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terrancc/internal/model"
	"terrancc/internal/store"
	"terrancc/internal/store/sqlite"
)

var ctx = context.Background()

type FixedClock struct{ T time.Time }

func (c *FixedClock) Now() time.Time { return c.T }

func newSvc(t *testing.T) (*Service, *FixedClock) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clk := &FixedClock{T: time.Date(2026, 10, 17, 14, 2, 0, 0, time.Local)}
	return &Service{Store: db, Clock: clk}, clk
}

func ptr[T any](v T) *T { return &v }

func mustProject(t *testing.T, s *Service) model.Project {
	t.Helper()
	p, err := s.AddProject(ctx, "BDTV Bonanza", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddDefaults(t *testing.T) {
	s, _ := newSvc(t)
	todo, err := s.AddItem(ctx, NewItem{Title: "  Book barber  "})
	if err != nil {
		t.Fatal(err)
	}
	if todo.Title != "Book barber" || todo.Kind != model.KindTodo || todo.Status != model.StatusBacklog ||
		todo.When != model.WhenAnytime || todo.Context != model.ContextIRL {
		t.Fatalf("personal defaults: %+v", todo)
	}
	p := mustProject(t, s)
	tk, _ := s.AddItem(ctx, NewItem{Title: "Fix melee", ProjectID: &p.ID})
	if tk.Kind != model.KindTask || tk.Status != model.StatusBacklog || tk.Context != model.ContextWork {
		t.Fatalf("project defaults: %+v", tk)
	}
	in, _ := s.AddItem(ctx, NewItem{Title: "Stairs idea", ProjectID: &p.ID, Inbox: true})
	if in.Status != model.StatusInbox {
		t.Fatalf("inbox: %+v", in)
	}
}

func TestAddValidation(t *testing.T) {
	s, _ := newSvc(t)
	for name, n := range map[string]NewItem{
		"empty title": {Title: "   "},
		"long title":  {Title: strings.Repeat("x", 301)},
		"bad when":    {Title: "x", When: "tomorrow"},
		"bad kind":    {Title: "x", Kind: "epic"},
		"bad status":  {Title: "x", Status: "doing"},
		"bad context": {Title: "x", Context: "home"},
		"bad project": {Title: "x", ProjectID: ptr(int64(99))},
		"bad parent":  {Title: "x", ParentID: ptr(int64(99))},
	} {
		if _, err := s.AddItem(ctx, n); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}

func TestParentMustShareProject(t *testing.T) {
	s, _ := newSvc(t)
	p := mustProject(t, s)
	feat, _ := s.AddItem(ctx, NewItem{Title: "Lobby", ProjectID: &p.ID, Kind: model.KindFeature})
	if _, err := s.AddItem(ctx, NewItem{Title: "sub", ParentID: &feat.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("personal child of project item: %v", err)
	}
	sub, err := s.AddItem(ctx, NewItem{Title: "sub", ProjectID: &p.ID, ParentID: &feat.ID})
	if err != nil || *sub.ParentID != feat.ID {
		t.Fatalf("sub-task: %+v %v", sub, err)
	}
}

// A sub to-do lands next to its parent unless told otherwise.
func TestSubTodoInheritsWhenAndContext(t *testing.T) {
	s, _ := newSvc(t)
	raid, _ := s.AddItem(ctx, NewItem{Title: "Prep for raids S4", When: model.WhenWeek, Context: model.ContextOSRS})
	mace, err := s.AddItem(ctx, NewItem{Title: "Get inquisitor mace", ParentID: &raid.ID})
	if err != nil || mace.When != model.WhenWeek || mace.Context != model.ContextOSRS {
		t.Fatalf("%+v %v", mace, err)
	}
	own, _ := s.AddItem(ctx, NewItem{Title: "today bit", ParentID: &raid.ID, When: model.WhenToday})
	if own.When != model.WhenToday || own.Context != model.ContextOSRS {
		t.Fatalf("explicit when lost: %+v", own)
	}
}

func TestPromoteTodo(t *testing.T) {
	s, _ := newSvc(t)
	p := mustProject(t, s)
	todo, _ := s.AddItem(ctx, NewItem{Title: "Reply to issue"})
	pid := &p.ID
	got, err := s.EditItem(ctx, todo.ID, ItemPatch{ProjectID: &pid})
	if err != nil || got.Kind != model.KindTask || *got.ProjectID != p.ID {
		t.Fatalf("promote: %+v %v", got, err)
	}
	var none *int64
	back, _ := s.EditItem(ctx, todo.ID, ItemPatch{ProjectID: &none})
	if back.ProjectID != nil {
		t.Fatalf("demote: %+v", back)
	}
}

func TestStartReviewDone(t *testing.T) {
	s, clk := newSvc(t)
	p := mustProject(t, s)
	it, _ := s.AddItem(ctx, NewItem{Title: "Module split", ProjectID: &p.ID})
	got, err := s.StartItem(ctx, it.ID, StartInfo{Branch: "feature/module-boundaries", Worktree: `D:\bdb-modules`, Session: "bdb-t1", SessionModel: "claude-opus-5-5"})
	if err != nil || got.Status != model.StatusInProgress || got.Branch != "feature/module-boundaries" ||
		got.SessionStartedAt == nil || !got.SessionStartedAt.Equal(clk.T) {
		t.Fatalf("start: %+v %v", got, err)
	}
	got, _ = s.ReviewItem(ctx, it.ID)
	if got.Status != model.StatusReview {
		t.Fatalf("review: %+v", got)
	}
	got, _ = s.DoneItem(ctx, it.ID)
	if got.Status != model.StatusDone || got.DoneAt == nil {
		t.Fatalf("done: %+v", got)
	}
	if _, err := s.StartItem(ctx, it.ID, StartInfo{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("start from done: %v", err)
	}
	if _, err := s.ReviewItem(ctx, it.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("review from done: %v", err)
	}
	got, _ = s.ReopenItem(ctx, it.ID)
	if got.Status != model.StatusBacklog || got.DoneAt != nil {
		t.Fatalf("reopen: %+v", got)
	}
}

func TestStatusPatchSetsDoneAt(t *testing.T) {
	s, _ := newSvc(t)
	it, _ := s.AddItem(ctx, NewItem{Title: "x"})
	done := model.StatusDone
	got, _ := s.EditItem(ctx, it.ID, ItemPatch{Status: &done})
	if got.DoneAt == nil {
		t.Fatal("done_at not set")
	}
	open := model.StatusBacklog
	got, _ = s.EditItem(ctx, it.ID, ItemPatch{Status: &open})
	if got.DoneAt != nil {
		t.Fatal("done_at not cleared")
	}
}

func TestAppendNote(t *testing.T) {
	s, _ := newSvc(t)
	it, _ := s.AddItem(ctx, NewItem{Title: "x"})
	got, _ := s.AppendNote(ctx, it.ID, "first")
	if got.Notes != "**2026-10-17 14:02** first" {
		t.Fatalf("first note: %q", got.Notes)
	}
	got, _ = s.AppendNote(ctx, it.ID, "second")
	if got.Notes != "**2026-10-17 14:02** first\n\n**2026-10-17 14:02** second" {
		t.Fatalf("second note: %q", got.Notes)
	}
	if _, err := s.AppendNote(ctx, it.ID, "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty note: %v", err)
	}
}

func TestEvidence(t *testing.T) {
	s, clk := newSvc(t)
	it, _ := s.AddItem(ctx, NewItem{Title: "x"})
	if _, err := s.AddEvidence(ctx, it.ID, []model.Evidence{{Type: "video", Value: "v"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad type: %v", err)
	}
	got, err := s.AddEvidence(ctx, it.ID, []model.Evidence{{Type: "commit", Value: "7915ac6"}, {Type: "pr", Value: "#57"}})
	if err != nil || len(got.Evidence) != 2 || !got.Evidence[0].At.Equal(clk.T) {
		t.Fatalf("evidence: %+v %v", got.Evidence, err)
	}
}

func TestDependsOnCycle(t *testing.T) {
	s, _ := newSvc(t)
	a, _ := s.AddItem(ctx, NewItem{Title: "a"})
	b, _ := s.AddItem(ctx, NewItem{Title: "b"})
	c, _ := s.AddItem(ctx, NewItem{Title: "c"})
	if _, err := s.EditItem(ctx, a.ID, ItemPatch{DependsOn: &[]int64{b.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditItem(ctx, b.ID, ItemPatch{DependsOn: &[]int64{c.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EditItem(ctx, c.ID, ItemPatch{DependsOn: &[]int64{a.ID}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle accepted: %v", err)
	}
	if _, err := s.EditItem(ctx, a.ID, ItemPatch{DependsOn: &[]int64{a.ID}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self accepted: %v", err)
	}
	if _, err := s.EditItem(ctx, a.ID, ItemPatch{DependsOn: &[]int64{999}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing accepted: %v", err)
	}
}

func TestDeleteCleansDependsOn(t *testing.T) {
	s, _ := newSvc(t)
	p := mustProject(t, s)
	a, _ := s.AddItem(ctx, NewItem{Title: "a", ProjectID: &p.ID})
	b, _ := s.AddItem(ctx, NewItem{Title: "b", ProjectID: &p.ID})
	child, _ := s.AddItem(ctx, NewItem{Title: "child", ProjectID: &p.ID, ParentID: &b.ID})
	s.EditItem(ctx, a.ID, ItemPatch{DependsOn: &[]int64{b.ID}})
	if err := s.DeleteItem(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Store.GetItem(ctx, a.ID)
	if len(got.DependsOn) != 0 {
		t.Fatalf("depends_on not cleaned: %v", got.DependsOn)
	}
	ch, _ := s.Store.GetItem(ctx, child.ID)
	if ch.ParentID != nil {
		t.Fatalf("child not re-parented: %v", *ch.ParentID)
	}
	if err := s.DeleteItem(ctx, b.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"BDTV Bonanza": "bdtv-bonanza", "  enjoeneer.dev ": "enjoeneer-dev", "OSRS -- tasks!": "osrs-tasks"} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q want %q", in, got, want)
		}
	}
}

func TestResolveProject(t *testing.T) {
	s, _ := newSvc(t)
	p := mustProject(t, s)
	for _, ref := range []string{"bdtv-bonanza", "1"} {
		got, err := s.ResolveProject(ctx, ref)
		if err != nil || got.ID != p.ID {
			t.Errorf("%s: %+v %v", ref, got, err)
		}
	}
	if _, err := s.ResolveProject(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestOnChangeCalled(t *testing.T) {
	s, _ := newSvc(t)
	var got []string
	s.OnChange = func(kind string, id int64) { got = append(got, kind) }
	p := mustProject(t, s)
	it, _ := s.AddItem(ctx, NewItem{Title: "x", ProjectID: &p.ID})
	s.AppendNote(ctx, it.ID, "n")
	if strings.Join(got, ",") != "project,item,item" {
		t.Fatalf("events: %v", got)
	}
}

func TestParentCycle(t *testing.T) {
	s, _ := newSvc(t)
	p := mustProject(t, s)
	a, _ := s.AddItem(ctx, NewItem{Title: "a", ProjectID: &p.ID})
	b, _ := s.AddItem(ctx, NewItem{Title: "b", ProjectID: &p.ID, ParentID: &a.ID})
	pb := &b.ID
	if _, err := s.EditItem(ctx, a.ID, ItemPatch{ParentID: &pb}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("parent cycle accepted: %v", err)
	}
}

func TestAddReturnsEmptySlices(t *testing.T) {
	s, _ := newSvc(t)
	it, _ := s.AddItem(ctx, NewItem{Title: "x"})
	if it.DependsOn == nil || it.Evidence == nil || it.Attachments == nil {
		t.Fatalf("nil slices: %+v", it)
	}
}
