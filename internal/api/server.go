// Package api exposes the core service over HTTP JSON with a live event stream.
// Handlers translate HTTP to core calls and back; they hold no rules.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/store"
	"terrancc/internal/timer"
)

const Version = "0.1.0"

// TimerConfigKey is the settings key the timer config is persisted under.
const TimerConfigKey = "timer.config"

type server struct {
	svc   *core.Service
	timer *timer.Timer
	bus   *events.Bus
	token string
	home  string
}

// New wires svc.OnChange to the bus (when unset) and returns the API handler.
// home is where attachments are stored.
func New(svc *core.Service, t *timer.Timer, bus *events.Bus, token, home string) http.Handler {
	s := &server{svc: svc, timer: t, bus: bus, token: token, home: home}
	if svc.OnChange == nil {
		svc.OnChange = func(kind string, id int64) { bus.Publish(events.Event{Type: kind + ".changed", ID: id}) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "version": Version})
	})
	s.projectRoutes(mux)
	s.itemRoutes(mux)
	s.habitRoutes(mux)
	s.timerRoutes(mux)
	mux.HandleFunc("GET /api/events", s.events)
	return s.auth(mux)
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeJSON(w, 401, map[string]string{"error": "missing or wrong token"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RunTicker advances the timer every second, publishing timer.tick while it
// runs and timer.phase when a phase ends. It returns when ctx is done.
func RunTicker(ctx context.Context, t *timer.Timer, bus *events.Bus) {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			snap, ended := t.Advance()
			if ended {
				bus.Publish(events.Event{Type: "timer.phase", Data: snap})
			} else if snap.State == timer.Running {
				bus.Publish(events.Event{Type: "timer.tick", Data: snap})
			}
		}
	}
}

// --- helpers ---

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

var errBadRequest = errors.New("bad request")

func badRequest(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errBadRequest, fmt.Sprintf(format, a...))
}

func writeErr(w http.ResponseWriter, err error) {
	code := 500
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = 404
	case errors.Is(err, core.ErrInvalid), errors.Is(err, errBadRequest):
		code = 400
	case errors.Is(err, store.ErrConflict):
		code = 409
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// decode reads a JSON object body. An empty body leaves v untouched.
func decode(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("body must be a JSON object: %v", err)
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.PathValue("id"), "#"), 10, 64)
	if err != nil {
		return 0, badRequest("id must be a number")
	}
	return id, nil
}

func csv(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}
