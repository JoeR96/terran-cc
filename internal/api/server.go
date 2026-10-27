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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"terrancc/internal/assist"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/osrs"
	"terrancc/internal/store"
	"terrancc/internal/timer"
)

// Version is a var so tests can build an "older" tccd with -ldflags -X.
var Version = "0.13.0"

// TimerConfigKey is the settings key the timer config is persisted under.
const TimerConfigKey = "timer.config"

type server struct {
	svc       *core.Service
	timer     *timer.Timer
	bus       *events.Bus
	token     string
	home      string
	shutdown  func()
	assistant *assist.Client
	osrs      *osrs.Service
	wiki      string
	vault     string
	osrsSync  sync.Mutex
}

type Option func(*server)

// WithShutdown enables POST /api/shutdown, which calls fn.
func WithShutdown(fn func()) Option { return func(s *server) { s.shutdown = fn } }

// New wires svc.OnChange to the bus (when unset) and returns the API handler.
// home is where attachments are stored.
func New(svc *core.Service, t *timer.Timer, bus *events.Bus, token, home string, opts ...Option) http.Handler {
	s := &server{svc: svc, timer: t, bus: bus, token: token, home: home}
	for _, o := range opts {
		o(s)
	}
	if svc.OnChange == nil {
		svc.OnChange = func(kind string, id int64) { bus.Publish(events.Event{Type: kind + ".changed", ID: id}) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		// home and pid let a client or a second tccd tell whose service this is.
		writeJSON(w, 200, map[string]any{"ok": true, "version": Version, "home": home, "pid": os.Getpid()})
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if s.shutdown == nil {
			writeJSON(w, 404, map[string]string{"error": "shutdown is not enabled"})
			return
		}
		writeJSON(w, 202, map[string]bool{"stopping": true})
		go s.shutdown()
	})
	s.projectRoutes(mux)
	s.itemRoutes(mux)
	s.habitRoutes(mux)
	s.timerRoutes(mux)
	s.uiRoutes(mux)
	s.assistRoutes(mux)
	s.osrsRoutes(mux)
	s.vaultRoutes(mux)
	mux.HandleFunc("GET /api/events", s.events)
	return cors(s.auth(mux))
}

// cors lets the desktop and browser frontends call the API from their own
// origin. Every call except health needs the bearer token and no cookies are
// used, so allowing any origin grants nothing without the token.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			// A browser EventSource or <img> can't set headers, so the event
			// stream and attachment downloads also take the token from the query.
			if got == "" && r.Method == http.MethodGet && queryTokenPath(r.URL.Path) {
				got = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeJSON(w, 401, map[string]string{"error": "missing or wrong token"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func queryTokenPath(p string) bool {
	return p == "/api/events" || p == "/api/vault/file" || (strings.HasPrefix(p, "/api/items/") && strings.Contains(p, "/attachments/")) ||
		strings.HasPrefix(p, "/api/osrs/icon/")
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
