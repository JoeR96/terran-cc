package api

import (
	"net/http"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/model"
)

func (s *server) habitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/habits", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date == "" {
			date = s.svc.Today()
		} else if _, err := time.Parse("2006-01-02", date); err != nil {
			writeErr(w, badRequest("date must be YYYY-MM-DD"))
			return
		}
		days, err := s.svc.HabitsFor(r.Context(), date)
		if err != nil {
			writeErr(w, err)
			return
		}
		streak, err := s.svc.Streak(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"date": date, "streak": streak, "habits": days})
	})
	mux.HandleFunc("POST /api/habits", func(w http.ResponseWriter, r *http.Request) {
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
		h, err := s.svc.FindHabit(r.Context(), r.PathValue("ref"))
		if err != nil {
			writeErr(w, err)
			return
		}
		var op core.LogOp
		if err := decode(r, &op); err != nil {
			writeErr(w, err)
			return
		}
		day, err := s.svc.LogHabit(r.Context(), h.ID, op)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, day)
	})
}
