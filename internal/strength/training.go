package strength

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The wire shapes below mirror 99-Strength's plan DTOs (A2S.Application/
// DTOs/PlanDtos.cs: TodayDto, TodaySessionDto, UpcomingDayDto and friends).
//
// Unlike habits, the plan enums have no JsonStringEnumConverter and the API
// has no global one, so SessionKind, SessionStatus, PhaseType and DayNumber
// arrive as numbers. enum accepts the name as well, in case that changes.

// enum is a .NET enum read as its lower-case name ("lift", "done", "day1").
type enum string

// enumNames are the C# declaration orders. DayNumber starts at 1.
var (
	kindNames   = []string{"rest", "lift", "cardio", "mobility"}
	statusNames = []string{"planned", "done", "missed", "paused", "rest"}
	phaseNames  = []string{"cut", "maintenance", "pause", "deload"}
)

func decodeEnum(b []byte, names []string, first int, into *enum) error {
	b = bytes.TrimSpace(b)
	if string(b) == "null" {
		*into = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*into = enum(strings.ToLower(s))
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return fmt.Errorf("enum %s is neither a name nor a number", b)
	}
	if i := n - first; i >= 0 && i < len(names) {
		*into = enum(names[i])
	} else {
		*into = enum("unknown" + strconv.Itoa(n)) // a value newer than this build
	}
	return nil
}

// They are kept as their names, so the cache stores names too.
type sessionKind enum
type sessionStatus enum
type phaseType enum

// liftDay is DayNumber: Day1 = 1 … Day6 = 6. It reads as "Day1".
type liftDay string

func (k *sessionKind) UnmarshalJSON(b []byte) error { return decodeEnum(b, kindNames, 0, (*enum)(k)) }
func (s *sessionStatus) UnmarshalJSON(b []byte) error {
	return decodeEnum(b, statusNames, 0, (*enum)(s))
}
func (p *phaseType) UnmarshalJSON(b []byte) error { return decodeEnum(b, phaseNames, 0, (*enum)(p)) }

func (d *liftDay) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case string(b) == "null":
		*d = ""
	case len(b) > 0 && b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*d = liftDay(s)
	default:
		n, err := strconv.Atoi(string(b))
		if err != nil {
			return fmt.Errorf("liftDay %s is neither a name nor a number", b)
		}
		*d = liftDay("Day" + strconv.Itoa(n))
	}
	return nil
}

// RemoteSession is TodaySessionDto.
type RemoteSession struct {
	Kind              sessionKind   `json:"kind"`
	Label             string        `json:"label"`
	Status            sessionStatus `json:"status"`
	LiftDay           liftDay       `json:"liftDay"`
	CardioProtocolID  *string       `json:"cardioProtocolId"`
	MobilityRoutineID *string       `json:"mobilityRoutineId"`
	CompletionID      *string       `json:"completionId"`
	HevyRoutineID     *string       `json:"hevyRoutineId"`
}

// RemoteWeek is the part of WeeklyGoalsDto Terran CC reads.
type RemoteWeek struct {
	WeekStart       string `json:"weekStart"`
	LiftsPlanned    int    `json:"liftsPlanned"`
	LiftsDone       int    `json:"liftsDone"`
	CardioPlanned   int    `json:"cardioPlanned"`
	CardioDone      int    `json:"cardioDone"`
	MobilityPlanned int    `json:"mobilityPlanned"`
	MobilityDone    int    `json:"mobilityDone"`
}

// RemotePlanToday is TodayDto, the body of GET /plans/today.
type RemotePlanToday struct {
	Date     string          `json:"date"`
	Phase    phaseType       `json:"phase"`
	Sessions []RemoteSession `json:"sessions"`
	Week     RemoteWeek      `json:"week"`
}

// RemoteSet is UpcomingSetDto. The reps of an AMRAP set are its rep-out target.
type RemoteSet struct {
	WeightKg float64 `json:"weightKg"`
	Reps     int     `json:"reps"`
	IsAmrap  bool    `json:"isAmrap"`
}

type RemoteExercise struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	Sets     []RemoteSet `json:"sets"`
}

type RemoteFinisher struct {
	Name         string `json:"name"`
	Prescription string `json:"prescription"`
}

// RemoteLift is UpcomingLiftDto.
type RemoteLift struct {
	LiftDay      liftDay          `json:"liftDay"`
	Label        string           `json:"label"`
	WeekNumber   int              `json:"weekNumber"`
	IsDeloadWeek bool             `json:"isDeloadWeek"`
	Exercises    []RemoteExercise `json:"exercises"`
	Finisher     *RemoteFinisher  `json:"finisher"`
}

// RemoteUpcomingDay is UpcomingDayDto, one element of GET /plans/upcoming.
type RemoteUpcomingDay struct {
	Date     string          `json:"date"`
	Phase    phaseType       `json:"phase"`
	Sessions []RemoteSession `json:"sessions"`
	Lifts    []RemoteLift    `json:"lifts"`
}

// planToday reads the plan's sessions for a London date (yyyy-MM-dd).
func (r *remote) planToday(ctx context.Context, date string) (RemotePlanToday, error) {
	var t RemotePlanToday
	return t, r.do(ctx, "GET", "/plans/today?date="+url.QueryEscape(date), nil, &t)
}

// upcoming reads days (1–31) plan days from a date, rest days included.
func (r *remote) upcoming(ctx context.Context, from string, days int) ([]RemoteUpcomingDay, error) {
	d, _, err := r.upcomingIf(ctx, from, days, "")
	return d, err
}

// upcomingIf is upcoming with If-None-Match: errNotModified when etag holds.
func (r *remote) upcomingIf(ctx context.Context, from string, days int, etag string) ([]RemoteUpcomingDay, string, error) {
	var d []RemoteUpcomingDay
	q := "?from=" + url.QueryEscape(from) + "&days=" + strconv.Itoa(days)
	tag, err := r.get(ctx, "/plans/upcoming"+q, etag, &d)
	return d, tag, err
}

// --- what tccd serves ---

// TrainingSet is one planned set. For an AMRAP set Reps is the rep-out
// target: do at least that many, then as many as you can.
type TrainingSet struct {
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
	AMRAP    bool    `json:"amrap"`
}

type TrainingExercise struct {
	Name     string        `json:"name"`
	Category string        `json:"category"`
	Sets     []TrainingSet `json:"sets"`
}

type TrainingFinisher struct {
	Name         string `json:"name"`
	Prescription string `json:"prescription"`
}

// TrainingLift is the planned sets of a lift from the workout's current week.
type TrainingLift struct {
	Week      int                `json:"week"`
	Deload    bool               `json:"deload"`
	Exercises []TrainingExercise `json:"exercises"`
	Finisher  *TrainingFinisher  `json:"finisher,omitempty"`
}

// TrainingSession is one session of a day. Kind is rest, lift, cardio or
// mobility; Status is planned, done, missed, paused or rest. Lift is there
// for a lift still to do.
type TrainingSession struct {
	Kind          string        `json:"kind"`
	Label         string        `json:"label"`
	Status        string        `json:"status"`
	LiftDay       string        `json:"lift_day,omitempty"`
	HevyRoutineID string        `json:"hevy_routine_id,omitempty"`
	Lift          *TrainingLift `json:"lift,omitempty"`
}

// TrainingDay is one calendar day of the plan.
type TrainingDay struct {
	Date     string            `json:"date"`
	Phase    string            `json:"phase,omitempty"`
	Rest     bool              `json:"rest"` // nothing but rest is planned
	Sessions []TrainingSession `json:"sessions"`
}

// Training is the training API answer: plan days from From, today first
// when From is today.
type Training struct {
	Today  string        `json:"today"`
	From   string        `json:"from"`
	NoPlan bool          `json:"no_plan"` // 99-Strength has no active plan
	Days   []TrainingDay `json:"days"`
	Sync   *Sync         `json:"sync,omitempty"`
}

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// toTrainingDay maps an upcoming day and hangs each lift's sets on its session.
func toTrainingDay(d RemoteUpcomingDay) TrainingDay {
	out := TrainingDay{Date: d.Date, Phase: string(d.Phase), Rest: true, Sessions: []TrainingSession{}}
	used := make([]bool, len(d.Lifts))
	for _, s := range d.Sessions {
		ts := TrainingSession{
			Kind: string(s.Kind), Label: s.Label, Status: string(s.Status),
			LiftDay: string(s.LiftDay), HevyRoutineID: strOf(s.HevyRoutineID),
		}
		if ts.Kind == "lift" {
			for i, l := range d.Lifts {
				if !used[i] && l.LiftDay == s.LiftDay {
					used[i], ts.Lift = true, toLift(l)
					break
				}
			}
		}
		if ts.Kind != "rest" {
			out.Rest = false
		}
		out.Sessions = append(out.Sessions, ts)
	}
	for i, l := range d.Lifts { // a lift with no session line: still show it
		if !used[i] {
			out.Rest = false
			out.Sessions = append(out.Sessions, TrainingSession{Kind: "lift", Label: l.Label, Status: "planned", LiftDay: string(l.LiftDay), Lift: toLift(l)})
		}
	}
	return out
}

func toLift(l RemoteLift) *TrainingLift {
	t := &TrainingLift{Week: l.WeekNumber, Deload: l.IsDeloadWeek, Exercises: []TrainingExercise{}}
	for _, e := range l.Exercises {
		te := TrainingExercise{Name: e.Name, Category: e.Category, Sets: []TrainingSet{}}
		for _, s := range e.Sets {
			te.Sets = append(te.Sets, TrainingSet{WeightKg: s.WeightKg, Reps: s.Reps, AMRAP: s.IsAmrap})
		}
		t.Exercises = append(t.Exercises, te)
	}
	if l.Finisher != nil {
		t.Finisher = &TrainingFinisher{Name: l.Finisher.Name, Prescription: l.Finisher.Prescription}
	}
	return t
}

// --- the cache ---

const keyTraining = "strength.training"

// trainingWindow is how many days from today are read and kept. The status
// line and the desktop card only need today and the next planned day, and
// two weeks covers that through any plan.
const trainingWindow = 14

// trainingFresh is how long a read of the plan is served before reading again.
const trainingFresh = 2 * time.Minute

// trainingCache is the last successful GET /plans/upcoming from today.
type trainingCache struct {
	SyncedAt time.Time           `json:"synced_at"`
	From     string              `json:"from"`
	NoPlan   bool                `json:"no_plan"`
	Days     []RemoteUpcomingDay `json:"days"`
	ETag     string              `json:"etag,omitempty"` // for If-None-Match
}

// same reports whether two reads would show the same plan.
func (c *trainingCache) same(o *trainingCache) bool {
	if c == nil || o == nil {
		return c == o
	}
	a, _ := json.Marshal(trainingCache{From: c.From, NoPlan: c.NoPlan, Days: c.Days})
	b, _ := json.Marshal(trainingCache{From: o.From, NoPlan: o.NoPlan, Days: o.Days})
	return bytes.Equal(a, b)
}

func addDays(date string, n int) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func (s *Service) loadTraining() *trainingCache {
	if s.train != nil {
		return s.train
	}
	v, ok, err := s.store.GetSetting(context.Background(), keyTraining)
	if err != nil || !ok {
		return nil
	}
	var c trainingCache
	if json.Unmarshal([]byte(v), &c) != nil || c.From == "" {
		return nil
	}
	s.train = &c
	return s.train
}

// trainingSyncLocked is how fresh the training is: the habit connection's
// state, plus whether the plan itself could be read.
func (s *Service) trainingSyncLocked(c *trainingCache, failed string) Sync {
	sy := Sync{NeedsToken: s.cfg.NeedsToken, Error: failed}
	if c != nil {
		t := c.SyncedAt
		sy.LastSync = &t
	}
	sy.Stale = failed != "" || s.cfg.NeedsToken || c == nil
	switch {
	case sy.NeedsToken:
		sy.State = "auth"
	case sy.Stale:
		sy.State = "stale"
	default:
		sy.State = "ok"
	}
	return sy
}

func (c *trainingCache) slice(from string, days int) []RemoteUpcomingDay {
	last := addDays(from, days-1)
	var out []RemoteUpcomingDay
	for _, d := range c.Days {
		if d.Date >= from && d.Date <= last {
			out = append(out, d)
		}
	}
	return out
}

func buildTraining(today, from string, noPlan bool, days []RemoteUpcomingDay, sy Sync) Training {
	t := Training{Today: today, From: from, NoPlan: noPlan, Days: []TrainingDay{}, Sync: &sy}
	for _, d := range days {
		t.Days = append(t.Days, toTrainingDay(d))
	}
	return t
}

// Training is the plan from from ("" is today) for days days (1–31). Two
// weeks from today are kept: read again when more than two minutes old, and
// served as they were, marked stale, while 99-Strength is unreachable.
// Other ranges are read live.
func (s *Service) Training(ctx context.Context, from string, days int) (Training, error) {
	if days <= 0 {
		days = trainingWindow
	}
	if days > 31 {
		return Training{}, invalid("days must be 1 to 31")
	}
	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			return Training{}, invalid("from must be a date like 2026-11-24")
		}
	}

	s.mu.Lock()
	if !s.cfg.Connected {
		s.mu.Unlock()
		return Training{}, ErrNotConnected
	}
	today := s.today()
	if from == "" {
		from = today
	}
	cfg, epoch := s.cfg, s.epoch
	c := s.loadTraining()
	if c != nil && c.From != today {
		c = nil // yesterday's window: only good for an outage
	}
	inWindow := from >= today && addDays(from, days-1) <= addDays(today, trainingWindow-1)
	if c != nil && inWindow && s.now().Sub(c.SyncedAt) < trainingFresh {
		t := buildTraining(today, from, c.NoPlan, c.slice(from, days), s.trainingSyncLocked(c, ""))
		s.mu.Unlock()
		return t, nil
	}
	s.mu.Unlock()

	var err error
	if inWindow {
		var fresh *trainingCache
		if fresh, err = s.readWindow(ctx); err == nil {
			s.mu.Lock()
			defer s.mu.Unlock()
			return buildTraining(today, from, fresh.NoPlan, fresh.slice(from, days), s.trainingSyncLocked(fresh, "")), nil
		}
	} else {
		token, terr := s.tokens.Get()
		var got []RemoteUpcomingDay
		err = terr
		if err == nil {
			got, err = (&remote{base: cfg.BaseURL, token: token, http: s.http}).upcoming(ctx, from, days)
		}
		noPlan := statusOf(err) == http.StatusNotFound
		if err == nil || noPlan {
			if noPlan {
				got = nil
			}
			fresh := &trainingCache{SyncedAt: s.now(), From: from, NoPlan: noPlan, Days: got}
			s.mu.Lock()
			defer s.mu.Unlock()
			return buildTraining(today, from, noPlan, fresh.slice(from, days), s.trainingSyncLocked(fresh, "")), nil
		}
	}

	if unauthorized(err) {
		s.needToken(epoch, "99-Strength no longer accepts the token")
	}
	why := "99-Strength is not reachable"
	if st := statusOf(err); st != 0 {
		why = fmt.Sprintf("99-Strength answered %d", st)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.loadTraining(); old != nil && from >= old.From {
		if part := old.slice(from, days); len(part) > 0 || old.NoPlan {
			return buildTraining(today, from, old.NoPlan, part, s.trainingSyncLocked(old, why)), nil
		}
	}
	if !retryable(err) && !unauthorized(err) {
		return Training{}, fmt.Errorf("%w: it answered %d", ErrUnreachable, statusOf(err))
	}
	return Training{}, fmt.Errorf("%w: %s and no plan has been read yet", ErrUnreachable, why)
}

// RefreshTraining reads the two-week window again now (a conditional GET, so
// an unchanged plan costs a 304) and tells NotifyTraining when it changed.
func (s *Service) RefreshTraining(ctx context.Context) error {
	_, err := s.readWindow(ctx)
	return err
}

// readWindow reads the plan from today for trainingWindow days, sending the
// cached ETag. A 304 keeps the cached plan and makes it fresh again.
func (s *Service) readWindow(ctx context.Context) (*trainingCache, error) {
	s.mu.Lock()
	if !s.cfg.Connected {
		s.mu.Unlock()
		return nil, ErrNotConnected
	}
	cfg, epoch, today := s.cfg, s.epoch, s.today()
	c := s.loadTraining()
	etag := ""
	if c != nil && c.From == today {
		etag = c.ETag
	}
	s.mu.Unlock()

	token, err := s.tokens.Get()
	if err != nil {
		return nil, err
	}
	got, tag, err := (&remote{base: cfg.BaseURL, token: token, http: s.http}).upcomingIf(ctx, today, trainingWindow, etag)
	var fresh *trainingCache
	switch {
	case errors.Is(err, errNotModified) && c != nil:
		cp := *c
		cp.SyncedAt = s.now()
		fresh = &cp
	case err == nil:
		fresh = &trainingCache{SyncedAt: s.now(), From: today, Days: got, ETag: tag}
	case statusOf(err) == http.StatusNotFound:
		fresh = &trainingCache{SyncedAt: s.now(), From: today, NoPlan: true}
	default:
		if errors.Is(err, errNotModified) {
			err = fmt.Errorf("%w: a 304 with nothing cached", ErrUnreachable)
		}
		return nil, err
	}
	s.mu.Lock()
	changed := false
	if s.epoch == epoch {
		changed = !s.train.same(fresh)
		s.train = fresh
		s.save(keyTraining, fresh)
	}
	s.mu.Unlock()
	if changed {
		s.notifyTraining()
	}
	return fresh, nil
}
