package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/model"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/strength"
	"terrancc/internal/strength/strengthtest"
	"terrancc/internal/timer"
)

// strengthEnv is tccd with a real SQLite store, wired to a fake 99-Strength.
type strengthEnv struct {
	t      *testing.T
	srv    *httptest.Server
	fake   *strengthtest.Fake
	remote *httptest.Server
	tokens *strength.MemoryTokens
	home   string
	bus    *events.Bus

	mu     sync.Mutex
	seen   []string // every response body and every event, as JSON
	events <-chan events.Event
}

func newStrengthEnv(t *testing.T) *strengthEnv {
	t.Helper()
	e := &strengthEnv{t: t, home: t.TempDir(), fake: strengthtest.New(strengthtest.Token), tokens: &strength.MemoryTokens{}, bus: events.NewBus()}
	e.remote = httptest.NewServer(e.fake)
	t.Cleanup(e.remote.Close)
	db, err := sqlite.Open(filepath.Join(e.home, "tcc.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	if err := svc.SeedHabits(context.Background()); err != nil {
		t.Fatal(err)
	}
	remote, err := strength.New(strength.Options{
		Store: db, Tokens: e.tokens, LogWait: 2 * time.Second,
		Notify: func() { e.bus.Publish(events.Event{Type: "habit.changed"}) },
		HTTP:   &http.Client{Timeout: time.Second},
		Logf:   func(format string, a ...any) { e.keep(format, a) },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { remote.Run(ctx); close(done) }()
	e.srv = httptest.NewServer(New(svc, timer.New(timer.DefaultConfig(), time.Now), e.bus, tok, e.home, WithStrength(remote)))
	ch, stop := e.bus.Subscribe()
	e.events = ch
	t.Cleanup(func() {
		stop()
		e.srv.Close()
		cancel()
		<-done
		db.Close()
	})
	return e
}

func (e *strengthEnv) keep(v ...any) {
	b, _ := json.Marshal(v)
	e.mu.Lock()
	e.seen = append(e.seen, string(b))
	e.mu.Unlock()
}

func (e *strengthEnv) call(method, path string, body, out any) int {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	e.keep(string(raw))
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s answered %s", method, path, raw)
		}
	}
	return resp.StatusCode
}

func (e *strengthEnv) connect() {
	e.t.Helper()
	var st strength.Status
	body := map[string]string{"base_url": e.remote.URL + "/api/v1", "token": strengthtest.Token}
	if c := e.call("POST", "/api/strength/connect", body, &st); c != 200 || !st.Connected || st.State != "ok" {
		e.t.Fatalf("connect: %d %+v", c, st)
	}
}

// drain collects the events published so far.
func (e *strengthEnv) drain() []events.Event {
	var out []events.Event
	for {
		select {
		case ev := <-e.events:
			e.keep(ev)
			out = append(out, ev)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func (e *strengthEnv) today() string {
	return time.Now().In(mustLondon(e.t)).Format("2006-01-02")
}

func mustLondon(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

type habitsBody struct {
	Date       string               `json:"date"`
	Streak     int                  `json:"streak"`
	Habits     []model.HabitDay     `json:"habits"`
	Source     string               `json:"source"`
	Sync       *strength.Sync       `json:"sync"`
	StreakInfo *strength.StreakInfo `json:"streak_info"`
}

func TestHabitsStayLocalUntilConnected(t *testing.T) {
	e := newStrengthEnv(t)
	var st strength.Status
	if c := e.call("GET", "/api/strength", nil, &st); c != 200 || st.Connected || st.State != "disconnected" {
		t.Fatalf("status: %d %+v", c, st)
	}
	var d model.HabitDay
	if c := e.call("POST", "/api/habits/water/log", map[string]any{}, &d); c != 200 || d.Value != 250 || d.Source != "local" || d.Kind != "" || d.RemoteID != "" {
		t.Fatalf("local log: %d %+v", c, d)
	}
	var day habitsBody
	e.call("GET", "/api/habits", nil, &day)
	if day.Source != "local" || len(day.Habits) != 5 || day.Habits[1].Name != "Water" || day.Habits[1].Value != 250 || day.Sync != nil || day.StreakInfo != nil {
		t.Fatalf("local habits: %+v", day)
	}
	if c := e.call("POST", "/api/strength/sync", nil, nil); c != 409 {
		t.Fatalf("sync while not connected: %d", c)
	}
	if len(e.fake.Requests()) != 0 {
		t.Fatal("99-Strength was called before anything was connected")
	}

	// The local JSON keeps exactly the fields it had, plus "source".
	var raw struct {
		Habits []map[string]any `json:"habits"`
	}
	e.call("GET", "/api/habits", nil, &raw)
	var keys []string
	for k := range raw.Habits[0] {
		keys = append(keys, k)
	}
	if len(keys) != 14 {
		t.Fatalf("local habit JSON changed shape: %v", keys)
	}
	for _, k := range []string{"kind", "minimum", "min_hit", "remote_id", "pending"} {
		if _, ok := raw.Habits[0][k]; ok {
			t.Errorf("a local habit has %q", k)
		}
	}
}

func TestConnectRefusesABadToken(t *testing.T) {
	e := newStrengthEnv(t)
	var m map[string]string
	wrong := "nns_pat_" + strings.Repeat("z", 43)
	if c := e.call("POST", "/api/strength/connect", map[string]string{"base_url": e.remote.URL + "/api/v1", "token": wrong}, &m); c != 400 || !strings.Contains(m["error"], "did not accept") {
		t.Fatalf("refused token: %d %v", c, m)
	}
	if c := e.call("POST", "/api/strength/connect", map[string]string{"base_url": e.remote.URL + "/api/v1", "token": "nope"}, &m); c != 400 {
		t.Fatalf("malformed token: %d %v", c, m)
	}
	if c := e.call("POST", "/api/strength/connect", map[string]string{"base_url": "http://127.0.0.1:1/api/v1", "token": strengthtest.Token}, &m); c != 502 {
		t.Fatalf("nothing listening: %d %v", c, m)
	}
	if _, err := e.tokens.Get(); err == nil {
		t.Fatal("a token was stored")
	}
	var day habitsBody
	e.call("GET", "/api/habits", nil, &day)
	if day.Source != "local" {
		t.Fatalf("still local: %+v", day)
	}
	for _, s := range e.seen {
		if strings.Contains(s, wrong) {
			t.Fatalf("the refused token was echoed: %s", s)
		}
	}
}

func TestConnectedHabitsOverHTTP(t *testing.T) {
	e := newStrengthEnv(t)
	var local model.HabitDay
	e.call("POST", "/api/habits/water/log", map[string]any{"delta": 500}, &local) // local water: 500 ml
	e.connect()
	e.drain()

	var day habitsBody
	if c := e.call("GET", "/api/habits", nil, &day); c != 200 {
		t.Fatal(c)
	}
	if day.Source != "99-strength" || day.Date != e.today() || day.Streak != 12 || len(day.Habits) != 5 || day.Sync == nil || day.Sync.State != "ok" || day.StreakInfo == nil || day.StreakInfo.Best != 19 {
		t.Fatalf("remote habits: %+v", day)
	}
	water := day.Habits[0]
	if water.Name != "Water (532ml bottle)" || water.Type != model.HabitCount || water.Kind != "counter" || water.Unit != "bottles" ||
		water.Target != 6 || water.Minimum != 4 || water.Step != 1 || water.Value != 0 || water.Source != "99-strength" {
		t.Fatalf("water: %+v", water)
	}
	if w := day.Habits[2]; w.Type != model.HabitNumeric || w.Kind != "numeric" || w.Unit != "kg" {
		t.Fatalf("weigh-in: %+v", w)
	}

	// Log each kind, by the names the CLI and the Stream Deck use.
	var d model.HabitDay
	for i := 1; i <= 4; i++ {
		if c := e.call("POST", "/api/habits/water/log", map[string]any{}, &d); c != 200 || d.Value != float64(i) || d.Pending != 0 {
			t.Fatalf("water tap %d: %d %+v", i, c, d)
		}
	}
	if !d.MinHit || d.Hit {
		t.Fatalf("four of six bottles is the minimum, not the target: %+v", d)
	}
	if c := e.call("POST", "/api/habits/walks/log", map[string]any{"set": 2}, &d); c != 200 || d.Value != 2 || !d.Hit {
		t.Fatalf("walks: %d %+v", c, d)
	}
	if c := e.call("POST", "/api/habits/creatine/log", map[string]any{"toggle": true}, &d); c != 200 || d.Value != 1 || !d.Hit || d.Type != model.HabitCheck {
		t.Fatalf("creatine: %d %+v", c, d)
	}
	if c := e.call("POST", "/api/habits/weigh-in/log", map[string]any{"delta": 94.6}, &d); c != 200 || d.Value != 94.6 || !d.Hit {
		t.Fatalf("weigh-in: %d %+v", c, d)
	}
	if c := e.call("POST", "/api/habits/"+itoa(water.ID)+"/log", map[string]any{"delta": -1}, &d); c != 200 || d.Value != 3 || d.Name != water.Name {
		t.Fatalf("water by id: %d %+v", c, d)
	}
	date := e.today()
	if e.fake.Value(water.RemoteID, date) != 3 || e.fake.Value(day.Habits[1].RemoteID, date) != 2 || e.fake.Value(day.Habits[4].RemoteID, date) != 1 {
		t.Fatal("99-Strength does not have what was logged")
	}
	if kg, ok := e.fake.Weight(date); !ok || kg != 94.6 {
		t.Fatalf("body weight: %v %v", kg, ok)
	}

	// Every log told open widgets to reload.
	evs := e.drain()
	if len(evs) < 8 {
		t.Fatalf("expected habit.changed for each log, got %d", len(evs))
	}
	for _, ev := range evs {
		if ev.Type != "habit.changed" {
			t.Fatalf("event: %+v", ev)
		}
	}

	// Mistakes get the usual codes.
	var m map[string]string
	if c := e.call("POST", "/api/habits/wa/log", map[string]any{}, &m); c != 400 || !strings.Contains(m["error"], "ambiguous") {
		t.Fatalf("ambiguous: %d %v", c, m)
	}
	if c := e.call("POST", "/api/habits/protein/log", map[string]any{}, &m); c != 404 {
		t.Fatalf("a habit 99-Strength does not have: %d %v", c, m)
	}
	if c := e.call("POST", "/api/habits/water/log", map[string]any{"delta": 250}, &m); c != 400 || !strings.Contains(m["error"], "bottles") {
		t.Fatalf("250 bottles: %d %v", c, m)
	}
	if c := e.call("POST", "/api/habits", map[string]any{"name": "Read", "type": "check"}, &m); c != 400 || !strings.Contains(m["error"], "99-Strength") {
		t.Fatalf("adding a habit while connected: %d %v", c, m)
	}
	if c := e.call("PATCH", "/api/habits/water", map[string]any{"target": 8}, &m); c != 400 {
		t.Fatalf("editing a habit while connected: %d %v", c, m)
	}

	// Going back: the local habits are as they were left.
	var out map[string]any
	if c := e.call("POST", "/api/strength/disconnect", nil, &out); c != 200 || out["connected"] != false {
		t.Fatalf("disconnect: %d %v", c, out)
	}
	e.call("GET", "/api/habits", nil, &day)
	if day.Source != "local" || day.Habits[1].Name != "Water" || day.Habits[1].Value != 500 {
		t.Fatalf("local habits after disconnect: %+v", day)
	}
}

func TestOutageAndRevokedTokenOverHTTP(t *testing.T) {
	e := newStrengthEnv(t)
	e.connect()
	var d model.HabitDay
	e.call("POST", "/api/habits/water/log", map[string]any{}, &d)

	e.fake.Down(503)
	start := time.Now()
	if c := e.call("POST", "/api/habits/water/log", map[string]any{}, &d); c != 200 || d.Value != 2 || d.Pending != 1 {
		t.Fatalf("log during an outage: %d %+v", c, d)
	}
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Fatalf("a log during an outage took %s", took)
	}
	e.call("POST", "/api/habits/water/log", map[string]any{}, &d) // known to be down now: no waiting at all
	var day habitsBody
	e.call("GET", "/api/habits", nil, &day)
	if day.Sync.State != "stale" || !day.Sync.Stale || day.Sync.Pending != 2 || day.Sync.LastSync == nil || day.Habits[0].Value != 3 || day.Habits[0].Pending != 2 {
		t.Fatalf("stale habits: %+v %+v", *day.Sync, day.Habits[0])
	}

	e.fake.Down(0)
	var st strength.Status
	if c := e.call("POST", "/api/strength/sync", nil, &st); c != 200 || st.State != "ok" || st.Pending != 0 {
		t.Fatalf("sync on demand: %d %+v", c, st)
	}
	if got := e.fake.Value(day.Habits[0].RemoteID, e.today()); got != 3 {
		t.Fatalf("water after the replay: %v", got)
	}

	e.fake.SetToken("")
	e.call("POST", "/api/strength/sync", nil, &st)
	if st.State != "auth" || !st.NeedsToken || !st.Connected {
		t.Fatalf("after the token was revoked: %+v", st)
	}
	e.call("GET", "/api/habits", nil, &day)
	if day.Source != "99-strength" || day.Sync.State != "auth" || len(day.Habits) != 5 || day.Habits[0].Value != 3 {
		t.Fatalf("habits with a revoked token: %+v", day)
	}
}

func TestTheTokenIsNowhereButTheTokenStore(t *testing.T) {
	e := newStrengthEnv(t)
	e.connect()
	var d model.HabitDay
	e.call("POST", "/api/habits/water/log", map[string]any{}, &d)
	e.call("GET", "/api/habits", nil, nil)
	e.call("GET", "/api/strength", nil, nil)
	e.fake.Down(500)
	e.call("POST", "/api/habits/walks/log", map[string]any{}, &d)
	e.call("POST", "/api/strength/sync", nil, nil)
	e.fake.Down(0)
	e.fake.SetToken("")
	e.call("POST", "/api/strength/sync", nil, nil)
	e.call("GET", "/api/strength", nil, nil)
	e.call("GET", "/api/habits", nil, nil)
	e.call("GET", "/api/health", nil, nil)
	e.drain()

	if len(e.seen) < 12 {
		t.Fatalf("too little was captured: %d", len(e.seen))
	}
	for _, s := range e.seen { // API answers, events and log lines
		if strings.Contains(s, "nns_pat_") {
			t.Fatalf("the token leaked: %s", s)
		}
	}
	// And not in the database or anything else in the home folder.
	err := filepath.Walk(e.home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte("nns_pat_")) {
			t.Errorf("the token is in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok, _ := e.tokens.Get(); tok != strengthtest.Token {
		t.Fatal("the token store should still hold the token")
	}
}
