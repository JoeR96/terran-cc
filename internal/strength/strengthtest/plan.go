package strengthtest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// The plan routes. 99-Strength sends the plan enums (SessionKind,
// SessionStatus, PhaseType, DayNumber) as numbers, so the fake does too.
const (
	KindRest, KindLift, KindCardio, KindMobility = 0, 1, 2, 3
	StatusPlanned, StatusDone, StatusRest        = 0, 1, 4
)

// SetPlan sets the plan days GET /plans/upcoming serves: a JSON array of
// UpcomingDayDto. nil means no active plan (404).
func (f *Fake) SetPlan(days []byte) { f.mu.Lock(); f.plan = days; f.mu.Unlock() }

func session(kind int, label string, status int, liftDay int) map[string]any {
	s := map[string]any{
		"kind": kind, "label": label, "status": status, "liftDay": nil,
		"cardioProtocolId": nil, "mobilityRoutineId": nil, "completionId": nil, "hevyRoutineId": nil,
		"gymSessionId": nil, "hevyPostStatus": nil, "warmUpRoutineId": nil,
		"finisherCardioProtocolId": nil, "finisherCardioTrackId": nil,
	}
	if liftDay > 0 {
		s["liftDay"] = liftDay
	}
	return s
}

func sets(kg float64, reps, n int, amrap int) []map[string]any {
	var out []map[string]any
	for i := 0; i < n; i++ {
		out = append(out, map[string]any{"weightKg": kg, "reps": reps, "isAmrap": false})
	}
	if amrap > 0 {
		out = append(out, map[string]any{"weightKg": kg, "reps": amrap, "isAmrap": true})
	}
	return out
}

// SeedPlan is a week of an upper/lower plan from today: rest today, Upper B
// tomorrow, then cardio, Lower A, mobility, Upper A and rest again.
func SeedPlan(today time.Time) []byte {
	day := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	rest := func(n int) map[string]any {
		return map[string]any{"date": day(n), "phase": 0, "sessions": []any{session(KindRest, "Rest", StatusRest, 0)}, "lifts": []any{}}
	}
	lift := func(n, liftDay int, label string, ex []map[string]any, finisher any) map[string]any {
		return map[string]any{
			"date": day(n), "phase": 0,
			"sessions": []any{session(KindLift, label, StatusPlanned, liftDay)},
			"lifts": []any{map[string]any{
				"liftDay": liftDay, "label": label, "weekNumber": 3, "isDeloadWeek": false,
				"exercises": ex, "finisher": finisher,
			}},
		}
	}
	other := func(n, kind int, label string) map[string]any {
		return map[string]any{"date": day(n), "phase": 0, "sessions": []any{session(kind, label, StatusPlanned, 0)}, "lifts": []any{}}
	}
	days := []map[string]any{
		rest(0),
		lift(1, 2, "Upper B", []map[string]any{
			{"name": "Bench Press (Barbell)", "category": "MainLift", "sets": sets(75, 5, 4, 10)},
			{"name": "Seated Cable Row - V Grip (Cable)", "category": "Accessory", "sets": sets(50, 12, 3, 0)},
			{"name": "Lateral Raise (Cable)", "category": "Accessory", "sets": sets(7.5, 15, 3, 0)},
		}, map[string]any{"name": "Stair finisher", "prescription": "10 min · steady, level 8"}),
		other(2, KindCardio, "Zone 2 walk"),
		lift(3, 3, "Lower A", []map[string]any{
			{"name": "Squat (Barbell)", "category": "MainLift", "sets": sets(100, 5, 4, 10)},
			{"name": "Lying Leg Curl (Machine)", "category": "Accessory", "sets": sets(40, 12, 3, 0)},
		}, nil),
		other(4, KindMobility, "Mobility A"),
		lift(5, 1, "Upper A", []map[string]any{
			{"name": "Overhead Press (Barbell)", "category": "MainLift", "sets": sets(45, 5, 4, 10)},
		}, nil),
		rest(6),
	}
	b, _ := json.Marshal(days)
	return b
}

// servePlan answers GET /plans/upcoming and GET /plans/today. It reports
// false when the path is not one of them.
func (f *Fake) servePlan(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.Method != "GET" || (path != "/plans/upcoming" && path != "/plans/today") {
		return false
	}
	if f.plan == nil {
		problem(w, 404, "Not Found", "No active training plan.")
		return true
	}
	var days []map[string]any
	json.Unmarshal(f.plan, &days)
	q := r.URL.Query()
	if path == "/plans/today" {
		date := q.Get("date")
		if date == "" {
			date = f.Now().Format("2006-01-02")
		}
		for _, d := range days {
			if d["date"] == date {
				ok(w, map[string]any{"date": date, "phase": d["phase"], "sessions": d["sessions"], "week": map[string]any{"weekStart": date}})
				return true
			}
		}
		ok(w, map[string]any{"date": date, "phase": nil, "sessions": []any{}, "week": map[string]any{"weekStart": date}})
		return true
	}
	from := q.Get("from")
	if from == "" {
		from = f.Now().Format("2006-01-02")
	}
	n := 14
	if v := q.Get("days"); v != "" {
		n, _ = strconv.Atoi(v)
	}
	if n < 1 || n > 31 || !validDate(from) {
		problem(w, 400, "Validation Failed", "Days must be 1–31.")
		return true
	}
	out := []map[string]any{}
	for _, d := range days {
		if s, _ := d["date"].(string); s >= from && len(out) < n {
			out = append(out, d)
		}
	}
	ok(w, out)
	return true
}
