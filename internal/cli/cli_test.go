package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
	"terrancc/internal/timer"
)

type harness struct {
	t *testing.T
	c *client.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	home := t.TempDir()
	db, err := sqlite.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	svc.SeedHabits(context.Background())
	srv := httptest.NewServer(api.New(svc, timer.New(timer.DefaultConfig(), time.Now), events.NewBus(), "tok", home))
	t.Cleanup(srv.Close)
	return &harness{t: t, c: client.New(srv.Listener.Addr().String(), "tok")}
}

// run executes tcc with args and returns exit code, stdout, stderr.
func (h *harness) run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	root := NewRoot(func() (*client.Client, error) { return h.c, nil }, &out, &errOut)
	code := Execute(root, args)
	return code, out.String(), errOut.String()
}

func (h *harness) ok(args ...string) string {
	h.t.Helper()
	code, out, errOut := h.run(args...)
	if code != 0 {
		h.t.Fatalf("tcc %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

func TestAddAndLs(t *testing.T) {
	h := newHarness(t)
	h.ok("project", "add", "BDTV Bonanza", "--repo", `C:\repo`)
	h.ok("add", "Fix bot melee attacks", "--project", "bdtv-bonanza", "--kind", "bug")
	h.ok("todo", "Book barber")
	out := h.ok("ls")
	if !strings.Contains(out, "#1") || !strings.Contains(out, "Fix bot melee attacks") || !strings.Contains(out, "[bdtv-bonanza]") ||
		!strings.Contains(out, "Book barber") {
		t.Fatalf("ls:\n%s", out)
	}
	out = h.ok("ls", "--personal")
	if strings.Contains(out, "melee") || !strings.Contains(out, "barber") {
		t.Fatalf("ls --personal:\n%s", out)
	}
	h.ok("done", "2")
	if out = h.ok("ls"); strings.Contains(out, "barber") {
		t.Fatalf("done item shown by default:\n%s", out)
	}
	if out = h.ok("ls", "--all"); !strings.Contains(out, "barber") {
		t.Fatalf("ls --all:\n%s", out)
	}
}

func TestTodoDefaultsToday(t *testing.T) {
	h := newHarness(t)
	var it model.Item
	json.Unmarshal([]byte(h.ok("todo", "Order creatine", "--json")), &it)
	if it.When != model.WhenToday || it.ProjectID != nil || it.Context != model.ContextIRL {
		t.Fatalf("%+v", it)
	}
	json.Unmarshal([]byte(h.ok("todo", "Reply to issue", "--context", "work", "--when", "week", "--json")), &it)
	if it.When != model.WhenWeek || it.Context != model.ContextWork {
		t.Fatalf("%+v", it)
	}
}

func TestJSONOutput(t *testing.T) {
	h := newHarness(t)
	h.ok("todo", "x")
	var items []model.Item
	if err := json.Unmarshal([]byte(h.ok("ls", "--json")), &items); err != nil || len(items) != 1 {
		t.Fatalf("%v %v", items, err)
	}
}

func TestUnknownItemExitCode(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run("show", "999")
	if code != 2 || !strings.Contains(errOut, "#999") {
		t.Fatalf("exit %d %q", code, errOut)
	}
	code, out, _ := h.run("show", "999", "--json")
	var m map[string]string
	if code != 2 || json.Unmarshal([]byte(out), &m) != nil || m["error"] == "" {
		t.Fatalf("exit %d %q", code, out)
	}
	if code, _, _ := h.run("add", "x", "--project", "nope"); code != 2 {
		t.Fatalf("unknown project exit %d", code)
	}
}

func TestAmbiguousHabit(t *testing.T) {
	h := newHarness(t)
	code, _, errOut := h.run("habit", "log", "wa")
	if code != 1 || !strings.Contains(errOut, "ambiguous") {
		t.Fatalf("exit %d %q", code, errOut)
	}
}

func TestHabitLogSyntax(t *testing.T) {
	h := newHarness(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"water"}, "250/3000 ml"},
		{[]string{"water", "+250"}, "500/3000 ml"},
		{[]string{"water", "-100"}, "400/3000 ml"},
		{[]string{"water", "=1800"}, "1800/3000 ml"},
		{[]string{"walks"}, "1/2"},
		{[]string{"creatine", "toggle"}, "✓"},
	} {
		out := h.ok(append([]string{"habit", "log"}, c.args...)...)
		if !strings.Contains(out, c.want) {
			t.Errorf("%v: %q want %q", c.args, out, c.want)
		}
	}
	var d model.HabitDay
	json.Unmarshal([]byte(h.ok("habit", "log", "protein", "+40", "--json")), &d)
	if d.Value != 40 {
		t.Fatalf("json log: %+v", d)
	}
	if code, _, _ := h.run("habit", "log", "water", "lots"); code != 1 {
		t.Fatalf("bad amount exit %d", code)
	}
}

func TestEditProjectEmptyMeansPersonal(t *testing.T) {
	h := newHarness(t)
	h.ok("project", "add", "P")
	h.ok("add", "x", "--project", "p")
	var it model.Item
	json.Unmarshal([]byte(h.ok("edit", "1", "--project", "", "--json")), &it)
	if it.ProjectID != nil {
		t.Fatalf("%+v", it)
	}
	json.Unmarshal([]byte(h.ok("edit", "1", "--model", "Plan: Fable 5.1 high. Build: Sonnet 5.5 medium.", "--depends-on", "", "--json")), &it)
	if it.Model == "" || len(it.DependsOn) != 0 {
		t.Fatalf("%+v", it)
	}
}

func TestStartReviewFlow(t *testing.T) {
	h := newHarness(t)
	h.ok("project", "add", "BDTV Bonanza")
	h.ok("add", "Module split", "--project", "bdtv-bonanza")
	h.ok("start", "1", "--branch", "feature/module-boundaries", "--worktree", `D:\bdb modules`, "--session", "bdb-t1", "--session-model", "claude-opus-5-5")
	h.ok("note", "1", "split", "Player", "first")
	h.ok("evidence", "1", "--commit", "7915ac6", "--commit", "0118c99", "--pr", "#57", "--test", "go test ./...")
	h.ok("review", "1")
	var it model.Item
	json.Unmarshal([]byte(h.ok("show", "1", "--json")), &it)
	if it.Status != model.StatusReview || it.Worktree != `D:\bdb modules` || len(it.Evidence) != 4 ||
		!strings.Contains(it.Notes, "split Player first") {
		t.Fatalf("%+v", it)
	}
	out := h.ok("show", "1")
	if !strings.Contains(out, "7915ac6") || !strings.Contains(out, "review") {
		t.Fatalf("show:\n%s", out)
	}
	if out = h.ok("project", "show", "bdtv-bonanza"); !strings.Contains(out, "review") {
		t.Fatalf("project show:\n%s", out)
	}
}

func TestHashID(t *testing.T) {
	h := newHarness(t)
	h.ok("todo", "x")
	if out := h.ok("show", "#1"); !strings.Contains(out, "x") {
		t.Fatal(out)
	}
}

func TestNextOrdered(t *testing.T) {
	h := newHarness(t)
	h.ok("project", "add", "P")
	h.ok("add", "second", "--project", "p")
	h.ok("add", "first", "--project", "p")
	h.ok("edit", "1", "--status", "up_next", "--position", "2")
	h.ok("edit", "2", "--status", "up_next", "--position", "1", "--why-first", "biggest change")
	out := h.ok("next", "--project", "p")
	if strings.Index(out, "first") > strings.Index(out, "second") || !strings.Contains(out, "biggest change") {
		t.Fatalf("next:\n%s", out)
	}
}

func TestTimerAndStatus(t *testing.T) {
	h := newHarness(t)
	h.ok("todo", "Fix stairs")
	if out := h.ok("timer", "start", "--item", "1"); !strings.Contains(out, "running") || !strings.Contains(out, "Fix stairs") {
		t.Fatalf("timer start: %q", out)
	}
	if out := h.ok("timer", "toggle"); !strings.Contains(out, "paused") {
		t.Fatalf("timer toggle: %q", out)
	}
	if out := h.ok("timer", "config", "--focus", "50"); !strings.Contains(out, "50") {
		t.Fatalf("timer config: %q", out)
	}
	out := h.ok("status")
	for _, want := range []string{"Walks 0/2", "Water 0/3000 ml", "Fix stairs", "streak"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}
