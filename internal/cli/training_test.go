package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"terrancc/internal/strength"
	"terrancc/internal/strength/strengthtest"
)

// planFrom is strength's plans_upcoming.json moved so its first day
// (a rest day) is today: then Upper A with AMRAP and a finisher, a cardio
// day, and a deload lower day.
func planFrom(t *testing.T, today time.Time) []byte {
	b, err := os.ReadFile("../strength/testdata/plans_upcoming.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for i, d := range []string{"2026-11-22", "2026-11-23", "2026-11-24", "2026-11-25"} {
		s = strings.ReplaceAll(s, d, today.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return []byte(s)
}

func londonNow(t *testing.T) time.Time { return time.Now().In(london(t)) }

func TestTrainingInStatusAndWorkout(t *testing.T) {
	h := newStrengthHarness(t)
	now := londonNow(t)
	h.fake.SetPlan(planFrom(t, now))
	h.connect()
	tomorrow := now.AddDate(0, 0, 1).Format("Mon 2 Jan")

	out := h.ok("status")
	has(t, out, "\n🏋 Today: Rest · Up next: "+tomorrow+" Upper A — Bench Press 5×5+ @ 75 kg\n")

	var st struct {
		Training struct {
			Days []json.RawMessage `json:"days"`
		} `json:"training"`
	}
	if err := json.Unmarshal([]byte(h.ok("status", "--json")), &st); err != nil || len(st.Training.Days) != 4 {
		t.Fatalf("status --json: %v %d", err, len(st.Training.Days))
	}

	out = h.ok("workout")
	has(t, out,
		"Today is a rest day. Next planned day:\n"+tomorrow+" · Cut phase\n",
		"  🏋 Upper A (planned) · week 3\n",
		"    Bench Press (Barbell)\n      75 kg × 5 · 75 kg × 5 · 75 kg × 5 · 75 kg × 5 · 75 kg × 10+ (AMRAP)\n",
		"    Lat Pulldown (Cable)\n      52.5 kg × 12 · 52.5 kg × 12 · 52.5 kg × 12\n",
		"    Finisher: Stair finisher — 10 min · steady, level 8\n",
		futureNote)
	if out2 := h.ok("workout", "tomorrow"); !strings.Contains(out2, "Upper A") || strings.Contains(out2, "rest day") {
		t.Fatalf("tomorrow:\n%s", out2)
	}
	if out2 := h.ok("workout", "next"); !strings.HasPrefix(out2, tomorrow) {
		t.Fatalf("next:\n%s", out2)
	}

	out = h.ok("workout", now.AddDate(0, 0, 3).Format("2006-01-02"))
	has(t, out, "  🏋 Lower A (planned) · week 7\n", "    Deload week: lighter weights and fewer sets, no AMRAP\n", "      60 kg × 5 · 60 kg × 5")

	out = h.ok("workout", now.AddDate(0, 0, 2).Format("2006-01-02"))
	has(t, out, "  🏃 Zone 2 walk (planned)\n", "  🧘 Mobility A (planned)\n")

	var w workoutOut
	if err := json.Unmarshal([]byte(h.ok("workout", "--json")), &w); err != nil || w.Day == nil || w.Day.Sessions[0].Lift == nil || !w.Future {
		t.Fatalf("workout --json: %v %+v", err, w)
	}

	if code, _, errOut := h.runIn("", "workout", "someday"); code == 0 || !strings.Contains(errOut, "not a day") {
		t.Fatalf("someday: %d %s", code, errOut)
	}
}

func TestTrainingTodayLiftDone(t *testing.T) {
	h := newStrengthHarness(t)
	now := londonNow(t)
	plan := strings.Replace(string(planFrom(t, now.AddDate(0, 0, -1))), `"status": 0, "liftDay": 1`, `"status": 1, "liftDay": 1`, 1)
	h.fake.SetPlan([]byte(plan))
	h.connect()
	// Today is the lift day, done; up next is the cardio day.
	out := h.ok("status")
	has(t, out, "🏋 Today: Upper A ✓ · finisher: Stair 10 min · Up next: "+now.AddDate(0, 0, 1).Format("Mon 2 Jan")+" Zone 2 walk · Mobility A\n")
}

func TestTrainingNoPlanAndOutage(t *testing.T) {
	h := newStrengthHarness(t)
	h.connect()
	has(t, h.ok("status"), "🏋 Training: no active plan in 99-Strength\n")
	has(t, h.ok("workout"), "No active training plan in 99-Strength.\n")

	h2 := newStrengthHarness(t)
	h2.fake.SetPlan(strengthtest.SeedPlan(londonNow(t)))
	h2.connect()
	h2.fake.Down(503)
	// Nothing read yet: status still works and says why there is no plan.
	has(t, h2.ok("status"), "🏋 Training: 99-Strength is not reachable")
}

func TestStatusWithoutStrengthHasNoTraining(t *testing.T) {
	h := newStrengthHarness(t)
	if out := h.ok("status"); strings.Contains(out, "🏋") {
		t.Fatalf("training shown while not connected:\n%s", out)
	}
}

func TestTrainingLinesStale(t *testing.T) {
	at := time.Date(2026, 11, 20, 8, 5, 0, 0, time.Local)
	tr := strength.Training{
		Today: "2026-11-22",
		Days: []strength.TrainingDay{{Date: "2026-11-22", Sessions: []strength.TrainingSession{
			{Kind: "lift", Label: "Upper A", Status: "planned", Lift: &strength.TrainingLift{Finisher: &strength.TrainingFinisher{Name: "Stair finisher", Prescription: "10 min · steady, level 8"}}},
		}}},
		Sync: &strength.Sync{State: "stale", Stale: true, LastSync: &at},
	}
	got := strings.Join(trainingLines(tr), "\n")
	want := "🏋 Today: Upper A (planned) · finisher: Stair 10 min\n  ⚠ 99-Strength is offline. Showing the plan from Fri 20 Nov 08:05"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestWeightZero(t *testing.T) {
	set := func(w float64, amrap bool) strength.TrainingSet { return strength.TrainingSet{WeightKg: w, Reps: 10, AMRAP: amrap} }
	curl := strength.TrainingExercise{Name: "Lying Leg Curl (Machine)", Category: "Accessory", Sets: []strength.TrainingSet{set(0, false)}}
	dip := strength.TrainingExercise{Name: "Bodyweight Dip", Category: "Accessory", Sets: []strength.TrainingSet{set(0, false)}}
	squat := strength.TrainingExercise{Name: "Squat (Barbell)", Category: "MainLift", Sets: []strength.TrainingSet{set(0, false), set(0, true)}}
	cases := []struct{ got, want string }{
		{setText(curl, set(0, false)), "× 10"},
		{setText(dip, set(0, false)), "bodyweight × 10"},
		{setText(squat, set(0, true)), "— × 10+ (AMRAP)"},
		{setText(curl, set(52.5, false)), "52.5 kg × 10"},
		{liftSummary(&strength.TrainingLift{Exercises: []strength.TrainingExercise{squat}}), "Squat 2×10+ @ —"},
		{liftSummary(&strength.TrainingLift{Exercises: []strength.TrainingExercise{curl}}), "Lying Leg Curl 1×10"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if !needsWeight(squat) || needsWeight(curl) || needsWeight(dip) {
		t.Error("needsWeight: only a loaded lift with no weight needs one")
	}
}
