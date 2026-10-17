package api

import (
	"net/http"

	"terrancc/internal/core"
)

func (s *server) projectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		ps, err := s.svc.Store.ListProjects(r.Context(), r.URL.Query().Get("archived") == "1")
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, ps)
	})
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
			RepoPath    string `json:"repo_path"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		p, err := s.svc.AddProject(r.Context(), body.Name, body.Slug, body.Description, body.RepoPath)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("GET /api/projects/{ref}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.svc.ResolveProject(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, projectErr(err, r.PathValue("ref")))
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("PATCH /api/projects/{ref}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.svc.ResolveProject(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, projectErr(err, r.PathValue("ref")))
			return
		}
		var patch core.ProjectPatch
		if err := decode(r, &patch); err != nil {
			writeErr(w, err)
			return
		}
		p, err = s.svc.EditProject(r.Context(), p.ID, patch)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, p)
	})
}
