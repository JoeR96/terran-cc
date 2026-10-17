// Package timer is the Pomodoro state machine. It lives in memory only: the
// timer counts down and forgets, so nothing here is persisted except Config.
package timer

import (
	"encoding/json"
	"sync"
	"time"
)

type Phase string

const (
	Focus      Phase = "focus"
	ShortBreak Phase = "short_break"
	LongBreak  Phase = "long_break"
)

type State string

const (
	Idle    State = "idle"
	Running State = "running"
	Paused  State = "paused"
)

type Config struct {
	Focus, Short, Long time.Duration
	LongEvery          int
	AutoStart          bool
}

func DefaultConfig() Config {
	return Config{Focus: 25 * time.Minute, Short: 5 * time.Minute, Long: 15 * time.Minute, LongEvery: 4}
}

type configJSON struct {
	FocusMin  float64 `json:"focus_min"`
	ShortMin  float64 `json:"short_min"`
	LongMin   float64 `json:"long_min"`
	LongEvery int     `json:"long_every"`
	AutoStart bool    `json:"auto_start"`
}

func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(configJSON{c.Focus.Minutes(), c.Short.Minutes(), c.Long.Minutes(), c.LongEvery, c.AutoStart})
}

func (c *Config) UnmarshalJSON(b []byte) error {
	var j configJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
	*c = Config{min(j.FocusMin), min(j.ShortMin), min(j.LongMin), j.LongEvery, j.AutoStart}
	return nil
}

// normalized replaces missing or nonsensical values with the defaults.
func (c Config) normalized() Config {
	d := DefaultConfig()
	if c.Focus <= 0 {
		c.Focus = d.Focus
	}
	if c.Short <= 0 {
		c.Short = d.Short
	}
	if c.Long <= 0 {
		c.Long = d.Long
	}
	if c.LongEvery < 1 {
		c.LongEvery = d.LongEvery
	}
	return c
}

type Snapshot struct {
	Phase     Phase
	State     State
	Remaining time.Duration
	Cycle     int // completed focus sessions since the last Stop
	ItemID    *int64
	Config    Config
}

func (s Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Phase     Phase  `json:"phase"`
		State     State  `json:"state"`
		Remaining int    `json:"remaining_s"`
		Cycle     int    `json:"cycle"`
		ItemID    *int64 `json:"item_id"`
		Config    Config `json:"config"`
	}{s.Phase, s.State, int((s.Remaining + time.Second - 1) / time.Second), s.Cycle, s.ItemID, s.Config})
}

type Timer struct {
	mu        sync.Mutex
	now       func() time.Time
	cfg       Config
	phase     Phase
	state     State
	remaining time.Duration // valid when idle or paused
	end       time.Time     // valid when running
	cycle     int
	item      *int64
}

func New(cfg Config, now func() time.Time) *Timer {
	t := &Timer{now: now, cfg: cfg.normalized(), phase: Focus, state: Idle}
	t.remaining = t.duration(Focus)
	return t
}

func (t *Timer) duration(p Phase) time.Duration {
	switch p {
	case ShortBreak:
		return t.cfg.Short
	case LongBreak:
		return t.cfg.Long
	}
	return t.cfg.Focus
}

func (t *Timer) left() time.Duration {
	if t.state != Running {
		return t.remaining
	}
	if d := t.end.Sub(t.now()); d > 0 {
		return d
	}
	return 0
}

func (t *Timer) snap() Snapshot {
	return Snapshot{Phase: t.phase, State: t.state, Remaining: t.left(), Cycle: t.cycle, ItemID: t.item, Config: t.cfg}
}

func (t *Timer) run() {
	t.end = t.now().Add(t.remaining)
	t.state = Running
}

func (t *Timer) locked(fn func()) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn()
	return t.snap()
}

func (t *Timer) Snapshot() Snapshot { return t.locked(func() {}) }

// Start runs the current phase. A non-nil itemID sets the focus target.
func (t *Timer) Start(itemID *int64) Snapshot {
	return t.locked(func() {
		if itemID != nil {
			t.item = itemID
		}
		if t.state != Running {
			t.run()
		}
	})
}

func (t *Timer) Pause() Snapshot {
	return t.locked(func() {
		if t.state == Running {
			t.remaining, t.state = t.left(), Paused
		}
	})
}

func (t *Timer) Resume() Snapshot {
	return t.locked(func() {
		if t.state == Paused {
			t.run()
		}
	})
}

func (t *Timer) Toggle() Snapshot {
	return t.locked(func() {
		switch t.state {
		case Running:
			t.remaining, t.state = t.left(), Paused
		default:
			t.run()
		}
	})
}

// next ends the current phase and moves to the following one.
func (t *Timer) next() {
	if t.phase == Focus {
		t.cycle++
		t.phase = ShortBreak
		if t.cycle%t.cfg.LongEvery == 0 {
			t.phase = LongBreak
		}
	} else {
		t.phase = Focus
	}
	t.remaining, t.state = t.duration(t.phase), Idle
	if t.cfg.AutoStart {
		t.run()
	}
}

func (t *Timer) Skip() Snapshot { return t.locked(t.next) }

func (t *Timer) Stop() Snapshot {
	return t.locked(func() {
		t.phase, t.state, t.cycle, t.item = Focus, Idle, 0, nil
		t.remaining = t.duration(Focus)
	})
}

// SetConfig applies at the next phase, or immediately if the timer is idle.
func (t *Timer) SetConfig(cfg Config) Snapshot {
	return t.locked(func() {
		t.cfg = cfg.normalized()
		if t.state == Idle {
			t.remaining = t.duration(t.phase)
		}
	})
}

// Advance is called every second; it reports true when a running phase ended.
func (t *Timer) Advance() (Snapshot, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == Running && t.left() == 0 {
		t.next()
		return t.snap(), true
	}
	return t.snap(), false
}
