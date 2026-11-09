package strength

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/model"
	"terrancc/internal/store"
	"terrancc/internal/strength/strengthtest"
)

// memStore is the settings table in memory.
type memStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

func (s *memStore) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memStore) dump() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.m)
	return string(b)
}

type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) logf(format string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.b, format+"\n", a...)
}

func (l *logBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type env struct {
	t      *testing.T
	fake   *strengthtest.Fake
	srv    *httptest.Server
	store  *memStore
	tokens *MemoryTokens
	logs   *logBuf
	notes  atomic.Int32
	mu     sync.Mutex
	now    time.Time
	svc    *Service
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = e.now.Add(time.Second) // every reading is later than the last
	return e.now
}

func (e *env) setClock(t time.Time) {
	e.mu.Lock()
	e.now = t
	e.mu.Unlock()
}

func (e *env) base() string { return e.srv.URL + "/api/v1" }

// open builds a Service over the env's store, as tccd does at start.
func (e *env) open(o Options) *Service {
	e.t.Helper()
	o.Store, o.Tokens, o.Now, o.Logf = e.store, e.tokens, e.clock, e.logs.logf
	o.Notify = func() { e.notes.Add(1) }
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 2 * time.Second}
	}
	s, err := New(o)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// newEnv is a fake 99-Strength with nothing logged and a service that is
// not connected yet. The worker is not running: tests call Sync.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, fake: strengthtest.New(strengthtest.Token), store: &memStore{m: map[string]string{}}, tokens: &MemoryTokens{}, logs: &logBuf{}}
	e.now = time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	e.srv = httptest.NewServer(e.fake)
	t.Cleanup(e.srv.Close)
	e.svc = e.open(Options{})
	return e
}

func connected(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	if _, err := e.svc.Connect(context.Background(), e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	return e
}

var bg = context.Background()

func f(v float64) *float64 { return &v }

func (e *env) log(ref string, op core.LogOp) model.HabitDay {
	e.t.Helper()
	d, err := e.svc.Log(bg, ref, op)
	if err != nil {
		e.t.Fatalf("log %s: %v", ref, err)
	}
	return d
}

func (e *env) day() Day {
	e.t.Helper()
	d, err := e.svc.Habits(bg, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func (e *env) habit(name string) model.HabitDay {
	e.t.Helper()
	for _, h := range e.day().Habits {
		if strings.HasPrefix(h.Name, name) {
			return h
		}
	}
	e.t.Fatalf("no habit %q", name)
	return model.HabitDay{}
}

// writes lists the fake's write calls as "METHOD path body".
func (e *env) writes() []string {
	var out []string
	for _, r := range e.fake.Writes() {
		out = append(out, r.Method+" "+strings.TrimPrefix(r.Path, "/api/v1")+" "+r.Body)
	}
	return out
}

// --- connecting ---

func TestConnectChecksTheTokenBeforeSavingIt(t *testing.T) {
	e := newEnv(t)
	wrong := "nns_pat_" + strings.Repeat("x", 43)
	if _, err := e.svc.Connect(bg, e.base(), wrong); !errors.Is(err, ErrRejected) {
		t.Fatalf("a token 99-Strength refuses: %v", err)
	}
	if _, err := e.tokens.Get(); !errors.Is(err, ErrNoToken) {
		t.Fatal("a refused token was stored")
	}
	if e.svc.Connected() || e.store.dump() != "{}" {
		t.Fatalf("a refused connect left state behind: %s", e.store.dump())
	}

	for _, bad := range []string{"", "hello", "nns_pat_short", strengthtest.Token + "x"} {
		if _, err := e.svc.Connect(bg, e.base(), bad); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("token %q: %v", bad, err)
		}
	}
	for _, bad := range []string{"", "api-99strength.enjoeneer.dev", "ftp://x/api/v1", "http://api-99strength.enjoeneer.dev/api/v1", "https://user:pw@host/api/v1"} {
		if _, err := e.svc.Connect(bg, bad, strengthtest.Token); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("address %q: %v", bad, err)
		}
	}
	if len(e.fake.Requests()) != 1 {
		t.Fatalf("only the well-formed token should have been sent: %+v", e.fake.Requests())
	}

	e.fake.Down(503)
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a server that is down: %v", err)
	}
	e.fake.Down(0)
	st, err := e.svc.Connect(bg, e.base()+"/", strengthtest.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected || st.State != "ok" || st.Email != "joe@example.com" || st.TokenName != "Terran CC" || st.BaseURL != e.base() || st.LastSync == nil {
		t.Fatalf("status: %+v", st)
	}
	if tok, _ := e.tokens.Get(); tok != strengthtest.Token {
		t.Fatal("the working token was not stored")
	}
}

func TestNotConnected(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Habits(bg, ""); !errors.Is(err, ErrNotConnected) {
		t.Fatal(err)
	}
	if _, err := e.svc.Log(bg, "water", core.LogOp{}); !errors.Is(err, ErrNotConnected) {
		t.Fatal(err)
	}
	e.svc.Sync(bg)
	if st := e.svc.Status(); st.Connected || st.State != "disconnected" || len(e.fake.Requests()) != 0 {
		t.Fatalf("status %+v, calls %v", st, e.fake.Requests())
	}
}

// --- mapping ---

func TestEveryHabitKindMapsOntoTheTerranShape(t *testing.T) {
	// Serve the fixture itself, so this is the mapping of a real answer.
	e := newEnv(t)
	e.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := "habits_today.json"
		if strings.HasSuffix(r.URL.Path, "/whoami") {
			name = "whoami.json"
		}
		w.Write(fixture(t, name))
	})
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	d := e.day()
	if d.Source != "99-strength" || d.Date != fixDate || d.Streak != 12 || len(d.Habits) != 5 {
		t.Fatalf("day: %+v", d)
	}
	if *d.StreakInfo != (StreakInfo{Current: 12, Best: 19, LastCountedDay: "2026-11-08", GraceUsed: true}) {
		t.Fatalf("streak: %+v", *d.StreakInfo)
	}
	if d.Sync.State != "ok" || d.Sync.Stale || d.Sync.Pending != 0 || d.Sync.LastSync == nil {
		t.Fatalf("sync: %+v", *d.Sync)
	}
	want := []model.HabitDay{
		{Habit: model.Habit{ID: 1, Name: "Water (532ml bottle)", Icon: "💧", Unit: "bottles", Type: model.HabitCount, Target: 6, Step: 1, Order: 1},
			Date: fixDate, Value: 3, Source: "99-strength", Kind: "counter", Minimum: 4, RemoteID: waterID},
		{Habit: model.Habit{ID: 2, Name: "Walks", Icon: "🚶", Type: model.HabitCount, Target: 2, Step: 1, Order: 2},
			Date: fixDate, Value: 1, Source: "99-strength", Kind: "counter", Minimum: 1, MinHit: true, RemoteID: walksID},
		{Habit: model.Habit{ID: 3, Name: "Morning weigh-in", Icon: "⚖️", Unit: "kg", Type: model.HabitNumeric, Target: 1, Order: 3},
			Date: fixDate, Value: 94.6, Hit: true, Source: "99-strength", Kind: "numeric", Minimum: 1, MinHit: true, RemoteID: weighID},
		{Habit: model.Habit{ID: 4, Name: "Mobility A done", Icon: "🧘", Type: model.HabitCheck, Target: 1, Step: 1, Order: 4},
			Date: fixDate, Value: 0, Source: "99-strength", Kind: "checkbox", Minimum: 1, RemoteID: mobID},
		{Habit: model.Habit{ID: 5, Name: "Creatine (3–5g)", Icon: "💊", Type: model.HabitCheck, Target: 1, Step: 1, Order: 5},
			Date: fixDate, Value: 1, Hit: true, Source: "99-strength", Kind: "checkbox", Minimum: 1, MinHit: true, RemoteID: creaID},
	}
	for i, w := range want {
		if d.Habits[i] != w {
			t.Errorf("habit %d:\n got %+v\nwant %+v", i, d.Habits[i], w)
		}
	}

	// The JSON keeps every field the local shape has, and adds to it.
	b, _ := json.Marshal(d)
	var raw struct {
		Habits []map[string]any `json:"habits"`
	}
	json.Unmarshal(b, &raw)
	for _, k := range []string{"id", "name", "icon", "unit", "type", "target", "step", "order", "archived", "created_at", "date", "value", "hit",
		"source", "kind", "minimum", "remote_id"} {
		if _, ok := raw.Habits[0][k]; !ok {
			t.Errorf("habit JSON has no %q: %s", k, b)
		}
	}
}

func TestIcons(t *testing.T) {
	s := func(v string) *string { return &v }
	for in, want := range map[string]string{"droplets": "💧", "scale": "⚖️", "🚶": "🚶", "unknown-key": ""} {
		if got := iconFor(s(in)); got != want {
			t.Errorf("icon %q: %q, want %q", in, got, want)
		}
	}
	if iconFor(nil) != "" {
		t.Error("no icon should stay empty")
	}
}

// --- logging, with 99-Strength reachable ---

func TestLogEachOperation(t *testing.T) {
	e := connected(t)
	at := "\"clientTimestamp\":\""

	// Counter: a bare log and +1 are 99-Strength's increment.
	e.log("water", core.LogOp{})
	e.log("water", core.LogOp{Delta: f(1)})
	e.svc.Sync(bg)
	if v := e.fake.Value(waterID, fixDate); v != 2 {
		t.Fatalf("water after two taps: %v", v)
	}
	// -1 is its decrement.
	e.log("water", core.LogOp{Delta: f(-1)})
	e.svc.Sync(bg)
	// +3 and =5 are "set to value".
	e.log("water", core.LogOp{Delta: f(3)})
	e.svc.Sync(bg)
	e.log("water", core.LogOp{Set: f(5)})
	e.svc.Sync(bg)
	// Checkbox: toggle on, toggle off, a bare log ticks it again.
	e.log("creatine", core.LogOp{Toggle: true})
	e.svc.Sync(bg)
	e.log("creatine", core.LogOp{Toggle: true})
	e.svc.Sync(bg)
	e.log("creatine", core.LogOp{})
	e.svc.Sync(bg)
	// Numeric: the weigh-in goes to body weight and to the habit.
	e.log("weigh-in", core.LogOp{Delta: f(94.64)})
	e.svc.Sync(bg)

	got := e.writes()
	inc := " /habits/" + waterID + "/logs/2026-11-09/increment "
	set := "PUT /habits/%s/logs/2026-11-09 "
	want := []string{
		"POST" + inc + `{"clientTimestamp":"…","delta":1}`,
		"POST" + inc + `{"clientTimestamp":"…","delta":1}`,
		"POST" + inc + `{"clientTimestamp":"…","delta":-1}`,
		fmt.Sprintf(set, waterID) + `{"clientTimestamp":"…","value":4}`,
		fmt.Sprintf(set, waterID) + `{"clientTimestamp":"…","value":5}`,
		fmt.Sprintf(set, creaID) + `{"clientTimestamp":"…","value":1}`,
		fmt.Sprintf(set, creaID) + `{"clientTimestamp":"…","value":0}`,
		fmt.Sprintf(set, creaID) + `{"clientTimestamp":"…","value":1}`,
		"PUT /body-weight/2026-11-09 " + `{"kg":94.6}`,
		fmt.Sprintf(set, weighID) + `{"clientTimestamp":"…","value":94.6}`,
	}
	if len(got) != len(want) {
		t.Fatalf("writes:\n%s", strings.Join(got, "\n"))
	}
	for i := range want {
		g := got[i]
		if j := strings.Index(g, at); j >= 0 { // blank the timestamp
			k := j + len(at)
			g = g[:k] + "…" + g[k+len("2026-11-09T09:00:00.000Z"):]
		}
		if g != want[i] {
			t.Errorf("write %d:\n got %s\nwant %s", i, g, want[i])
		}
	}

	if kg, ok := e.fake.Weight(fixDate); !ok || kg != 94.6 {
		t.Fatalf("body weight: %v %v", kg, ok)
	}
	water, crea, weigh := e.habit("Water"), e.habit("Creatine"), e.habit("Morning")
	if water.Value != 5 || !water.MinHit || water.Hit || water.Pending != 0 {
		t.Fatalf("water: %+v", water)
	}
	if crea.Value != 1 || !crea.Hit || weigh.Value != 94.6 || !weigh.Hit {
		t.Fatalf("creatine %+v, weigh-in %+v", crea, weigh)
	}
	if d := e.day(); d.Sync.Pending != 0 || d.Sync.State != "ok" {
		t.Fatalf("sync: %+v", *d.Sync)
	}
}

func TestLogAnswersWithTheConfirmedValue(t *testing.T) {
	e := newEnv(t)
	e.svc = e.open(Options{LogWait: 2 * time.Second})
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	go e.svc.Run(ctx)

	// The phone logged two bottles since Terran CC last looked.
	e.fake.Put(waterID, fixDate, 2, time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC))
	d := e.log("water", core.LogOp{})
	if d.Value != 3 || d.Pending != 0 {
		t.Fatalf("an increment adds to what 99-Strength has: %+v", d)
	}
}

func TestLogRefusesWhatMakesNoSense(t *testing.T) {
	e := connected(t)
	for name, c := range map[string]struct {
		ref string
		op  core.LogOp
	}{
		"toggle a counter":       {"water", core.LogOp{Toggle: true}},
		"millilitres of bottles": {"water", core.LogOp{Delta: f(250)}},
		"weigh-in with no value": {"weigh-in", core.LogOp{}},
		"toggle the weigh-in":    {"weigh-in", core.LogOp{Toggle: true}},
		"a 12 kg person":         {"weigh-in", core.LogOp{Delta: f(12)}},
		"a 500 kg person":        {"weigh-in", core.LogOp{Set: f(500)}},
		"two habits match":       {"wa", core.LogOp{}},
	} {
		if _, err := e.svc.Log(bg, c.ref, c.op); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.Log(bg, "protein", core.LogOp{}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown habit: %v", err)
	}
	// Taking one off an empty counter changes nothing and sends nothing.
	if d := e.log("walks", core.LogOp{Delta: f(-1)}); d.Value != 0 || d.Pending != 0 {
		t.Fatalf("walks: %+v", d)
	}
	e.svc.Sync(bg)
	if w := e.writes(); len(w) != 0 {
		t.Fatalf("nothing should have been written: %v", w)
	}
}

func TestHabitNamesResolveForgivingly(t *testing.T) {
	e := connected(t)
	for ref, want := range map[string]string{
		"water": waterID, "WATER": waterID, "Water (532ml bottle)": waterID, "bottle": waterID, "1": waterID,
		"walks": walksID, "walk": walksID, "2": walksID,
		"weigh-in": weighID, "weigh in": weighID, "morning": weighID, "weigh": weighID,
		"mobility": mobID, "mob": mobID,
		"creatine": creaID, "crea": creaID, creaID: creaID,
	} {
		e.svc.mu.Lock()
		h, err := e.svc.findLocked(ref)
		e.svc.mu.Unlock()
		if err != nil || h.ID != want {
			t.Errorf("%q: %v %v", ref, h.Name, err)
		}
	}
	e.svc.mu.Lock()
	_, err := e.svc.findLocked("m") // Morning weigh-in, Mobility A done
	e.svc.mu.Unlock()
	if !errors.Is(err, core.ErrInvalid) || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous: %v", err)
	}
}

// --- offline ---

func TestOfflineWritesQueueAndReplayInOrderWithoutDoubleCounting(t *testing.T) {
	e := connected(t)
	e.log("water", core.LogOp{})
	e.log("water", core.LogOp{})
	e.svc.Sync(bg) // 99-Strength has 2 bottles
	synced := *e.day().Sync.LastSync

	e.fake.Down(503)
	// The first tap finds out the hard way. It was meant as an increment.
	if d := e.log("water", core.LogOp{}); d.Value != 3 || d.Pending != 1 {
		t.Fatalf("first offline tap: %+v", d)
	}
	e.svc.Sync(bg)
	d := e.day()
	if d.Sync.State != "stale" || !d.Sync.Stale || d.Sync.Pending != 1 || !d.Sync.LastSync.Equal(synced) || d.Sync.Error == "" {
		t.Fatalf("after the failure: %+v", *d.Sync)
	}
	// More writes while it is down, to different habits, in this order.
	e.log("water", core.LogOp{})
	e.log("creatine", core.LogOp{Toggle: true})
	e.log("water", core.LogOp{Delta: f(-1)})
	e.log("walks", core.LogOp{Delta: f(2)})
	e.log("weigh-in", core.LogOp{Delta: f(93.9)})
	e.svc.Sync(bg) // still down

	d = e.day()
	if d.Sync.Pending != 6 || d.Sync.State != "stale" {
		t.Fatalf("queued: %+v", *d.Sync)
	}
	if w := e.habit("Water"); w.Value != 3 || w.Pending != 3 || w.MinHit {
		t.Fatalf("water while offline: %+v", w)
	}
	if c := e.habit("Creatine"); c.Value != 1 || !c.Hit || c.Pending != 1 {
		t.Fatalf("creatine while offline: %+v", c)
	}
	if w := e.habit("Walks"); w.Value != 2 || !w.Hit || w.Pending != 1 {
		t.Fatalf("walks while offline: %+v", w)
	}
	if e.fake.Value(waterID, fixDate) != 2 || e.fake.Value(creaID, fixDate) != 0 {
		t.Fatal("the fake changed while it was down")
	}

	before := len(e.fake.Writes())
	e.fake.Down(0)
	e.svc.Sync(bg)

	var replayed []string
	for _, w := range e.writes()[before:] {
		if i := strings.LastIndex(w, ","); i >= 0 { // drop the timestamp
			w = w[:strings.Index(w, " {")] + " " + w[i+1:]
		}
		replayed = append(replayed, w)
	}
	want := []string{
		"PUT /habits/" + waterID + "/logs/2026-11-09 \"value\":3}",
		"PUT /habits/" + waterID + "/logs/2026-11-09 \"value\":4}",
		"PUT /habits/" + creaID + "/logs/2026-11-09 \"value\":1}",
		"PUT /habits/" + waterID + "/logs/2026-11-09 \"value\":3}",
		"PUT /habits/" + walksID + "/logs/2026-11-09 \"value\":2}",
		"PUT /body-weight/2026-11-09 {\"kg\":93.9}",
		"PUT /habits/" + weighID + "/logs/2026-11-09 \"value\":93.9}",
	}
	if strings.Join(replayed, "\n") != strings.Join(want, "\n") {
		t.Fatalf("replay order:\n%s\nwant\n%s", strings.Join(replayed, "\n"), strings.Join(want, "\n"))
	}
	for id, v := range map[string]float64{waterID: 3, creaID: 1, walksID: 2, weighID: 93.9} {
		if got := e.fake.Value(id, fixDate); got != v {
			t.Errorf("%s: 99-Strength has %v, want %v", id, got, v)
		}
	}
	d = e.day()
	if d.Sync.State != "ok" || d.Sync.Pending != 0 || d.Sync.Error != "" || !d.Sync.LastSync.After(synced) {
		t.Fatalf("after the replay: %+v", *d.Sync)
	}
	if w := e.habit("Water"); w.Value != 3 || w.Pending != 0 {
		t.Fatalf("water after the replay: %+v", w)
	}

	// Replaying the very same writes again (a crash before the queue was
	// saved, say) changes nothing: they are sets, not increments.
	for _, r := range e.fake.Writes()[before:] {
		req, _ := http.NewRequest(r.Method, e.srv.URL+r.Path, strings.NewReader(r.Body))
		req.Header.Set("Authorization", r.Authorization)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if got := e.fake.Value(waterID, fixDate); got != 3 {
		t.Fatalf("a second replay changed water to %v", got)
	}
}

func TestAnIncrementWhoseAnswerIsLostIsNotCountedTwice(t *testing.T) {
	e := connected(t)
	e.log("water", core.LogOp{})
	e.svc.Sync(bg) // 1 bottle

	e.fake.LoseNext(1) // the next write is applied, but its answer never arrives
	if d := e.log("water", core.LogOp{}); d.Value != 2 {
		t.Fatalf("tap: %+v", d)
	}
	e.svc.Sync(bg)
	if got := e.fake.Value(waterID, fixDate); got != 2 {
		t.Fatalf("the lost increment should have been applied by the fake: %v", got)
	}
	if d := e.day(); d.Sync.Pending != 1 || !d.Sync.Stale {
		t.Fatalf("the write must still be queued: %+v", *d.Sync)
	}

	e.svc.Sync(bg) // the retry
	w := e.writes()
	if last := w[len(w)-1]; !strings.HasPrefix(last, "PUT /habits/"+waterID+"/logs/2026-11-09 ") || !strings.Contains(last, `"value":2`) {
		t.Fatalf("the retry must be a set to 2, not another increment: %s", last)
	}
	if got := e.fake.Value(waterID, fixDate); got != 2 {
		t.Fatalf("water was counted twice: %v", got)
	}
	if h := e.habit("Water"); h.Value != 2 || h.Pending != 0 {
		t.Fatalf("water: %+v", h)
	}
}

func TestQueueAndCacheSurviveARestart(t *testing.T) {
	e := connected(t)
	e.log("water", core.LogOp{})
	e.svc.Sync(bg)
	e.fake.Down(503)
	e.log("water", core.LogOp{})
	e.log("mobility", core.LogOp{})
	calls := len(e.fake.Requests())

	// tccd restarts while 99-Strength is still down.
	e.svc = e.open(Options{})
	d := e.day()
	if len(e.fake.Requests()) != calls {
		t.Fatal("drawing the widget must not wait for the network")
	}
	if len(d.Habits) != 5 || d.Sync.Pending != 2 || d.Sync.LastSync == nil {
		t.Fatalf("after restart: %+v", *d.Sync)
	}
	if w := e.habit("Water"); w.Value != 2 || w.Pending != 1 {
		t.Fatalf("water from the cache plus the queue: %+v", w)
	}
	e.svc.Sync(bg)
	if d := e.day(); !d.Sync.Stale || d.Sync.State != "stale" {
		t.Fatalf("still down: %+v", *d.Sync)
	}

	e.fake.Down(0)
	e.svc.Sync(bg)
	if e.fake.Value(waterID, fixDate) != 2 || e.fake.Value(mobID, fixDate) != 1 {
		t.Fatalf("replay after restart: water %v, mobility %v", e.fake.Value(waterID, fixDate), e.fake.Value(mobID, fixDate))
	}
}

func TestCacheGoesStaleWithAge(t *testing.T) {
	e := connected(t)
	if e.day().Sync.Stale {
		t.Fatal("fresh after connect")
	}
	e.setClock(time.Date(2026, 11, 9, 9, 30, 0, 0, time.UTC)) // no sync for half an hour
	if d := e.day(); !d.Sync.Stale || d.Sync.State != "stale" {
		t.Fatalf("old cache: %+v", *d.Sync)
	}
	e.svc.Sync(bg)
	if e.day().Sync.Stale {
		t.Fatal("fresh after a sync")
	}
}

func TestANewDayStartsAtZeroUntilTheNextRead(t *testing.T) {
	e := connected(t)
	e.log("water", core.LogOp{})
	e.svc.Sync(bg)
	e.fake.Down(503)
	e.log("walks", core.LogOp{}) // queued for the 9th
	e.svc.Sync(bg)

	// 00:30 London time on the 10th. The cache still holds the 9th.
	e.setClock(time.Date(2026, 11, 10, 0, 30, 0, 0, time.UTC))
	d := e.day()
	if d.Date != "2026-11-10" || !d.Sync.Stale || d.Sync.Pending != 1 {
		t.Fatalf("new day: %s %+v", d.Date, *d.Sync)
	}
	for _, h := range d.Habits {
		if h.Value != 0 || h.Hit || h.MinHit || h.Pending != 0 {
			t.Fatalf("yesterday's value shown today: %+v", h)
		}
	}
	// A tap now belongs to the 10th; yesterday's queued write to the 9th.
	e.log("water", core.LogOp{})
	e.fake.Down(0)
	e.svc.Sync(bg)
	if e.fake.Value(walksID, "2026-11-09") != 1 || e.fake.Value(waterID, "2026-11-10") != 1 || e.fake.Value(waterID, "2026-11-09") != 1 {
		t.Fatal("writes landed on the wrong day")
	}
	if w := e.habit("Water"); w.Value != 1 || w.Date != "2026-11-10" {
		t.Fatalf("water on the 10th: %+v", w)
	}
}

func TestLondonDayNotUTCDay(t *testing.T) {
	e := connected(t)
	// 23:30 UTC on 30 June is 00:30 on 1 July in London (BST).
	e.setClock(time.Date(2026, 6, 30, 23, 30, 0, 0, time.UTC))
	if d := e.day(); d.Date != "2026-07-01" {
		t.Fatalf("date: %s", d.Date)
	}
}

// --- a slow API ---

func TestASlowAPINeverHoldsUpALog(t *testing.T) {
	e := newEnv(t)
	e.svc = e.open(Options{LogWait: 100 * time.Millisecond, HTTP: &http.Client{Timeout: 400 * time.Millisecond}})
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	go e.svc.Run(ctx)
	time.Sleep(50 * time.Millisecond) // let the first round finish

	e.fake.Slow(3 * time.Second)
	start := time.Now()
	d := e.log("water", core.LogOp{})
	if took := time.Since(start); took > time.Second {
		t.Fatalf("log took %s", took)
	}
	if d.Value != 1 || d.Pending != 1 {
		t.Fatalf("the answer should be the expected value, pending: %+v", d)
	}
	start = time.Now()
	e.day()
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("reading habits took %s", took)
	}

	// Once the timeout has been seen the service knows it is offline and
	// does not wait at all.
	deadline := time.Now().Add(3 * time.Second)
	for !e.svc.Status().Stale && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if st := e.svc.Status(); !st.Stale || st.Pending != 1 {
		t.Fatalf("status: %+v", st)
	}
}

// --- 401 ---

func TestA401AsksForANewTokenAndKeepsEverything(t *testing.T) {
	e := connected(t)
	e.log("water", core.LogOp{})
	e.svc.Sync(bg)
	synced := *e.day().Sync.LastSync

	e.fake.SetToken("") // revoked
	e.log("water", core.LogOp{})
	e.svc.Sync(bg)

	st := e.svc.Status()
	if !st.Connected || st.State != "auth" || !st.NeedsToken || !st.Stale || st.Pending != 1 || st.Error == "" {
		t.Fatalf("status after a 401: %+v", st)
	}
	d := e.day()
	if d.Sync.State != "auth" || len(d.Habits) != 5 || !d.Sync.LastSync.Equal(synced) {
		t.Fatalf("the cache must stay: %+v", *d.Sync)
	}
	if w := e.habit("Water"); w.Value != 2 || w.Pending != 1 {
		t.Fatalf("water: %+v", w)
	}
	// It stops calling until there is a new token...
	calls := len(e.fake.Requests())
	e.svc.Sync(bg)
	e.day()
	if len(e.fake.Requests()) != calls {
		t.Fatal("kept calling with a token that was refused")
	}
	// ...but still takes writes, and survives a restart in that state.
	e.log("creatine", core.LogOp{})
	e.svc = e.open(Options{})
	if st := e.svc.Status(); st.State != "auth" || st.Pending != 2 {
		t.Fatalf("after restart: %+v", st)
	}

	// A new token for the same account replays what was queued.
	renewed := "nns_pat_" + strings.Repeat("N", 43)
	e.fake.SetToken(renewed)
	st, err := e.svc.Connect(bg, e.base(), renewed)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" || st.NeedsToken || st.Pending != 0 {
		t.Fatalf("after renewing: %+v", st)
	}
	if e.fake.Value(waterID, fixDate) != 2 || e.fake.Value(creaID, fixDate) != 1 {
		t.Fatalf("replay after renewing: water %v, creatine %v", e.fake.Value(waterID, fixDate), e.fake.Value(creaID, fixDate))
	}
}

func TestAMissingStoredTokenAsksForANewOne(t *testing.T) {
	e := connected(t)
	e.tokens.Delete() // removed in Credential Manager by hand
	e.svc.Sync(bg)
	if st := e.svc.Status(); st.State != "auth" || !st.Connected {
		t.Fatalf("status: %+v", st)
	}
}

// --- a write 99-Strength will never accept ---

func TestARefusedWriteIsDroppedNotRetriedForever(t *testing.T) {
	e := connected(t)
	e.fake.SetHabits(strengthtest.SeedHabits()[1:]) // water was deleted on the phone
	e.log("water", core.LogOp{})
	e.log("walks", core.LogOp{})
	e.svc.Sync(bg)
	d := e.day()
	if d.Sync.Pending != 0 || len(d.Habits) != 4 || e.fake.Value(walksID, fixDate) != 1 {
		t.Fatalf("after the 404: %+v, %d habits", *d.Sync, len(d.Habits))
	}
	if d.Sync.State != "ok" || !strings.Contains(d.Sync.Error, "refused the change to Water") {
		t.Fatalf("the widget should be told: %+v", *d.Sync)
	}
	if !strings.Contains(e.logs.String(), "dropping the write to Water") {
		t.Fatalf("the drop should be logged:\n%s", e.logs.String())
	}
}

// --- disconnecting ---

func TestDisconnectForgetsEverything(t *testing.T) {
	e := connected(t)
	e.fake.Down(503)
	e.log("water", core.LogOp{})
	dropped, err := e.svc.Disconnect()
	if err != nil || dropped != 1 {
		t.Fatalf("disconnect: %d %v", dropped, err)
	}
	if _, err := e.tokens.Get(); !errors.Is(err, ErrNoToken) {
		t.Fatal("the token is still stored")
	}
	if st := e.svc.Status(); st.Connected || st.State != "disconnected" || st.BaseURL != "" {
		t.Fatalf("status: %+v", st)
	}
	if strings.Contains(e.store.dump(), "Water") || strings.Contains(e.store.dump(), e.base()) {
		t.Fatalf("cache or address left behind: %s", e.store.dump())
	}
	e.svc = e.open(Options{})
	if e.svc.Connected() {
		t.Fatal("connected again after a restart")
	}
}

func TestConnectingAnotherAccountStartsClean(t *testing.T) {
	e := connected(t)
	e.fake.Down(503)
	e.log("water", core.LogOp{})

	other := strengthtest.New(strengthtest.Token)
	other.SetHabits(strengthtest.SeedHabits()[:2])
	srv := httptest.NewServer(other)
	defer srv.Close()
	st, err := e.svc.Connect(bg, srv.URL+"/api/v1", strengthtest.Token)
	if err != nil || st.Pending != 0 {
		t.Fatalf("connect: %+v %v", st, err)
	}
	if d := e.day(); len(d.Habits) != 2 || d.Habits[0].Value != 0 {
		t.Fatalf("habits: %+v", d.Habits)
	}
	if len(other.Writes()) != 0 {
		t.Fatal("a write queued for one server was sent to another")
	}
}

// --- events ---

func TestNotifyOnlyWhenTheWidgetWouldChange(t *testing.T) {
	e := connected(t)
	n := e.notes.Load()
	e.svc.Sync(bg)
	e.svc.Sync(bg)
	if e.notes.Load() != n {
		t.Fatal("a refresh that changed nothing must not publish")
	}
	e.fake.Put(walksID, fixDate, 1, time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)) // logged on the phone
	e.svc.Sync(bg)
	if e.notes.Load() == n {
		t.Fatal("a change made elsewhere must publish")
	}
	n = e.notes.Load()
	e.log("water", core.LogOp{})
	if e.notes.Load() == n {
		t.Fatal("a log must publish")
	}
	n = e.notes.Load()
	e.fake.Down(503)
	e.svc.Sync(bg)
	if e.notes.Load() == n {
		t.Fatal("going stale must publish")
	}
}

// --- the token stays in the token store ---

func TestTheTokenNeverLeavesTheTokenStore(t *testing.T) {
	e := connected(t)
	var out []string
	keep := func(v any) {
		b, _ := json.Marshal(v)
		out = append(out, string(b))
	}
	keep(e.svc.Status())
	keep(e.day())
	keep(e.log("water", core.LogOp{}))
	e.svc.Sync(bg)
	e.fake.Down(500)
	keep(e.log("water", core.LogOp{}))
	e.svc.Sync(bg)
	keep(e.svc.Status())
	e.fake.Down(0)
	e.fake.SetToken("")
	e.svc.Sync(bg)
	keep(e.svc.Status())
	keep(e.day())
	_, err := e.svc.Connect(bg, "http://127.0.0.1:1/api/v1", strengthtest.Token) // nothing listens there
	out = append(out, fmt.Sprint(err))
	_, err = e.svc.Connect(bg, e.base(), strengthtest.Token) // refused
	out = append(out, fmt.Sprint(err))

	out = append(out, e.store.dump(), e.logs.String())
	for _, s := range out {
		if strings.Contains(s, strengthtest.Token) || strings.Contains(s, "nns_pat_") {
			t.Fatalf("the token leaked into: %s", s)
		}
	}
	if e.logs.String() == "" || !strings.Contains(e.store.dump(), "base_url") {
		t.Fatal("the test looked at nothing")
	}
	// It did go where it should: every call carried it.
	for _, r := range e.fake.Requests()[:3] {
		if r.Authorization != "Bearer "+strengthtest.Token {
			t.Fatalf("call without the token: %+v", r)
		}
	}
}

func TestRedact(t *testing.T) {
	if got := redact("Get https://x/?t="+strengthtest.Token+": refused", strengthtest.Token); strings.Contains(got, "nns_pat_") {
		t.Fatal(got)
	}
}

func TestOtherDatesAreReadLive(t *testing.T) {
	e := connected(t)
	e.fake.Put(walksID, "2026-11-07", 2, time.Date(2026, 11, 7, 18, 0, 0, 0, time.UTC))
	d, err := e.svc.Habits(bg, "2026-11-07")
	if err != nil || d.Date != "2026-11-07" || d.Habits[1].Value != 2 || !d.Habits[1].Hit {
		t.Fatalf("7 Nov: %+v %v", d, err)
	}
	e.fake.Down(503)
	if _, err := e.svc.Habits(bg, "2026-11-07"); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("offline: %v", err)
	}
}
