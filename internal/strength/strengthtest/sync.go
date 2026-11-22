package strengthtest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The live-sync contract: GET /sync/version, GET /sync/events (Server-Sent
// Events), and ETag / If-None-Match on the GETs Terran CC polls.

// Areas are the names /sync/version and "changed" events use.
var Areas = []string{"habits", "plan", "workout", "cardio", "mobility"}

type syncState struct {
	version   int64
	areas     map[string]int64
	subs      map[chan struct{}]struct{}
	noStream  bool // /sync/events answers 404
	noVersion bool // /sync/version answers 404 too
	keepalive time.Duration
}

func (f *Fake) syncLocked() *syncState {
	if f.sync == nil {
		f.sync = &syncState{areas: map[string]int64{}, subs: map[chan struct{}]struct{}{}, keepalive: 25 * time.Second}
		for _, a := range Areas {
			f.sync.areas[a] = 0
		}
	}
	return f.sync
}

// Changed bumps the version for areas and tells every open stream.
func (f *Fake) Changed(areas ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changedLocked(areas...)
}

func (f *Fake) changedLocked(areas ...string) {
	st := f.syncLocked()
	st.version++
	for _, a := range areas {
		st.areas[a] = st.version
	}
	for ch := range st.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Version is the current sync version.
func (f *Fake) Version() int64 { f.mu.Lock(); defer f.mu.Unlock(); return f.syncLocked().version }

// NoStream makes /sync/events answer 404, the way a 99-Strength from before
// live sync does; with noVersion, /sync/version too.
func (f *Fake) NoStream(noStream, noVersion bool) {
	f.mu.Lock()
	st := f.syncLocked()
	st.noStream, st.noVersion = noStream, noVersion
	f.mu.Unlock()
}

// Keepalive sets how often an open stream sends a ":keepalive" comment.
func (f *Fake) Keepalive(d time.Duration) { f.mu.Lock(); f.syncLocked().keepalive = d; f.mu.Unlock() }

// DropStreams ends every open event stream, as a deploy or a proxy would.
func (f *Fake) DropStreams() {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.syncLocked()
	for ch := range st.subs {
		close(ch)
		delete(st.subs, ch)
	}
}

// Streams is how many event streams are open.
func (f *Fake) Streams() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.syncLocked().subs) }

// okTagged answers v with an ETag over its bytes, or 304 when the request's
// If-None-Match already names it.
func okTagged(w http.ResponseWriter, r *http.Request, v any) {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	tag := `"` + hex.EncodeToString(sum[:8]) + `"`
	writeTagged(w, r, tag, b)
}

func writeTagged(w http.ResponseWriter, r *http.Request, tag string, b []byte) {
	w.Header().Set("ETag", tag)
	if match(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(append(b, '\n'))
}

func match(header, tag string) bool {
	for _, t := range strings.Split(header, ",") {
		if t = strings.TrimSpace(t); t == tag || t == "*" {
			return true
		}
	}
	return false
}

// serveVersion is GET /sync/version, under f.mu.
func (f *Fake) serveVersion(w http.ResponseWriter, r *http.Request) {
	st := f.syncLocked()
	if st.noVersion {
		problem(w, 404, "Not Found", "")
		return
	}
	b, _ := json.Marshal(map[string]any{"version": st.version, "areas": st.areas})
	writeTagged(w, r, `"`+strconv.FormatInt(st.version, 10)+`"`, b)
}

// serveEvents is GET /sync/events. It runs without f.mu held, for as long
// as the client stays.
func (f *Fake) serveEvents(w http.ResponseWriter, r *http.Request) int {
	f.mu.Lock()
	st := f.syncLocked()
	switch {
	case f.down != 0:
		f.mu.Unlock()
		problem(w, f.down, "Service Unavailable", "The fake is down.")
		return f.down
	case f.token == "" || r.Header.Get("Authorization") != "Bearer "+f.token:
		f.mu.Unlock()
		problem(w, 401, "Unauthenticated", "The API token is unknown or revoked.")
		return 401
	case st.noStream:
		f.mu.Unlock()
		problem(w, 404, "Not Found", "")
		return 404
	}
	ch := make(chan struct{}, 1)
	st.subs[ch] = struct{}{}
	sent := st.version
	if id, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil {
		sent = id // catch up from there
	}
	keep := st.keepalive
	hello := st.version
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		if _, open := st.subs[ch]; open {
			delete(st.subs, ch)
		}
		f.mu.Unlock()
	}()

	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	fmt.Fprintf(w, "event: hello\ndata: {\"version\":%d}\n\n", hello)
	// changes since Last-Event-ID, as one event
	flushChanges := func() {
		f.mu.Lock()
		var areas []string
		for _, a := range Areas {
			if st.areas[a] > sent {
				areas = append(areas, a)
			}
		}
		v := st.version
		f.mu.Unlock()
		if v > sent && len(areas) > 0 {
			b, _ := json.Marshal(map[string]any{"version": v, "areas": areas})
			fmt.Fprintf(w, "id: %d\nevent: changed\ndata: %s\n\n", v, b)
		}
		sent = max(sent, v)
	}
	flushChanges()
	if fl != nil {
		fl.Flush()
	}
	tick := time.NewTicker(keep)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return 200
		case _, open := <-ch:
			if !open {
				return 200 // DropStreams
			}
			flushChanges()
		case <-tick.C:
			fmt.Fprint(w, ":keepalive\n\n")
		}
		if fl != nil {
			fl.Flush()
		}
	}
}
