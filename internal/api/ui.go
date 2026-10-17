package api

import (
	"encoding/json"
	"io"
	"net/http"

	"terrancc/internal/events"
)

// uiKeys are the frontend state documents tccd stores for the clients:
// boards (dashboard layouts) and windows (which windows are open, where).
// tccd keeps them as opaque JSON; the shape belongs to the frontend.
var uiKeys = map[string]bool{"boards": true, "windows": true}

func (s *server) uiRoutes(mux *http.ServeMux) {
	key := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		k := r.PathValue("key")
		if !uiKeys[k] {
			writeJSON(w, 404, map[string]string{"error": "no ui state called " + k})
			return "", false
		}
		return "ui." + k, true
	}
	mux.HandleFunc("GET /api/ui/{key}", func(w http.ResponseWriter, r *http.Request) {
		k, ok := key(w, r)
		if !ok {
			return
		}
		v, found, err := s.svc.Store.GetSetting(r.Context(), k)
		if err != nil {
			writeErr(w, err)
			return
		}
		if !found {
			v = "null"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, v)
	})
	mux.HandleFunc("PUT /api/ui/{key}", func(w http.ResponseWriter, r *http.Request) {
		k, ok := key(w, r)
		if !ok {
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil || !json.Valid(b) {
			writeErr(w, badRequest("body must be JSON"))
			return
		}
		if err := s.svc.Store.SetSetting(r.Context(), k, string(b)); err != nil {
			writeErr(w, err)
			return
		}
		s.bus.Publish(events.Event{Type: "ui.changed", Data: r.PathValue("key")})
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
