package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"terrancc/internal/strength"
)

// strengthTimeout bounds every handler that may wait on 99-Strength, well
// inside the 10 s the tcc client allows.
const strengthTimeout = 6 * time.Second

// WithStrength lets habits come from 99-Strength once it is connected.
// Without it (or while not connected) habits are the local ones.
func WithStrength(svc *strength.Service) Option { return func(s *server) { s.strength = svc } }

// remoteHabits reports whether 99-Strength is the habit source right now.
func (s *server) remoteHabits() bool { return s.strength != nil && s.strength.Connected() }

var errNoStrength = errors.New("this tccd was started without 99-Strength support")

// strengthRoutes manage the 99-Strength connection. The token arrives in
// the connect body and goes straight to the token store; no answer, event
// or log line ever carries it.
func (s *server) strengthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/strength", func(w http.ResponseWriter, r *http.Request) {
		if s.strength == nil {
			writeJSON(w, 200, strength.Status{Sync: strength.Sync{State: "disconnected"}})
			return
		}
		writeJSON(w, 200, s.strength.Status())
	})
	mux.HandleFunc("POST /api/strength/connect", func(w http.ResponseWriter, r *http.Request) {
		if s.strength == nil {
			writeErr(w, errNoStrength)
			return
		}
		var body struct {
			BaseURL string `json:"base_url"`
			Token   string `json:"token"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), strengthTimeout)
		defer cancel()
		st, err := s.strength.Connect(ctx, body.BaseURL, body.Token)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, st)
	})
	mux.HandleFunc("POST /api/strength/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if s.strength == nil {
			writeErr(w, errNoStrength)
			return
		}
		dropped, err := s.strength.Disconnect()
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"connected": false, "dropped": dropped})
	})
	// Refresh on demand: replay anything queued, then read today again.
	mux.HandleFunc("POST /api/strength/sync", func(w http.ResponseWriter, r *http.Request) {
		if !s.remoteHabits() {
			writeErr(w, strength.ErrNotConnected)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), strengthTimeout)
		defer cancel()
		writeJSON(w, 200, s.strength.Sync(ctx))
	})
}
