package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"terrancc/internal/events"
	"terrancc/internal/timer"
)

func (s *server) timerRoutes(mux *http.ServeMux) {
	reply := func(w http.ResponseWriter, snap timer.Snapshot) {
		s.bus.Publish(events.Event{Type: "timer.tick", Data: snap})
		writeJSON(w, 200, snap)
	}
	mux.HandleFunc("GET /api/timer", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.timer.Snapshot())
	})
	mux.HandleFunc("POST /api/timer/start", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ItemID  *int64   `json:"item_id"`
			Minutes *float64 `json:"minutes"` // a one-off countdown instead of the pomodoro cycle
			Label   string   `json:"label"`   // the task a countdown is for
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if body.Minutes != nil {
			if m := *body.Minutes; m < 1 || m > 12*60 {
				writeErr(w, badRequest("minutes must be between 1 and 720"))
				return
			}
			reply(w, s.timer.Countdown(time.Duration(*body.Minutes*float64(time.Minute)), strings.TrimSpace(body.Label)))
			return
		}
		if body.ItemID != nil {
			if _, err := s.svc.Store.GetItem(r.Context(), *body.ItemID); err != nil {
				writeErr(w, itemErr(err, *body.ItemID))
				return
			}
		}
		reply(w, s.timer.Start(body.ItemID))
	})
	for name, fn := range map[string]func() timer.Snapshot{
		"pause": s.timer.Pause, "resume": s.timer.Resume, "toggle": s.timer.Toggle,
		"skip": s.timer.Skip, "stop": s.timer.Stop,
	} {
		mux.HandleFunc("POST /api/timer/"+name, func(w http.ResponseWriter, r *http.Request) { reply(w, fn()) })
	}
	mux.HandleFunc("PUT /api/timer/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg timer.Config
		if err := decode(r, &cfg); err != nil {
			writeErr(w, err)
			return
		}
		snap := s.timer.SetConfig(cfg)
		b, _ := json.Marshal(snap.Config)
		if err := s.svc.Store.SetSetting(r.Context(), TimerConfigKey, string(b)); err != nil {
			writeErr(w, err)
			return
		}
		reply(w, snap)
	})
}
