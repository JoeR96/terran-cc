package strength

import (
	"context"
	"strings"
	"testing"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/strength/strengthtest"
)

// liveEnv is a connected service with fast live-sync timings.
func liveEnv(t *testing.T, o Options) *env {
	t.Helper()
	e := newEnv(t)
	if o.PollEvery == 0 {
		o.PollEvery = 40 * time.Millisecond
	}
	if o.RetryMin == 0 {
		o.RetryMin = 20 * time.Millisecond
	}
	e.svc = e.open(o)
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) goLive() {
	ctx, cancel := context.WithCancel(bg)
	e.t.Cleanup(cancel)
	go e.svc.Live(ctx)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// webLog is the web app logging a habit: straight into the fake.
func (e *env) webLog(id string, v float64) {
	e.fake.Put(id, fixDate, v, time.Date(2026, 11, 9, 8, 30, 0, 0, time.UTC))
}

func (e *env) value(name string) float64 { return e.habit(name).Value }

func TestAWebWriteArrivesOverTheEventStream(t *testing.T) {
	e := liveEnv(t, Options{})
	e.goLive()
	eventually(t, "the stream", func() bool { return e.svc.LiveMode() == "stream" && e.fake.Streams() == 1 })

	before := e.notes.Load()
	e.webLog(waterID, 3)
	eventually(t, "water from the web app", func() bool { return e.value("Water") == 3 })
	if e.notes.Load() == before {
		t.Fatal("habit.changed was not published")
	}
}

func TestPollingWhenTheServerHasNoEventStream(t *testing.T) {
	e := liveEnv(t, Options{})
	e.fake.NoStream(true, false)
	e.goLive()
	eventually(t, "polling", func() bool { return e.svc.LiveMode() == "poll" })

	e.webLog(waterID, 4)
	eventually(t, "water by polling", func() bool { return e.value("Water") == 4 })

	// Unchanged versions are 304s.
	eventually(t, "a 304 from /sync/version", func() bool {
		for _, r := range e.fake.Requests() {
			if strings.HasPrefix(r.Path, "/api/v1/sync/version") && r.Status == 304 {
				return true
			}
		}
		return false
	})
}

func TestNoSyncRoutesAtAllLeavesTheRefresh(t *testing.T) {
	e := liveEnv(t, Options{})
	e.fake.NoStream(true, true)
	e.goLive()
	eventually(t, "off", func() bool { return e.svc.LiveMode() == "off" })
}

func TestTheStreamReconnectsAndCatchesUp(t *testing.T) {
	e := liveEnv(t, Options{})
	e.goLive()
	eventually(t, "the stream", func() bool { return e.fake.Streams() == 1 })
	e.webLog(waterID, 1)
	eventually(t, "first change", func() bool { return e.value("Water") == 1 })

	e.fake.DropStreams()
	e.webLog(waterID, 5) // while the stream is down
	eventually(t, "a second stream", func() bool {
		n := 0
		for _, r := range e.fake.Requests() {
			if r.Path == "/api/v1/sync/events" {
				n++
			}
		}
		return n >= 1 && e.fake.Streams() == 1
	})
	eventually(t, "the missed change", func() bool { return e.value("Water") == 5 })
}

func TestAStreamThatGoesQuietIsReplaced(t *testing.T) {
	e := liveEnv(t, Options{StreamIdle: 150 * time.Millisecond})
	e.fake.Keepalive(time.Hour) // no keepalives: the client must notice
	e.goLive()
	eventually(t, "the stream", func() bool { return e.fake.Streams() == 1 })
	eventually(t, "a reconnect", func() bool {
		n := 0
		for _, r := range e.fake.Requests() {
			if r.Path == "/api/v1/sync/events" {
				n++
			}
		}
		return n >= 2
	})
}

func TestKeepalivesKeepAStreamOpen(t *testing.T) {
	e := liveEnv(t, Options{StreamIdle: 150 * time.Millisecond})
	e.fake.Keepalive(30 * time.Millisecond)
	e.goLive()
	eventually(t, "the stream", func() bool { return e.fake.Streams() == 1 })
	time.Sleep(400 * time.Millisecond)
	for _, r := range e.fake.Requests() {
		if r.Path == "/api/v1/sync/events" {
			t.Fatal("a stream with keepalives was dropped")
		}
	}
}

func TestAPlanChangeOverTheStreamRefreshesTraining(t *testing.T) {
	e := liveEnv(t, Options{})
	e.fake.SetPlan(fixture(t, "plans_upcoming.json"))
	e.setClock(time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	if _, err := e.svc.Training(bg, "", 0); err != nil {
		t.Fatal(err)
	}
	e.goLive()
	eventually(t, "the stream", func() bool { return e.fake.Streams() == 1 })
	before := e.tnotes.Load()

	e.fake.SetPlan(strengthtest.SeedPlan(time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)))
	eventually(t, "training.changed", func() bool { return e.tnotes.Load() > before })
	tr, err := e.svc.Training(bg, "", 0)
	if err != nil || len(tr.Days) != 7 {
		t.Fatalf("the new plan was not cached: %d days, %v", len(tr.Days), err)
	}
}

func TestDisconnectStopsTheStream(t *testing.T) {
	e := liveEnv(t, Options{})
	e.goLive()
	eventually(t, "the stream", func() bool { return e.fake.Streams() == 1 })
	if _, err := e.svc.Disconnect(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the stream to close", func() bool { return e.fake.Streams() == 0 && e.svc.LiveMode() == "" })
}

func TestARevokedTokenStopsTheStream(t *testing.T) {
	e := liveEnv(t, Options{})
	e.fake.SetToken("")
	e.goLive()
	eventually(t, "needs token", func() bool { return e.svc.Status().NeedsToken })
	if n := e.fake.Streams(); n != 0 {
		t.Fatalf("%d streams open", n)
	}
}

// The bug this fixes: water logged on the web, then a walk added in
// Terran CC, showed the walk but not the water.
func TestAWriteReadsEveryHabitAgain(t *testing.T) {
	e := newEnv(t)
	e.svc = e.open(Options{LogWait: 2 * time.Second})
	if _, err := e.svc.Connect(bg, e.base(), strengthtest.Token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	go e.svc.Run(ctx)

	e.webLog(waterID, 3)
	before := e.notes.Load()
	if d := e.log("walks", core.LogOp{}); d.Value != 1 || d.Pending != 0 {
		t.Fatalf("walk: %+v", d)
	}
	if v := e.value("Water"); v != 3 {
		t.Fatalf("after adding a walk, water shows %v, want the web app's 3", v)
	}
	if e.notes.Load() == before {
		t.Fatal("habit.changed was not published")
	}
}

func TestAnUnchangedReadIsA304AndFreshensTheCache(t *testing.T) {
	e := connected(t)
	first := *e.svc.Status().LastSync
	e.svc.Sync(bg)
	reqs := e.fake.Requests()
	last := reqs[len(reqs)-1]
	if !strings.HasPrefix(last.Path, "/api/v1/habits/today") || last.Status != 304 {
		t.Fatalf("second read: %s %d, want a 304", last.Path, last.Status)
	}
	if got := *e.svc.Status().LastSync; !got.After(first) {
		t.Fatal("a 304 did not freshen the cache")
	}
	if len(e.day().Habits) != 5 {
		t.Fatal("a 304 lost the habits")
	}

	// A change on the server is a 200 with the new value.
	e.webLog(waterID, 2)
	e.svc.Sync(bg)
	if v := e.value("Water"); v != 2 {
		t.Fatalf("water %v after a change", v)
	}
}

func TestAnUnchangedPlanIsA304(t *testing.T) {
	e := trainingEnv(t)
	if _, err := e.svc.Training(bg, "", 0); err != nil {
		t.Fatal(err)
	}
	e.setClock(e.clock().Add(3 * time.Minute))
	tr, err := e.svc.Training(bg, "", 0)
	if err != nil || len(tr.Days) != 4 || tr.Sync.State != "ok" {
		t.Fatalf("after a 304: %+v %v", tr, err)
	}
	reqs := e.fake.Requests()
	if last := reqs[len(reqs)-1]; last.Status != 304 {
		t.Fatalf("second plan read answered %d, want 304", last.Status)
	}
	// Fresh again: the next ask is from the cache.
	n := e.planCalls()
	e.svc.Training(bg, "", 0)
	if e.planCalls() != n {
		t.Fatal("a 304 did not freshen the plan")
	}
	if e.tnotes.Load() != 1 {
		t.Fatalf("training.changed %d times, want once (the first read)", e.tnotes.Load())
	}
}
