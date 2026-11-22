package strength

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"terrancc/internal/strength/strengthtest"
)

// testdata/plans_*.json are written by hand from PlanDtos.cs (TodayDto,
// UpcomingDayDto). The plan enums have no string converter in 99-Strength,
// so they are numbers there; plans_upcoming_names.json is the same shape
// with names, which must read the same.

func TestPlanTodayContract(t *testing.T) {
	r, got := replay(t, 200, "plans_today.json")
	d, err := r.planToday(context.Background(), "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	if *got != (call{"GET", "/api/v1/plans/today?date=2026-11-23", fixBearer, ""}) {
		t.Fatalf("call: %+v", *got)
	}
	if d.Date != "2026-11-23" || string(d.Phase) != "cut" || len(d.Sessions) != 2 || d.Week.LiftsDone != 1 {
		t.Fatalf("today: %+v", d)
	}
	s := d.Sessions[0]
	if string(s.Kind) != "lift" || string(s.Status) != "done" || s.LiftDay != "Day1" || strOf(s.HevyRoutineID) != "f3b0c1d2-hevy-routine" {
		t.Fatalf("lift session: %+v", s)
	}
	if m := d.Sessions[1]; string(m.Kind) != "mobility" || string(m.Status) != "planned" || m.LiftDay != "" {
		t.Fatalf("mobility session: %+v", m)
	}
}

func TestUpcomingContract(t *testing.T) {
	r, got := replay(t, 200, "plans_upcoming.json")
	days, err := r.upcoming(context.Background(), "2026-11-22", 14)
	if err != nil {
		t.Fatal(err)
	}
	if *got != (call{"GET", "/api/v1/plans/upcoming?from=2026-11-22&days=14", fixBearer, ""}) {
		t.Fatalf("call: %+v", *got)
	}
	if len(days) != 4 {
		t.Fatalf("days: %d", len(days))
	}

	rest := toTrainingDay(days[0])
	if !rest.Rest || rest.Phase != "cut" || len(rest.Sessions) != 1 || rest.Sessions[0].Status != "rest" {
		t.Fatalf("rest day: %+v", rest)
	}

	lift := toTrainingDay(days[1])
	want := TrainingSession{
		Kind: "lift", Label: "Upper A", Status: "planned", LiftDay: "Day1", HevyRoutineID: "f3b0c1d2-hevy-routine",
		Lift: &TrainingLift{Week: 3, Exercises: []TrainingExercise{
			{Name: "Bench Press (Barbell)", Category: "MainLift", Sets: []TrainingSet{
				{75, 5, false}, {75, 5, false}, {75, 5, false}, {75, 5, false}, {75, 10, true},
			}},
			{Name: "Lat Pulldown (Cable)", Category: "Accessory", Sets: []TrainingSet{{52.5, 12, false}, {52.5, 12, false}, {52.5, 12, false}}},
		}, Finisher: &TrainingFinisher{Name: "Stair finisher", Prescription: "10 min · steady, level 8"}},
	}
	if lift.Rest || len(lift.Sessions) != 1 || !reflect.DeepEqual(lift.Sessions[0], want) {
		t.Fatalf("lift day: %+v", lift.Sessions)
	}

	cardio := toTrainingDay(days[2])
	if cardio.Rest || len(cardio.Sessions) != 2 || cardio.Sessions[0].Kind != "cardio" || cardio.Sessions[0].Label != "Zone 2 walk" || cardio.Sessions[0].Lift != nil {
		t.Fatalf("cardio day: %+v", cardio)
	}

	deload := toTrainingDay(days[3])
	if deload.Phase != "" || deload.Sessions[0].Lift == nil || !deload.Sessions[0].Lift.Deload || deload.Sessions[0].Lift.Finisher != nil {
		t.Fatalf("deload day: %+v", deload)
	}
}

func TestUpcomingEnumNames(t *testing.T) {
	r, _ := replay(t, 200, "plans_upcoming_names.json")
	days, err := r.upcoming(context.Background(), "2026-11-23", 1)
	if err != nil {
		t.Fatal(err)
	}
	d := toTrainingDay(days[0])
	if d.Phase != "cut" || d.Sessions[0].Kind != "lift" || d.Sessions[0].Status != "done" || d.Sessions[0].LiftDay != "Day1" {
		t.Fatalf("names: %+v", d)
	}
}

func TestUpcomingNoPlanContract(t *testing.T) {
	r, _ := replay(t, 404, "problem_404_plan.json")
	_, err := r.upcoming(context.Background(), "2026-11-23", 14)
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 404 || he.Detail != "No active training plan." {
		t.Fatalf("err: %v", err)
	}
}

func TestFakeMatchesPlanFixture(t *testing.T) {
	e := newEnv(t)
	e.fake.SetPlan(fixture(t, "plans_upcoming.json"))
	r := &remote{base: e.base(), token: strengthtest.Token, http: e.srv.Client()}
	days, err := r.upcoming(bg, "2026-11-23", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Date != "2026-11-23" || days[1].Date != "2026-11-24" {
		t.Fatalf("fake upcoming: %+v", days)
	}
	if _, err := r.upcoming(bg, "2026-11-23", 32); statusOf(err) != 400 {
		t.Fatalf("days 32: %v", err)
	}
	seed, err := r.planToday(bg, "2026-11-23")
	if err != nil || string(seed.Sessions[0].Kind) != "lift" {
		t.Fatalf("fake today: %+v %v", seed, err)
	}
}

// --- the service ---

func trainingEnv(t *testing.T) *env {
	e := connected(t)
	e.fake.SetPlan(fixture(t, "plans_upcoming.json"))
	e.setClock(time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC))
	return e
}

func (e *env) planCalls() int {
	n := 0
	for _, r := range e.fake.Requests() {
		if len(r.Path) > 20 && r.Path[:20] == "/api/v1/plans/upcomi" {
			n++
		}
	}
	return n
}

func TestTrainingReadsAndCaches(t *testing.T) {
	e := trainingEnv(t)
	tr, err := e.svc.Training(bg, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Today != "2026-11-22" || tr.From != "2026-11-22" || tr.NoPlan || len(tr.Days) != 4 || tr.Sync.State != "ok" {
		t.Fatalf("training: %+v", tr)
	}
	if !tr.Days[0].Rest || tr.Days[1].Sessions[0].Lift == nil {
		t.Fatalf("days: %+v", tr.Days)
	}
	// Inside the window and fresh: from the cache.
	one, err := e.svc.Training(bg, "2026-11-23", 1)
	if err != nil || len(one.Days) != 1 || one.Days[0].Date != "2026-11-23" {
		t.Fatalf("one day: %+v %v", one, err)
	}
	if n := e.planCalls(); n != 1 {
		t.Fatalf("plan calls: %d, want 1", n)
	}
	// Older than trainingFresh: read again.
	e.setClock(e.clock().Add(3 * time.Minute))
	if _, err := e.svc.Training(bg, "", 0); err != nil {
		t.Fatal(err)
	}
	if n := e.planCalls(); n != 2 {
		t.Fatalf("plan calls: %d, want 2", n)
	}
	// Beyond the window: live, and not cached.
	if _, err := e.svc.Training(bg, "2026-12-20", 3); err != nil {
		t.Fatal(err)
	}
	if n := e.planCalls(); n != 3 {
		t.Fatalf("plan calls: %d, want 3", n)
	}
	if c := e.svc.loadTraining(); c.From != "2026-11-22" {
		t.Fatalf("cache moved to %s", c.From)
	}
}

func TestTrainingNoPlan(t *testing.T) {
	e := trainingEnv(t)
	e.fake.SetPlan(nil)
	tr, err := e.svc.Training(bg, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !tr.NoPlan || len(tr.Days) != 0 || tr.Sync.Stale {
		t.Fatalf("no plan: %+v", tr)
	}
}

func TestTrainingOutageShowsLastKnown(t *testing.T) {
	e := trainingEnv(t)
	if _, err := e.svc.Training(bg, "", 0); err != nil {
		t.Fatal(err)
	}
	e.fake.Down(503)
	e.setClock(e.clock().Add(10 * time.Minute))
	tr, err := e.svc.Training(bg, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !tr.Sync.Stale || tr.Sync.State != "stale" || tr.Sync.Error != "99-Strength answered 503" || tr.Sync.LastSync == nil || len(tr.Days) != 4 {
		t.Fatalf("outage: %+v %+v", tr, tr.Sync)
	}
	// A restart keeps the last plan read.
	e.svc = e.open(Options{})
	tr, err = e.svc.Training(bg, "2026-11-23", 1)
	if err != nil || !tr.Sync.Stale || len(tr.Days) != 1 {
		t.Fatalf("after restart: %+v %v", tr, err)
	}
	// The next morning the cached days from today on still show.
	e.setClock(time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC))
	tr, err = e.svc.Training(bg, "", 0)
	if err != nil || tr.Today != "2026-11-23" || len(tr.Days) != 3 || tr.Days[0].Date != "2026-11-23" {
		t.Fatalf("next day: %+v %v", tr, err)
	}
}

func TestTrainingOutageWithNothingRead(t *testing.T) {
	e := trainingEnv(t)
	e.fake.Down(503)
	if _, err := e.svc.Training(bg, "", 0); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err: %v", err)
	}
}

func TestTrainingTokenRevoked(t *testing.T) {
	e := trainingEnv(t)
	e.fake.SetToken("")
	if _, err := e.svc.Training(bg, "", 0); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err: %v", err)
	}
	if st := e.svc.Status(); st.State != "auth" {
		t.Fatalf("state: %s", st.State)
	}
}

func TestTrainingNotConnectedAndBadInput(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Training(bg, "", 0); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("err: %v", err)
	}
	e = trainingEnv(t)
	if _, err := e.svc.Training(bg, "", 40); err == nil {
		t.Fatal("days 40 accepted")
	}
	if _, err := e.svc.Training(bg, "tomorrow", 1); err == nil {
		t.Fatal("bad date accepted")
	}
}

func TestTrainingDisconnectForgetsPlan(t *testing.T) {
	e := trainingEnv(t)
	if _, err := e.svc.Training(bg, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if c := e.svc.loadTraining(); c != nil {
		t.Fatalf("cache kept: %+v", c)
	}
}
