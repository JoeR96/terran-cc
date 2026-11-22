package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terrancc/internal/api"
	"terrancc/internal/client"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/model"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/strength"
	"terrancc/internal/strength/strengthtest"
	"terrancc/internal/timer"
)

// strengthHarness is tcc talking to a tccd that can reach a fake 99-Strength.
type strengthHarness struct {
	*harness
	fake   *strengthtest.Fake
	base   string
	tokens *strength.MemoryTokens
}

func newStrengthHarness(t *testing.T) *strengthHarness {
	t.Helper()
	home := t.TempDir()
	db, err := sqlite.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	svc.SeedHabits(context.Background())
	fake := strengthtest.New(strengthtest.Token)
	remote := httptest.NewServer(fake)
	tokens := &strength.MemoryTokens{}
	bus := events.NewBus()
	st, err := strength.New(strength.Options{
		Store: db, Tokens: tokens, LogWait: 2 * time.Second, HTTP: &http.Client{Timeout: time.Second},
		Notify: func() { bus.Publish(events.Event{Type: "habit.changed"}) },
		Logf:   func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { st.Run(ctx); close(done) }()
	srv := httptest.NewServer(api.New(svc, timer.New(timer.DefaultConfig(), time.Now), bus, "tok", home, api.WithStrength(st)))
	t.Cleanup(func() {
		srv.Close()
		remote.Close()
		cancel()
		<-done
		db.Close()
	})
	return &strengthHarness{
		harness: &harness{t: t, c: client.New(srv.Listener.Addr().String(), "tok")},
		fake:    fake, base: remote.URL + "/api/v1", tokens: tokens,
	}
}

// runIn is run with something on stdin.
func (h *strengthHarness) runIn(stdin string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	root := NewRoot(func() (*client.Client, error) { return h.c, nil }, &out, &errOut)
	root.SetIn(strings.NewReader(stdin))
	code := Execute(root, args)
	return code, out.String(), errOut.String()
}

func (h *strengthHarness) connect() {
	h.t.Helper()
	h.ok("strength", "connect", "--api", h.base, "--token", strengthtest.Token)
}

func has(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in:\n%s", w, out)
		}
	}
}

func TestStrengthConnectStatusDisconnect(t *testing.T) {
	h := newStrengthHarness(t)
	localHabits, localStatus := h.ok("habit"), h.ok("status")
	has(t, h.ok("strength", "status"), "not connected", "tcc strength connect --api")

	// A token that does not work is refused and nothing is stored.
	code, _, errOut := h.run("strength", "connect", "--api", h.base, "--token", "nns_pat_"+strings.Repeat("q", 43))
	if code != 1 || !strings.Contains(errOut, "did not accept that token") {
		t.Fatalf("refused token: %d %q", code, errOut)
	}
	if code, _, errOut := h.run("strength", "connect", "--token", strengthtest.Token); code != 1 || !strings.Contains(errOut, "--api is required") {
		t.Fatalf("no --api: %d %q", code, errOut)
	}
	if code, _, errOut := h.runIn("", "strength", "connect", "--api", h.base); code != 1 || !strings.Contains(errOut, "no token given") {
		t.Fatalf("empty stdin: %d %q", code, errOut)
	}
	if _, err := h.tokens.Get(); err == nil {
		t.Fatal("a token was stored")
	}
	if h.ok("habit") != localHabits {
		t.Fatal("a failed connect changed the habits")
	}

	// The token arrives on stdin, as from a pipe.
	code, out, errOut := h.runIn(strengthtest.Token+"\r\n", "strength", "connect", "--api", h.base)
	if code != 0 {
		t.Fatalf("connect from stdin: %d %s%s", code, out, errOut)
	}
	has(t, out, "Connected to "+h.base, "joe@example.com", `token "Terran CC"`, "5 habits now come from 99-Strength", "memory (not saved)")
	if strings.Contains(out+errOut, "nns_pat_") {
		t.Fatalf("connect printed the token: %s%s", out, errOut)
	}
	if tok, _ := h.tokens.Get(); tok != strengthtest.Token {
		t.Fatal("the token was not stored")
	}

	out = h.ok("strength", "status")
	has(t, out, "connected to "+h.base, "joe@example.com", "up to date", "token     kept in")
	var st strength.Status
	if err := json.Unmarshal([]byte(h.ok("strength", "status", "--json")), &st); err != nil || !st.Connected || st.State != "ok" || st.BaseURL != h.base {
		t.Fatalf("status --json: %+v %v", st, err)
	}
	if strings.Contains(h.ok("strength", "--json"), "nns_pat_") {
		t.Fatal("status --json holds the token")
	}

	out = h.ok("strength", "disconnect")
	has(t, out, "Disconnected", "local ones again")
	if _, err := h.tokens.Get(); err == nil {
		t.Fatal("the token is still stored")
	}
	// Local habits are exactly what they were.
	if got := h.ok("habit"); got != localHabits {
		t.Fatalf("habits after disconnect:\n%s\nbefore connect:\n%s", got, localHabits)
	}
	if got := h.ok("status"); got != localStatus {
		t.Fatalf("status after disconnect:\n%s\nbefore connect:\n%s", got, localStatus)
	}
}

func TestHabitCommandsAgainst99Strength(t *testing.T) {
	h := newStrengthHarness(t)
	h.connect()

	out := h.ok("habit")
	has(t, out, "🔥 12-day streak · 99-Strength",
		"  💧 Water (532ml bottle) 0/6 bottles (min 4)\n",
		"  🚶 Walks 0/2 (min 1)\n",
		"  ⚖️ Morning weigh-in not logged\n",
		"  🧘 Mobility A done ·\n",
		"  💊 Creatine (3–5g) ·\n")

	// The same forgiving names as before, and every way of logging.
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"water"}, "💧 Water (532ml bottle) 1/6 bottles (min 4)\n"},
		{[]string{"water", "+1"}, "2/6 bottles (min 4)\n"},
		{[]string{"WATER", "=5"}, "5/6 bottles (min 4) ✓ min\n"},
		{[]string{"water", "-1"}, "4/6 bottles (min 4) ✓ min\n"},
		{[]string{"water", "2"}, "6/6 bottles (min 4) ✓\n"},
		{[]string{"walks"}, "🚶 Walks 1/2 (min 1) ✓ min\n"},
		{[]string{"creatine", "toggle"}, "💊 Creatine (3–5g) ✓\n"},
		{[]string{"creatine"}, "💊 Creatine (3–5g) ·\n"},
		{[]string{"mob"}, "🧘 Mobility A done ✓\n"},
		{[]string{"weigh-in", "94.6"}, "⚖️ Morning weigh-in 94.6 kg ✓\n"},
		{[]string{"weigh", "=94.2"}, "⚖️ Morning weigh-in 94.2 kg ✓\n"},
	} {
		out := h.ok(append([]string{"habit", "log"}, c.args...)...)
		if !strings.HasSuffix(out, c.want) {
			t.Errorf("habit log %v: %q, want …%q", c.args, out, c.want)
		}
	}
	today := time.Now().In(london(t)).Format("2006-01-02")
	hs := strengthtest.SeedHabits()
	for i, want := range []float64{6, 1, 94.2, 1, 0} {
		if got := h.fake.Value(hs[i].ID, today); got != want {
			t.Errorf("%s: 99-Strength has %v, want %v", hs[i].Name, got, want)
		}
	}
	if kg, _ := h.fake.Weight(today); kg != 94.2 {
		t.Errorf("body weight: %v", kg)
	}

	// Mistakes.
	code, _, errOut := h.run("habit", "log", "wa")
	if code != 1 || !strings.Contains(errOut, "ambiguous") || !strings.Contains(errOut, "Water (532ml bottle), Walks") {
		t.Fatalf("ambiguous: %d %q", code, errOut)
	}
	if code, _, _ := h.run("habit", "log", "protein"); code != 2 {
		t.Fatalf("a habit that is not in 99-Strength should be not found: %d", code)
	}
	if code, _, errOut := h.run("habit", "log", "water", "+250"); code != 1 || !strings.Contains(errOut, "target of 6 bottles") {
		t.Fatalf("+250 bottles: %d %q", code, errOut)
	}
	if code, _, errOut := h.run("habit", "log", "weigh-in"); code != 1 || !strings.Contains(errOut, "needs a value") {
		t.Fatalf("weigh-in with no value: %d %q", code, errOut)
	}
	if code, _, errOut := h.run("habit", "add", "Read", "--type", "check"); code != 1 || !strings.Contains(errOut, "99-Strength") {
		t.Fatalf("habit add while connected: %d %q", code, errOut)
	}

	// JSON says where the habits come from.
	var day client.HabitsDay
	json.Unmarshal([]byte(h.ok("habit", "--json")), &day)
	if day.Source != "99-strength" || len(day.Habits) != 5 || day.Sync == nil || day.Sync.State != "ok" || day.StreakInfo == nil ||
		day.Habits[0].Minimum != 4 || day.Habits[0].Kind != "counter" || !day.Habits[0].Hit || day.Habits[2].Type != model.HabitNumeric {
		t.Fatalf("habit --json: %+v", day)
	}
	var d model.HabitDay
	json.Unmarshal([]byte(h.ok("habit", "log", "walks", "--json")), &d)
	if d.Source != "99-strength" || d.Value != 2 || !d.Hit || d.RemoteID != hs[1].ID {
		t.Fatalf("habit log --json: %+v", d)
	}
	var status struct {
		Habits client.HabitsDay `json:"habits"`
	}
	json.Unmarshal([]byte(h.ok("status", "--json")), &status)
	if status.Habits.Source != "99-strength" || len(status.Habits.Habits) != 5 {
		t.Fatalf("status --json: %+v", status.Habits)
	}
	has(t, h.ok("status"), "streak · 99-Strength", "💧 Water (532ml bottle) 6/6 bottles (min 4) ✓", "Timer:")
}

func TestLocalJSONSaysLocal(t *testing.T) {
	h := newHarness(t)
	var day client.HabitsDay
	json.Unmarshal([]byte(h.ok("habit", "--json")), &day)
	if day.Source != "local" || day.Sync != nil || len(day.Habits) != 5 {
		t.Fatalf("habit --json: %+v", day)
	}
	var d model.HabitDay
	json.Unmarshal([]byte(h.ok("habit", "log", "water", "--json")), &d)
	if d.Source != "local" || d.Value != 250 {
		t.Fatalf("habit log --json: %+v", d)
	}
	if out := h.ok("habit"); strings.Contains(out, "99-Strength") || strings.Contains(out, "local") {
		t.Fatalf("the local habit list should look as it always did:\n%s", out)
	}
	has(t, h.ok("strength"), "not connected")
}

func TestOfflineAndRevokedTokenShowInTheCLI(t *testing.T) {
	h := newStrengthHarness(t)
	h.connect()
	h.ok("habit", "log", "water")

	h.fake.Down(503)
	out := h.ok("habit", "log", "water")
	has(t, out, "2/6 bottles", "⟳", "waiting to reach 99-Strength")
	h.ok("habit", "log", "creatine")
	out = h.ok("habit")
	has(t, out, "Water (532ml bottle) 2/6 bottles (min 4) ⟳", "Creatine (3–5g) ✓ ⟳", "⚠ 99-Strength is offline. Showing values from ", "2 changes waiting to sync")
	has(t, h.ok("status"), "⚠ 99-Strength is offline", "2 changes waiting to sync")
	has(t, h.ok("strength", "status"), "offline, showing saved values", "queued    2 changes waiting to sync", "problem   99-Strength answered 503")

	// Disconnecting now would lose those two changes, so it asks.
	if code, _, errOut := h.run("strength", "disconnect"); code != 1 || !strings.Contains(errOut, "2 changes waiting to sync") || !strings.Contains(errOut, "--force") {
		t.Fatalf("disconnect with queued changes: %d %q", code, errOut)
	}

	h.fake.Down(0)
	has(t, h.ok("strength", "sync"), "up to date")
	out = h.ok("habit")
	if strings.Contains(out, "⟳") || strings.Contains(out, "⚠") {
		t.Fatalf("after the sync:\n%s", out)
	}
	has(t, out, "2/6 bottles", "Creatine (3–5g) ✓")
	today := time.Now().In(london(t)).Format("2006-01-02")
	if got := h.fake.Value(strengthtest.SeedHabits()[0].ID, today); got != 2 {
		t.Fatalf("water after the replay: %v", got)
	}

	// The token is revoked in 99-Strength.
	h.fake.SetToken("")
	has(t, h.ok("strength", "sync"), "the token needs renewing", "tcc strength connect")
	has(t, h.ok("habit"), "⚠ 99-Strength token needs renewing: run `tcc strength connect`", "2/6 bottles")
	has(t, h.ok("status"), "⚠ 99-Strength token needs renewing")
	var st strength.Status
	json.Unmarshal([]byte(h.ok("strength", "status", "--json")), &st)
	if st.State != "auth" || !st.NeedsToken || !st.Connected {
		t.Fatalf("status --json: %+v", st)
	}

	// Renewing needs no --api: it reuses the address.
	renewed := "nns_pat_" + strings.Repeat("R", 43)
	h.fake.SetToken(renewed)
	code, out, errOut := h.runIn(renewed+"\n", "strength", "connect")
	if code != 0 {
		t.Fatalf("renew: %d %s%s", code, out, errOut)
	}
	has(t, h.ok("strength", "status"), "up to date")
}

func london(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
