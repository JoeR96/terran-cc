package timer

import (
	"encoding/json"
	"testing"
	"time"
)

type fake struct{ t time.Time }

func (f *fake) now() time.Time      { return f.t }
func (f *fake) add(d time.Duration) { f.t = f.t.Add(d) }
func newT(cfg Config) (*Timer, *fake) {
	f := &fake{t: time.Date(2026, 10, 17, 14, 0, 0, 0, time.UTC)}
	return New(cfg, f.now), f
}

func id(v int64) *int64 { return &v }

func TestDefaults(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	s := tm.Snapshot()
	if s.Phase != Focus || s.State != Idle || s.Remaining != 25*time.Minute || s.Cycle != 0 {
		t.Fatalf("%+v", s)
	}
	c := DefaultConfig()
	if c.Short != 5*time.Minute || c.Long != 15*time.Minute || c.LongEvery != 4 || c.AutoStart {
		t.Fatalf("config %+v", c)
	}
}

func TestStartCountsDown(t *testing.T) {
	tm, f := newT(DefaultConfig())
	tm.Start(nil)
	f.add(60 * time.Second)
	if s := tm.Snapshot(); s.State != Running || s.Remaining != 24*time.Minute {
		t.Fatalf("%+v", s)
	}
}

func TestPauseResume(t *testing.T) {
	tm, f := newT(DefaultConfig())
	tm.Start(nil)
	f.add(5 * time.Minute)
	tm.Pause()
	f.add(10 * time.Minute)
	if s := tm.Snapshot(); s.State != Paused || s.Remaining != 20*time.Minute {
		t.Fatalf("paused %+v", s)
	}
	tm.Resume()
	f.add(time.Minute)
	if s := tm.Snapshot(); s.Remaining != 19*time.Minute {
		t.Fatalf("resumed %+v", s)
	}
}

func TestPhaseEnd(t *testing.T) {
	tm, f := newT(DefaultConfig())
	tm.Start(nil)
	f.add(24 * time.Minute)
	if _, ended := tm.Advance(); ended {
		t.Fatal("ended early")
	}
	f.add(time.Minute)
	s, ended := tm.Advance()
	if !ended || s.Phase != ShortBreak || s.State != Idle || s.Cycle != 1 || s.Remaining != 5*time.Minute {
		t.Fatalf("%v %+v", ended, s)
	}
}

func TestLongBreakEvery4(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	var s Snapshot
	for i := 0; i < 4; i++ {
		s = tm.Skip() // focus → break
		if i < 3 && s.Phase != ShortBreak {
			t.Fatalf("cycle %d: %s", i+1, s.Phase)
		}
		if i < 3 {
			s = tm.Skip() // break → focus
		}
	}
	if s.Phase != LongBreak || s.Cycle != 4 || s.Remaining != 15*time.Minute {
		t.Fatalf("%+v", s)
	}
	if s = tm.Skip(); s.Phase != Focus {
		t.Fatalf("after long break: %+v", s)
	}
}

func TestAutoStart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStart = true
	tm, f := newT(cfg)
	tm.Start(nil)
	f.add(25 * time.Minute)
	s, _ := tm.Advance()
	if s.Phase != ShortBreak || s.State != Running {
		t.Fatalf("%+v", s)
	}
}

func TestToggle(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	for _, want := range []State{Running, Paused, Running} {
		if s := tm.Toggle(); s.State != want {
			t.Fatalf("want %s got %s", want, s.State)
		}
	}
}

func TestStopResets(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	tm.Start(id(7))
	tm.Skip()
	s := tm.Stop()
	if s.Phase != Focus || s.State != Idle || s.Cycle != 0 || s.ItemID != nil || s.Remaining != 25*time.Minute {
		t.Fatalf("%+v", s)
	}
}

func TestItemKept(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	tm.Start(id(7))
	s := tm.Skip()
	if s.ItemID == nil || *s.ItemID != 7 {
		t.Fatalf("%+v", s)
	}
	s = tm.Start(nil)
	if s.ItemID == nil || *s.ItemID != 7 {
		t.Fatalf("start(nil) cleared item: %+v", s)
	}
}

func TestSetConfigAppliesWhenIdle(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	cfg := DefaultConfig()
	cfg.Focus = 50 * time.Minute
	if s := tm.SetConfig(cfg); s.Remaining != 50*time.Minute {
		t.Fatalf("%+v", s)
	}
}

func TestSnapshotJSON(t *testing.T) {
	tm, _ := newT(DefaultConfig())
	b, _ := json.Marshal(tm.Snapshot())
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["remaining_s"] != float64(1500) || m["phase"] != "focus" || m["state"] != "idle" {
		t.Fatalf("%s", b)
	}
	cfg := m["config"].(map[string]any)
	if cfg["focus_min"] != float64(25) || cfg["long_every"] != float64(4) {
		t.Fatalf("%s", b)
	}
	var back Config
	if err := json.Unmarshal([]byte(`{"focus_min":30,"short_min":5,"long_min":20,"long_every":3}`), &back); err != nil ||
		back.Focus != 30*time.Minute || back.LongEvery != 3 {
		t.Fatalf("%+v %v", back, err)
	}
}

// A countdown started with a chosen length runs once, then goes idle: no
// break follows and no cycle is counted.
func TestCountdownRunsOnceThenIdles(t *testing.T) {
	tm, f := newT(DefaultConfig())
	tm.Start(id(3))
	s := tm.Countdown(10 * time.Minute)
	if s.State != Running || s.Remaining != 10*time.Minute || s.Total != 10*time.Minute || s.ItemID != nil {
		t.Fatalf("start: %+v", s)
	}
	f.add(10 * time.Minute)
	s, ended := tm.Advance()
	if !ended || s.State != Idle || s.Phase != Focus || s.Cycle != 0 {
		t.Fatalf("end: %v %+v", ended, s)
	}
	tm.Start(nil)
	f.add(25*time.Minute + time.Second)
	if s, _ := tm.Advance(); s.Phase != ShortBreak {
		t.Fatalf("a plain start after a countdown should be a normal pomodoro: %+v", s)
	}
}

func TestCountdownReplacesRunningTimer(t *testing.T) {
	tm, f := newT(DefaultConfig())
	tm.Countdown(45 * time.Minute)
	f.add(5 * time.Minute)
	if s := tm.Countdown(5 * time.Minute); s.State != Running || s.Remaining != 5*time.Minute {
		t.Fatalf("%+v", s)
	}
	if s := tm.Pause(); s.State != Paused || s.Total != 5*time.Minute {
		t.Fatalf("%+v", s)
	}
}
