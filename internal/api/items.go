package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"terrancc/internal/core"
	"terrancc/internal/model"
	"terrancc/internal/store"
)

const maxAttachment = 20 << 20

func itemErr(err error, id int64) error {
	if errors.Is(err, store.ErrNotFound) && !strings.Contains(err.Error(), ":") {
		return fmt.Errorf("%w: item #%d", store.ErrNotFound, id)
	}
	return err
}

func projectErr(err error, ref string) error {
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("%w: project %q", store.ErrNotFound, ref)
	}
	return err
}

// itemPatch is the JSON form of core.ItemPatch. project_id and parent_id use
// RawMessage so an explicit null can clear them; project_id may be a slug.
type itemPatch struct {
	Title     *string         `json:"title"`
	Notes     *string         `json:"notes"`
	WhyFirst  *string         `json:"why_first"`
	Branch    *string         `json:"branch"`
	Worktree  *string         `json:"worktree"`
	Model     *string         `json:"model"`
	Effort    *string         `json:"effort"`
	Kind      *model.Kind     `json:"kind"`
	Status    *model.Status   `json:"status"`
	When      *model.When     `json:"when"`
	Context   *model.Context  `json:"context"`
	Priority  *int            `json:"priority"`
	Position  *int            `json:"position"`
	DependsOn *[]int64        `json:"depends_on"`
	ProjectID json.RawMessage `json:"project_id"`
	ParentID  json.RawMessage `json:"parent_id"`
}

// projectRef turns a JSON number, string (id or slug) or null into a project id.
func (s *server) projectRef(r *http.Request, raw json.RawMessage) (*int64, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, badRequest("project_id must be a number, slug or null")
		}
		ref = strconv.FormatInt(n, 10)
	}
	p, err := s.svc.ResolveProject(r.Context(), ref)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("%w: project %q does not exist", core.ErrInvalid, ref)
		}
		return nil, err
	}
	return &p.ID, nil
}

func (s *server) toPatch(r *http.Request, in itemPatch) (core.ItemPatch, error) {
	p := core.ItemPatch{
		Title: in.Title, Notes: in.Notes, WhyFirst: in.WhyFirst, Branch: in.Branch, Worktree: in.Worktree,
		Model: in.Model, Effort: in.Effort, Kind: in.Kind, Status: in.Status, When: in.When, Context: in.Context,
		Priority: in.Priority, Position: in.Position, DependsOn: in.DependsOn,
	}
	if len(in.ProjectID) > 0 {
		pid, err := s.projectRef(r, in.ProjectID)
		if err != nil {
			return p, err
		}
		p.ProjectID = &pid
	}
	if len(in.ParentID) > 0 {
		var parent *int64
		if err := json.Unmarshal(in.ParentID, &parent); err != nil {
			return p, badRequest("parent_id must be a number or null")
		}
		p.ParentID = &parent
	}
	return p, nil
}

func (s *server) listFilter(r *http.Request) (store.ItemFilter, error) {
	q := r.URL.Query()
	var f store.ItemFilter
	if ref := q.Get("project"); ref != "" {
		p, err := s.svc.ResolveProject(r.Context(), ref)
		if err != nil {
			return f, projectErr(err, ref)
		}
		f.ProjectID = &p.ID
	}
	f.Personal = q.Get("personal") == "1"
	for _, v := range csv(q.Get("status")) {
		f.Status = append(f.Status, model.Status(v))
	}
	for _, v := range csv(q.Get("when")) {
		f.When = append(f.When, model.When(v))
	}
	for _, v := range csv(q.Get("kind")) {
		f.Kind = append(f.Kind, model.Kind(v))
	}
	if c := q.Get("context"); c != "" {
		ctx := model.Context(c)
		f.Context = &ctx
	}
	if p := q.Get("parent"); p != "" {
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return f, badRequest("parent must be a number")
		}
		f.ParentID = &id
	}
	f.Query = q.Get("q")
	return f, nil
}

// withItem parses the id, runs fn and writes the resulting item.
func withItem(fn func(r *http.Request, id int64) (model.Item, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		it, err := fn(r, id)
		if err != nil {
			writeErr(w, itemErr(err, id))
			return
		}
		writeJSON(w, 200, it)
	}
}

func (s *server) itemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		f, err := s.listFilter(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		items, err := s.svc.Store.ListItems(r.Context(), f)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, items)
	})
	mux.HandleFunc("POST /api/items", func(w http.ResponseWriter, r *http.Request) {
		var n core.NewItem
		if err := decode(r, &n); err != nil {
			writeErr(w, err)
			return
		}
		it, err := s.svc.AddItem(r.Context(), n)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, it)
	})
	mux.HandleFunc("GET /api/items/{id}", withItem(func(r *http.Request, id int64) (model.Item, error) {
		return s.svc.Store.GetItem(r.Context(), id)
	}))
	mux.HandleFunc("PATCH /api/items/{id}", withItem(func(r *http.Request, id int64) (model.Item, error) {
		var in itemPatch
		if err := decode(r, &in); err != nil {
			return model.Item{}, err
		}
		p, err := s.toPatch(r, in)
		if err != nil {
			return model.Item{}, err
		}
		return s.svc.EditItem(r.Context(), id, p)
	}))
	mux.HandleFunc("DELETE /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := s.svc.DeleteItem(r.Context(), id); err != nil {
			writeErr(w, itemErr(err, id))
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/items/{id}/start", withItem(func(r *http.Request, id int64) (model.Item, error) {
		var info core.StartInfo
		if err := decode(r, &info); err != nil {
			return model.Item{}, err
		}
		return s.svc.StartItem(r.Context(), id, info)
	}))
	mux.HandleFunc("POST /api/items/{id}/review", withItem(func(r *http.Request, id int64) (model.Item, error) {
		return s.svc.ReviewItem(r.Context(), id)
	}))
	mux.HandleFunc("POST /api/items/{id}/done", withItem(func(r *http.Request, id int64) (model.Item, error) {
		return s.svc.DoneItem(r.Context(), id)
	}))
	mux.HandleFunc("POST /api/items/{id}/reopen", withItem(func(r *http.Request, id int64) (model.Item, error) {
		return s.svc.ReopenItem(r.Context(), id)
	}))
	mux.HandleFunc("POST /api/items/{id}/note", withItem(func(r *http.Request, id int64) (model.Item, error) {
		var body struct {
			Text string `json:"text"`
		}
		if err := decode(r, &body); err != nil {
			return model.Item{}, err
		}
		return s.svc.AppendNote(r.Context(), id, body.Text)
	}))
	mux.HandleFunc("POST /api/items/{id}/evidence", withItem(func(r *http.Request, id int64) (model.Item, error) {
		var body struct {
			Evidence []model.Evidence `json:"evidence"`
		}
		if err := decode(r, &body); err != nil {
			return model.Item{}, err
		}
		return s.svc.AddEvidence(r.Context(), id, body.Evidence)
	}))
	mux.HandleFunc("POST /api/items/{id}/attachments", withItem(s.attach))
}

func (s *server) attach(r *http.Request, id int64) (model.Item, error) {
	if _, err := s.svc.Store.GetItem(r.Context(), id); err != nil {
		return model.Item{}, err
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxAttachment)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return model.Item{}, badRequest("expected a multipart file field named \"file\" (max 20 MB): %v", err)
	}
	defer f.Close()
	// Clients may send Windows paths; keep only the final name.
	name := filepath.Base(strings.ReplaceAll(hdr.Filename, `\`, "/"))
	if name == "." || name == "/" || name == ".." {
		return model.Item{}, badRequest("attachment needs a file name")
	}
	dir := filepath.Join(s.home, "attachments", itoa(id))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return model.Item{}, err
	}
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return model.Item{}, err
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		return model.Item{}, err
	}
	if err := out.Close(); err != nil {
		return model.Item{}, err
	}
	return s.svc.AddAttachment(r.Context(), id, dst)
}
