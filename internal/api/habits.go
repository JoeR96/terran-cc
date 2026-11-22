package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/model"
)

// errRemoteHabits answers attempts to add or edit a habit while 99-Strength
// owns them. The local habits are still in the database, just not in use.
var errRemoteHabits = fmt.Errorf("%w: habits come from 99-Strength while it is connected; add and edit them there, or run \"tcc strength disconnect\" to go back to local habits", core.ErrInvalid)

func (s *server) habitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/habits", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date != "" {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				writeErr(w, badRequest("date must be YYYY-MM-DD"))
				return
			}
		}
		if s.remoteHabits() {
			// Today comes from the cache at once; another date is read live.
			ctx, cancel := context.WithTimeout(r.Context(), strengthTimeout)
			defer cancel()
			day, err := s.strength.Habits(ctx, date)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, 200, day)
			return
		}
		if date == "" {
			date = s.svc.Today()
		}
		days, err := s.svc.HabitsFor(r.Context(), date)
		if err != nil {
			writeErr(w, err)
			return
		}
		for i := range days {
			days[i].Source = model.SourceLocal
		}
		streak, err := s.svc.Streak(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"date": date, "streak": streak, "habits": days, "source": model.SourceLocal})
	})
	mux.HandleFunc("POST /api/habits", func(w http.ResponseWriter, r *http.Request) {
		if s.remoteHabits() {
			writeErr(w, errRemoteHabits)
			return
		}
		var h model.Habit
		if err := decode(r, &h); err != nil {
			writeErr(w, err)
			return
		}
		h, err := s.svc.AddHabit(r.Context(), h)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, h)
	})
	mux.HandleFunc("PATCH /api/habits/{ref}", func(w http.ResponseWriter, r *http.Request) {
		if s.remoteHabits() {
			writeErr(w, errRemoteHabits)
			return
		}
		h, err := s.svc.FindHabit(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, err)
			return
		}
		var p core.HabitPatch
		if err := decode(r, &p); err != nil {
			writeErr(w, err)
			return
		}
		h, err = s.svc.EditHabit(r.Context(), h.ID, p)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, h)
	})
	mux.HandleFunc("POST /api/habits/{ref}/log", func(w http.ResponseWriter, r *http.Request) {
		var op core.LogOp
		if s.remoteHabits() {
			if err := decode(r, &op); err != nil {
				writeErr(w, err)
				return
			}
			// Queued and saved first; this never waits long on 99-Strength.
			day, err := s.strength.Log(r.Context(), r.PathValue("ref"), op)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, 200, day)
			return
		}
		h, err := s.svc.FindHabit(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := decode(r, &op); err != nil {
			writeErr(w, err)
			return
		}
		day, err := s.svc.LogHabit(r.Context(), h.ID, op)
		if err != nil {
			writeErr(w, err)
			return
		}
		day.Source = model.SourceLocal
		writeJSON(w, 200, day)
	})
}
