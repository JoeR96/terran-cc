package core

import (
	"errors"
	"testing"
	"time"

	"terrancc/internal/model"
	"terrancc/internal/store"
)

func f(v float64) *float64 { return &v }

// seeded returns a service whose habits were seeded 10 days before the clock.
func seeded(t *testing.T) (*Service, *FixedClock, map[string]model.Habit) {
	t.Helper()
	s, clk := newSvc(t)
	now := clk.T
	clk.T = now.AddDate(0, 0, -10)
	if err := s.SeedHabits(ctx); err != nil {
		t.Fatal(err)
	}
	clk.T = now
	hs, _ := s.Store.ListHabits(ctx, false)
	byName := map[string]model.Habit{}
	for _, h := range hs {
		byName[h.Name] = h
	}
	return s, clk, byName
}

// fillDay hits every habit target on the clock's current day.
func fillDay(t *testing.T, s *Service, hs map[string]model.Habit) {
	t.Helper()
	for _, h := range hs {
		if _, err := s.LogHabit(ctx, h.ID, LogOp{Set: f(h.Target)}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeedOnce(t *testing.T) {
	s, _, hs := seeded(t)
	if len(hs) != 4 {
		t.Fatalf("seeded %d habits", len(hs))
	}
	w := hs["Water"]
	if w.Type != model.HabitAmount || w.Target != 3000 || w.Step != 250 || w.Unit != "ml" || w.Icon != "💧" {
		t.Fatalf("water: %+v", w)
	}
	if hs["Walks"].Target != 2 || hs["Protein"].Target != 180 || hs["Protein"].Step != 20 ||
		hs["Creatine + supplements"].Type != model.HabitCheck {
		t.Fatalf("seed values: %+v", hs)
	}
	s.SeedHabits(ctx)
	if all, _ := s.Store.ListHabits(ctx, true); len(all) != 4 {
		t.Fatalf("reseeded: %d", len(all))
	}
}

func TestLogDeltaDefaultStep(t *testing.T) {
	s, _, hs := seeded(t)
	d, err := s.LogHabit(ctx, hs["Water"].ID, LogOp{})
	if err != nil || d.Value != 250 || d.Hit {
		t.Fatalf("%+v %v", d, err)
	}
	d, _ = s.LogHabit(ctx, hs["Water"].ID, LogOp{Delta: f(2750)})
	if d.Value != 3000 || !d.Hit {
		t.Fatalf("hit: %+v", d)
	}
}

func TestLogClamp(t *testing.T) {
	s, _, hs := seeded(t)
	s.LogHabit(ctx, hs["Water"].ID, LogOp{Delta: f(250)})
	d, _ := s.LogHabit(ctx, hs["Water"].ID, LogOp{Delta: f(-500)})
	if d.Value != 0 {
		t.Fatalf("got %v", d.Value)
	}
	if _, err := s.LogHabit(ctx, hs["Water"].ID, LogOp{Set: f(-1)}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckToggle(t *testing.T) {
	s, _, hs := seeded(t)
	id := hs["Creatine + supplements"].ID
	d, _ := s.LogHabit(ctx, id, LogOp{})
	if d.Value != 1 || !d.Hit {
		t.Fatalf("toggle on: %+v", d)
	}
	d, _ = s.LogHabit(ctx, id, LogOp{Toggle: true})
	if d.Value != 0 || d.Hit {
		t.Fatalf("toggle off: %+v", d)
	}
	d, _ = s.LogHabit(ctx, id, LogOp{Delta: f(5)})
	if d.Value != 1 {
		t.Fatalf("clamped to 1: %+v", d)
	}
}

func TestLogUsesLocalDate(t *testing.T) {
	s, clk, hs := seeded(t)
	clk.T = time.Date(2026, 10, 17, 23, 59, 0, 0, time.Local)
	s.LogHabit(ctx, hs["Water"].ID, LogOp{})
	clk.T = time.Date(2026, 10, 18, 0, 1, 0, 0, time.Local)
	today, _ := s.HabitsFor(ctx, s.Today())
	for _, d := range today {
		if d.Value != 0 || d.Date != "2026-10-18" {
			t.Fatalf("new day not empty: %+v", d)
		}
	}
	if v, _ := s.Store.GetHabitValue(ctx, hs["Water"].ID, "2026-10-17"); v != 250 {
		t.Fatalf("yesterday: %v", v)
	}
}

func TestStreak(t *testing.T) {
	s, clk, hs := seeded(t)
	now := clk.T
	for i := 3; i >= 0; i-- {
		clk.T = now.AddDate(0, 0, -i)
		fillDay(t, s, hs)
	}
	clk.T = now
	if n, _ := s.Streak(ctx); n != 4 {
		t.Fatalf("streak %d want 4", n)
	}
}

func TestStreakTodayIncomplete(t *testing.T) {
	s, clk, hs := seeded(t)
	now := clk.T
	for i := 3; i >= 1; i-- {
		clk.T = now.AddDate(0, 0, -i)
		fillDay(t, s, hs)
	}
	clk.T = now
	s.LogHabit(ctx, hs["Water"].ID, LogOp{})
	if n, _ := s.Streak(ctx); n != 3 {
		t.Fatalf("streak %d want 3", n)
	}
}

func TestStreakBreaks(t *testing.T) {
	s, clk, hs := seeded(t)
	now := clk.T
	for _, i := range []int{4, 3, 1} { // day -2 missed
		clk.T = now.AddDate(0, 0, -i)
		fillDay(t, s, hs)
	}
	clk.T = now
	if n, _ := s.Streak(ctx); n != 1 {
		t.Fatalf("streak %d want 1", n)
	}
}

func TestStreakIgnoresNewHabit(t *testing.T) {
	s, clk, hs := seeded(t)
	now := clk.T
	for i := 2; i >= 1; i-- {
		clk.T = now.AddDate(0, 0, -i)
		fillDay(t, s, hs)
	}
	clk.T = now
	if _, err := s.AddHabit(ctx, model.Habit{Name: "Guitar", Type: model.HabitCheck}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Streak(ctx); n != 2 {
		t.Fatalf("streak %d want 2", n)
	}
}

func TestFindHabit(t *testing.T) {
	s, _, hs := seeded(t)
	if _, err := s.FindHabit(ctx, "wa"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wa: %v", err)
	}
	if h, err := s.FindHabit(ctx, "wat"); err != nil || h.Name != "Water" {
		t.Fatalf("wat: %+v %v", h, err)
	}
	if h, err := s.FindHabit(ctx, "PROTEIN"); err != nil || h.Name != "Protein" {
		t.Fatalf("PROTEIN: %+v %v", h, err)
	}
	if h, err := s.FindHabit(ctx, "3"); err != nil || h.ID != 3 {
		t.Fatalf("3: %+v %v", h, err)
	}
	if _, err := s.FindHabit(ctx, "zzz"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("zzz: %v", err)
	}
	_ = hs
}

func TestAddHabitValidation(t *testing.T) {
	s, _ := newSvc(t)
	if _, err := s.AddHabit(ctx, model.Habit{Name: "x", Type: "streak"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad type: %v", err)
	}
	if _, err := s.AddHabit(ctx, model.Habit{Name: "x", Type: model.HabitAmount}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("amount without target: %v", err)
	}
	h, err := s.AddHabit(ctx, model.Habit{Name: "Read", Type: model.HabitCount, Target: 1})
	if err != nil || h.Step != 1 {
		t.Fatalf("default step: %+v %v", h, err)
	}
}
