package core

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"terrancc/internal/model"
	"terrancc/internal/store"
)

const dateFormat = "2006-01-02"

// Today is the current local date. Habit days reset at local midnight.
func (s *Service) Today() string { return s.Clock.Now().Local().Format(dateFormat) }

func target(h model.Habit) float64 {
	if h.Type == model.HabitCheck {
		return 1
	}
	return h.Target
}

func (s *Service) HabitsFor(ctx context.Context, date string) ([]model.HabitDay, error) {
	hs, err := s.Store.ListHabits(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]model.HabitDay, 0, len(hs))
	for _, h := range hs {
		v, err := s.Store.GetHabitValue(ctx, h.ID, date)
		if err != nil {
			return nil, err
		}
		out = append(out, model.HabitDay{Habit: h, Date: date, Value: v, Hit: v >= target(h)})
	}
	return out, nil
}

// LogOp changes today's value: add Delta, Set it outright, or Toggle a check
// habit. An empty op adds one Step (or toggles a check habit).
type LogOp struct {
	Delta  *float64 `json:"delta,omitempty"`
	Set    *float64 `json:"set,omitempty"`
	Toggle bool     `json:"toggle,omitempty"`
}

func (s *Service) LogHabit(ctx context.Context, habitID int64, op LogOp) (model.HabitDay, error) {
	var day model.HabitDay
	date := s.Today()
	err := s.Store.WithTx(ctx, func(st store.Store) error {
		h, err := st.GetHabit(ctx, habitID)
		if err != nil {
			return err
		}
		v, err := st.GetHabitValue(ctx, h.ID, date)
		if err != nil {
			return err
		}
		switch {
		case op.Set != nil:
			v = *op.Set
		case op.Delta != nil:
			v += *op.Delta
		case op.Toggle || h.Type == model.HabitCheck:
			v = 1 - math.Min(v, 1)
		default:
			v += h.Step
		}
		v = math.Max(v, 0)
		if h.Type == model.HabitCheck {
			v = math.Min(v, 1)
		}
		if err := st.SetHabitValue(ctx, h.ID, date, v); err != nil {
			return err
		}
		day = model.HabitDay{Habit: h, Date: date, Value: v, Hit: v >= target(h)}
		return nil
	})
	if err == nil {
		s.changed("habit", habitID)
	}
	return day, err
}

// FindHabit accepts an id, an exact name or a unique name prefix (any case).
func (s *Service) FindHabit(ctx context.Context, ref string) (model.Habit, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return s.Store.GetHabit(ctx, id)
	}
	hs, err := s.Store.ListHabits(ctx, false)
	if err != nil {
		return model.Habit{}, err
	}
	low := strings.ToLower(ref)
	var matches []model.Habit
	for _, h := range hs {
		name := strings.ToLower(h.Name)
		if name == low {
			return h, nil
		}
		if strings.HasPrefix(name, low) {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 0:
		return model.Habit{}, errors.Join(store.ErrNotFound, errors.New("no habit matches "+strconv.Quote(ref)))
	case 1:
		return matches[0], nil
	}
	names := make([]string, len(matches))
	for i, h := range matches {
		names[i] = h.Name
	}
	return model.Habit{}, invalid("ambiguous habit %q: %s", ref, strings.Join(names, ", "))
}

func validateHabit(h *model.Habit) error {
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" {
		return invalid("habit name is required")
	}
	if !h.Type.Valid() {
		return invalid("habit type must be count|amount|check")
	}
	if h.Type == model.HabitCheck {
		h.Target, h.Step = 1, 1
	}
	if h.Target <= 0 {
		return invalid("habit target must be above 0")
	}
	if h.Step <= 0 {
		h.Step = 1
	}
	return nil
}

func (s *Service) AddHabit(ctx context.Context, h model.Habit) (model.Habit, error) {
	if err := validateHabit(&h); err != nil {
		return h, err
	}
	h.CreatedAt = s.Clock.Now()
	if h.Order == 0 {
		all, err := s.Store.ListHabits(ctx, true)
		if err != nil {
			return h, err
		}
		h.Order = len(all) + 1
	}
	if err := s.Store.CreateHabit(ctx, &h); err != nil {
		return h, err
	}
	s.changed("habit", h.ID)
	return h, nil
}

type HabitPatch struct {
	Name, Icon, Unit *string
	Target, Step     *float64
	Order            *int
	Archived         *bool
}

func (s *Service) EditHabit(ctx context.Context, id int64, p HabitPatch) (model.Habit, error) {
	h, err := s.Store.GetHabit(ctx, id)
	if err != nil {
		return h, err
	}
	if p.Name != nil {
		h.Name = *p.Name
	}
	if p.Icon != nil {
		h.Icon = *p.Icon
	}
	if p.Unit != nil {
		h.Unit = *p.Unit
	}
	if p.Target != nil {
		h.Target = *p.Target
	}
	if p.Step != nil {
		h.Step = *p.Step
	}
	if p.Order != nil {
		h.Order = *p.Order
	}
	if p.Archived != nil {
		h.Archived = *p.Archived
	}
	if err := validateHabit(&h); err != nil {
		return h, err
	}
	if err := s.Store.UpdateHabit(ctx, h); err != nil {
		return h, err
	}
	s.changed("habit", h.ID)
	return h, nil
}

// Streak counts consecutive days on which every habit that existed that day
// hit its current target. An unfinished today never breaks the streak.
func (s *Service) Streak(ctx context.Context) (int, error) {
	hs, err := s.Store.ListHabits(ctx, false)
	if err != nil || len(hs) == 0 {
		return 0, err
	}
	now := s.Clock.Now().Local()
	const maxDays = 3650
	values, err := s.Store.HabitValuesSince(ctx, now.AddDate(0, 0, -maxDays).Format(dateFormat))
	if err != nil {
		return 0, err
	}
	full := func(date string) (bool, bool) { // (complete, any habit existed)
		existed := false
		for _, h := range hs {
			if h.CreatedAt.Local().Format(dateFormat) > date {
				continue
			}
			existed = true
			if values[date][h.ID] < target(h) {
				return false, true
			}
		}
		return existed, existed
	}
	streak := 0
	for i := 0; i < maxDays; i++ {
		date := now.AddDate(0, 0, -i).Format(dateFormat)
		ok, existed := full(date)
		if !existed {
			break
		}
		if !ok {
			if i == 0 {
				continue // today is still in progress
			}
			break
		}
		streak++
	}
	return streak, nil
}

// SeedHabits creates Joe's four daily habits the first time the service runs.
func (s *Service) SeedHabits(ctx context.Context) error {
	all, err := s.Store.ListHabits(ctx, true)
	if err != nil || len(all) > 0 {
		return err
	}
	for i, h := range []model.Habit{
		{Name: "Walks", Icon: "🚶", Type: model.HabitCount, Target: 2, Step: 1},
		{Name: "Water", Icon: "💧", Unit: "ml", Type: model.HabitAmount, Target: 3000, Step: 250},
		{Name: "Protein", Icon: "🥩", Unit: "g", Type: model.HabitAmount, Target: 180, Step: 20},
		{Name: "Creatine + supplements", Icon: "💊", Type: model.HabitCheck, Target: 1, Step: 1},
	} {
		h.Order = i + 1
		if _, err := s.AddHabit(ctx, h); err != nil {
			return err
		}
	}
	return nil
}
