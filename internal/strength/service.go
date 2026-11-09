// Package strength makes Terran CC's habits a client of 99-Strength, Joe's
// training app, which owns habits once it is connected.
//
// The service keeps the last "today" it read in the store, so the widget
// draws at once and still has values when the API is down. Writes go into a
// stored queue first and are sent by one background worker, in order. Nothing
// here blocks a caller on the network for longer than LogWait.
package strength

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Windows has no zoneinfo; 99-Strength's days are London days

	"terrancc/internal/core"
	"terrancc/internal/model"
)

// Settings is the slice of store.Store this package needs.
type Settings interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Settings keys. None of them ever holds the token.
const (
	keyConfig = "strength.config"
	keyToday  = "strength.today"
	keyQueue  = "strength.queue"
	keyIDs    = "strength.ids"
)

var (
	// ErrNotConnected: no 99-Strength connection is configured.
	ErrNotConnected = errors.New("not connected to 99-Strength")
	// ErrRejected: 99-Strength answered, and said no to the token.
	ErrRejected = errors.New("99-Strength did not accept that token")
	// ErrUnreachable: no usable answer came back.
	ErrUnreachable = errors.New("99-Strength is not reachable")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", core.ErrInvalid, fmt.Sprintf(format, a...))
}

type config struct {
	BaseURL    string `json:"base_url"`
	Connected  bool   `json:"connected"`
	NeedsToken bool   `json:"needs_token,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Email      string `json:"email,omitempty"`
	TokenName  string `json:"token_name,omitempty"`
}

// cached is the last successful GET /habits/today.
type cached struct {
	SyncedAt time.Time   `json:"synced_at"`
	Today    RemoteToday `json:"today"`
}

const (
	opInc = "inc" // POST .../increment, sent at most once
	opSet = "set" // PUT .../logs/{date}, safe to repeat
)

// op is one queued write. Date and At are fixed when it was made, never at
// replay, so a tap at 23:58 lands on that day even if it is sent at 00:05.
type op struct {
	ID       string    `json:"id"`
	HabitID  string    `json:"habit_id"`
	Name     string    `json:"name"`
	Date     string    `json:"date"`
	Kind     string    `json:"kind"`
	Delta    int       `json:"delta,omitempty"`
	Value    float64   `json:"value"`            // what this write should leave behind
	Weight   bool      `json:"weight,omitempty"` // a weigh-in: also PUT /body-weight/{date}
	At       time.Time `json:"at"`
	Attempts int       `json:"attempts,omitempty"`
}

type idMap struct {
	Next int64            `json:"next"`
	IDs  map[string]int64 `json:"ids"`
}

// Sync says how fresh the habits are.
type Sync struct {
	// State is "ok", "stale" (showing saved values) or "auth" (the token
	// needs renewing). A disconnected Status has "disconnected".
	State      string     `json:"state"`
	Stale      bool       `json:"stale"`
	NeedsToken bool       `json:"needs_token"`
	LastSync   *time.Time `json:"last_sync"`
	Pending    int        `json:"pending"` // writes waiting to reach 99-Strength
	Error      string     `json:"error,omitempty"`
}

// Status is what `tcc strength status` shows.
type Status struct {
	Connected  bool   `json:"connected"`
	BaseURL    string `json:"base_url,omitempty"`
	Email      string `json:"email,omitempty"`
	TokenName  string `json:"token_name,omitempty"`
	TokenStore string `json:"token_store"`
	Sync
}

// StreakInfo is 99-Strength's streak: one missed day is forgiven, two in a
// row end it.
type StreakInfo struct {
	Current        int    `json:"current"`
	Best           int    `json:"best"`
	LastCountedDay string `json:"last_counted_day,omitempty"`
	TodayCounted   bool   `json:"today_counted"`
	GraceUsed      bool   `json:"grace_used"`
}

// Day is the habits API answer when 99-Strength is the source. It extends
// the local shape (date, streak, habits) rather than replacing it.
type Day struct {
	Date       string           `json:"date"`
	Streak     int              `json:"streak"`
	Habits     []model.HabitDay `json:"habits"`
	Source     string           `json:"source"`
	StreakInfo *StreakInfo      `json:"streak_info,omitempty"`
	Sync       *Sync            `json:"sync,omitempty"`
}

type Options struct {
	Store  Settings
	Tokens TokenStore
	// Notify is called (never under a lock) when what the habit widget
	// shows has changed. tccd publishes habit.changed from it.
	Notify func()
	Now    func() time.Time
	HTTP   *http.Client
	// Refresh is how often today is re-read while all is well.
	Refresh time.Duration
	// LogWait is how long a log waits for 99-Strength before it answers
	// with the value it expects and leaves the write pending.
	LogWait time.Duration
	Logf    func(format string, a ...any)
}

type Service struct {
	store   Settings
	tokens  TokenStore
	notify  func()
	now     func() time.Time
	http    *http.Client
	refresh time.Duration
	logWait time.Duration
	logf    func(string, ...any)
	loc     *time.Location

	wake chan struct{}
	sem  chan struct{} // one sync round at a time

	mu          sync.Mutex
	cfg         config
	cache       *cached
	queue       []op
	ids         idMap
	offline     bool // the last round got no usable answer
	lastErr     string
	dropNote    string // the last write 99-Strength refused, shown for a while
	dropAt      time.Time
	failures    int
	lastAttempt time.Time
	epoch       int // bumps on connect and disconnect; stale rounds are ignored
	waiters     map[string]chan struct{}
}

// New loads the saved connection, cache and queue.
func New(o Options) (*Service, error) {
	s := &Service{
		store: o.Store, tokens: o.Tokens, notify: o.Notify, now: o.Now, http: o.HTTP,
		refresh: o.Refresh, logWait: o.LogWait, logf: o.Logf,
		wake: make(chan struct{}, 1), sem: make(chan struct{}, 1),
		ids:     idMap{Next: 1, IDs: map[string]int64{}},
		waiters: map[string]chan struct{}{},
	}
	if s.notify == nil {
		s.notify = func() {}
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.http == nil {
		s.http = &http.Client{Timeout: 4 * time.Second}
	}
	if s.refresh <= 0 {
		s.refresh = time.Minute
	}
	if s.logf == nil {
		s.logf = log.Printf
	}
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		loc = time.Local
	}
	s.loc = loc

	ctx := context.Background()
	load := func(key string, into any) error {
		v, ok, err := s.store.GetSetting(ctx, key)
		if err != nil || !ok {
			return err
		}
		if err := json.Unmarshal([]byte(v), into); err != nil {
			s.logf("strength: ignoring unreadable %s: %v", key, err)
		}
		return nil
	}
	var c cached
	if err := load(keyConfig, &s.cfg); err != nil {
		return nil, err
	}
	if err := load(keyToday, &c); err != nil {
		return nil, err
	}
	if c.Today.Date != "" {
		s.cache = &c
	}
	if err := load(keyQueue, &s.queue); err != nil {
		return nil, err
	}
	// An increment still queued when tccd stopped may already have been
	// sent, so after a restart it is replayed as a set like the rest.
	s.queueAsSetsLocked()
	if err := load(keyIDs, &s.ids); err != nil {
		return nil, err
	}
	if s.ids.IDs == nil {
		s.ids = idMap{Next: 1, IDs: map[string]int64{}}
	}
	return s, nil
}

func (s *Service) save(key string, v any) {
	b, err := json.Marshal(v)
	if err == nil {
		err = s.store.SetSetting(context.Background(), key, string(b))
	}
	if err != nil {
		s.logf("strength: saving %s: %v", key, err)
	}
}

// today is the current London date, the one 99-Strength's days use.
func (s *Service) today() string { return s.now().In(s.loc).Format("2006-01-02") }

// Connected reports whether habits come from 99-Strength. It stays true
// while the API is down or the token needs renewing.
func (s *Service) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Connected
}

// --- connecting ---

var tokenRE = regexp.MustCompile(`^nns_pat_[A-Za-z0-9_-]{43}$`)

// cleanBase checks a base URL. Plain http is only allowed to this machine,
// so a token is never sent over the network in the clear.
func cleanBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", invalid("the API address must look like https://api-99strength.enjoeneer.dev/api/v1")
	}
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", invalid("the API address must use https (http is only allowed for localhost)")
		}
	default:
		return "", invalid("the API address must start with https://")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// Connect checks the token with whoami and only then stores it. Renewing the
// token of the account already connected keeps the cache and queued writes.
func (s *Service) Connect(ctx context.Context, baseURL, token string) (Status, error) {
	base, err := cleanBase(baseURL)
	if err != nil {
		return s.Status(), err
	}
	token = strings.TrimSpace(token)
	if !tokenRE.MatchString(token) {
		return s.Status(), invalid("that is not a 99-Strength token (expected nns_pat_ followed by 43 characters)")
	}
	who, err := (&remote{base: base, token: token, http: s.http}).whoAmI(ctx)
	switch {
	case unauthorized(err):
		return s.Status(), ErrRejected
	case statusOf(err) == http.StatusForbidden:
		return s.Status(), fmt.Errorf("%w: it may not read its own details (403)", ErrRejected)
	case err != nil:
		s.logf("strength: connect to %s failed: %s", base, redact(err.Error(), token))
		if statusOf(err) != 0 {
			return s.Status(), fmt.Errorf("%w at %s: it answered %d", ErrUnreachable, base, statusOf(err))
		}
		return s.Status(), fmt.Errorf("%w at %s", ErrUnreachable, base)
	case who.UserID == "":
		return s.Status(), fmt.Errorf("%w at %s: the answer was not a whoami", ErrUnreachable, base)
	}
	if err := s.tokens.Set(token); err != nil {
		return s.Status(), fmt.Errorf("could not store the token in %s: %w", s.tokens.Where(), err)
	}

	s.mu.Lock()
	same := s.cfg.Connected && s.cfg.BaseURL == base && s.cfg.UserID == who.UserID
	if !same {
		s.cache, s.queue = nil, nil
		s.ids = idMap{Next: 1, IDs: map[string]int64{}}
		s.save(keyToday, cached{})
		s.save(keyQueue, s.queue)
		s.save(keyIDs, s.ids)
	}
	s.cfg = config{BaseURL: base, Connected: true, UserID: who.UserID, Email: who.Email, TokenName: who.TokenName}
	s.save(keyConfig, s.cfg)
	s.offline, s.lastErr, s.failures, s.dropNote = false, "", 0, ""
	s.epoch++
	s.mu.Unlock()
	s.logf("strength: connected to %s as %s (token %q)", base, who.Email, who.TokenName)

	s.Sync(ctx) // first read, and replay of anything queued; bounded by ctx
	s.notify()
	return s.Status(), nil
}

// Disconnect forgets the token, the connection, the cache and any writes
// still queued. It returns how many writes were dropped. Local habits were
// never touched and take over again.
func (s *Service) Disconnect() (int, error) {
	if err := s.tokens.Delete(); err != nil {
		return 0, fmt.Errorf("could not remove the token from %s: %w", s.tokens.Where(), err)
	}
	s.mu.Lock()
	dropped := len(s.queue)
	s.cfg = config{}
	s.cache, s.queue = nil, nil
	s.ids = idMap{Next: 1, IDs: map[string]int64{}}
	s.offline, s.lastErr, s.failures, s.dropNote = false, "", 0, ""
	s.epoch++
	s.save(keyConfig, s.cfg)
	s.save(keyToday, cached{})
	s.save(keyQueue, s.queue)
	s.save(keyIDs, s.ids)
	for id, ch := range s.waiters {
		close(ch)
		delete(s.waiters, id)
	}
	s.mu.Unlock()
	s.logf("strength: disconnected (%d queued writes dropped)", dropped)
	s.notify()
	return dropped, nil
}

// --- reading ---

func (s *Service) syncLocked() Sync {
	sy := Sync{NeedsToken: s.cfg.NeedsToken, Pending: len(s.queue), Error: s.lastErr}
	if sy.Error == "" && s.dropNote != "" && s.now().Sub(s.dropAt) < 10*time.Minute {
		sy.Error = s.dropNote
	}
	if s.cache != nil {
		t := s.cache.SyncedAt
		sy.LastSync = &t
	}
	sy.Stale = s.offline || s.cfg.NeedsToken || s.cache == nil || s.cache.Today.Date != s.today() ||
		s.now().Sub(s.cache.SyncedAt) > 3*s.refresh
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

func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Connected: s.cfg.Connected, TokenStore: s.tokens.Where()}
	if !s.cfg.Connected {
		st.Sync = Sync{State: "disconnected"}
		return st
	}
	st.BaseURL, st.Email, st.TokenName = s.cfg.BaseURL, s.cfg.Email, s.cfg.TokenName
	st.Sync = s.syncLocked()
	return st
}

// overlay is a habit's value with the queued writes for date laid over it,
// the way the phone app shows server + pending.
func (s *Service) overlay(h RemoteHabit, date string) (value float64, minHit, hit bool, pending int) {
	if s.cache.Today.Date == date {
		value, minHit, hit = h.Value, h.MetMinimum, h.MetTarget
	}
	for _, o := range s.queue {
		if o.HabitID != h.ID || o.Date != date {
			continue
		}
		pending++
		if o.Kind == opSet {
			value = o.Value
		} else {
			value = max(0, value+float64(o.Delta))
		}
	}
	if pending > 0 {
		minHit, hit = value >= h.Minimum, value >= h.Target
	}
	return
}

func (s *Service) dayLocked() Day {
	date := s.today()
	sy := s.syncLocked()
	d := Day{Date: date, Habits: []model.HabitDay{}, Source: model.SourceStrength, Sync: &sy}
	if s.cache == nil {
		return d
	}
	for _, h := range s.cache.Today.Habits {
		v, minHit, hit, pending := s.overlay(h, date)
		d.Habits = append(d.Habits, toHabitDay(h, s.ids.IDs[h.ID], date, v, minHit, hit, pending))
	}
	st := s.cache.Today.Streak
	d.Streak = st.Current
	d.StreakInfo = &StreakInfo{Current: st.Current, Best: st.Best, TodayCounted: st.TodayCounted, GraceUsed: st.GraceUsed}
	if st.LastCountedDay != nil {
		d.StreakInfo.LastCountedDay = *st.LastCountedDay
	}
	return d
}

// Habits is the habit list for a date ("" is today). Today comes from the
// cache and never waits on the network; another date is read live.
func (s *Service) Habits(ctx context.Context, date string) (Day, error) {
	s.mu.Lock()
	if !s.cfg.Connected {
		s.mu.Unlock()
		return Day{}, ErrNotConnected
	}
	if date == "" || date == s.today() {
		d := s.dayLocked()
		due := !s.cfg.NeedsToken && s.now().Sub(s.lastAttempt) > 15*time.Second
		s.mu.Unlock()
		if due {
			s.kick() // a widget is looking: freshen in the background
		}
		return d, nil
	}
	cfg := s.cfg
	sy := s.syncLocked()
	ids := s.ids.IDs
	s.mu.Unlock()

	token, err := s.tokens.Get()
	if err != nil {
		return Day{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	t, err := (&remote{base: cfg.BaseURL, token: token, http: s.http}).today(ctx, date)
	if err != nil {
		if statusOf(err) != 0 {
			return Day{}, fmt.Errorf("%w: it answered %d", ErrUnreachable, statusOf(err))
		}
		return Day{}, ErrUnreachable
	}
	d := Day{Date: t.Date, Streak: t.Streak.Current, Habits: []model.HabitDay{}, Source: model.SourceStrength, Sync: &sy}
	for _, h := range t.Habits {
		d.Habits = append(d.Habits, toHabitDay(h, ids[h.ID], t.Date, h.Value, h.MetMinimum, h.MetTarget, 0))
	}
	return d, nil
}

// fingerprint is what a widget would draw, minus the sync time while fresh.
func (s *Service) fingerprint() string {
	if !s.cfg.Connected {
		return ""
	}
	d := s.dayLocked()
	if !d.Sync.Stale {
		d.Sync.LastSync = nil
	}
	b, _ := json.Marshal(d)
	return string(b)
}

// change runs fn under the lock and notifies when the view changed.
func (s *Service) change(fn func()) {
	s.mu.Lock()
	before := s.fingerprint()
	fn()
	changed := s.fingerprint() != before
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

// --- writing ---

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Log changes today's value of a habit. The write is queued and saved
// first; the answer is the value 99-Strength confirmed, or, when it has not
// answered within LogWait (or is known to be down), the expected value with
// Pending set.
func (s *Service) Log(ctx context.Context, ref string, lop core.LogOp) (model.HabitDay, error) {
	s.mu.Lock()
	if !s.cfg.Connected {
		s.mu.Unlock()
		return model.HabitDay{}, ErrNotConnected
	}
	if s.cache == nil {
		s.mu.Unlock()
		return model.HabitDay{}, fmt.Errorf("%w: no habits have been read from 99-Strength yet", ErrUnreachable)
	}
	h, err := s.findLocked(ref)
	if err != nil {
		s.mu.Unlock()
		return model.HabitDay{}, err
	}
	date := s.today()
	cur, _, _, _ := s.overlay(h, date)
	live := !s.offline && !s.cfg.NeedsToken
	p, err := plan(h, cur, lop, live)
	if err != nil {
		s.mu.Unlock()
		return model.HabitDay{}, err
	}
	var done chan struct{}
	if !p.noop {
		o := op{ID: newID(), HabitID: h.ID, Name: h.Name, Date: date, Kind: p.kind, Delta: p.delta, Value: p.value, Weight: p.weight, At: s.now()}
		s.queue = append(s.queue, o)
		s.save(keyQueue, s.queue)
		if live && s.logWait > 0 {
			done = make(chan struct{})
			s.waiters[o.ID] = done
		}
	}
	s.mu.Unlock()

	if !p.noop {
		s.kick()
		s.notify()
	}
	if done != nil {
		t := time.NewTimer(s.logWait)
		select {
		case <-done:
		case <-t.C:
		case <-ctx.Done():
		}
		t.Stop()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil { // disconnected while we waited
		return model.HabitDay{}, ErrNotConnected
	}
	for _, c := range s.cache.Today.Habits {
		if c.ID == h.ID {
			h = c
			break
		}
	}
	v, minHit, hit, pending := s.overlay(h, date)
	return toHabitDay(h, s.ids.IDs[h.ID], date, v, minHit, hit, pending), nil
}

func (s *Service) kick() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// --- the worker ---

// Run replays queued writes and refreshes today until ctx ends: every
// Refresh while all is well, sooner after a failure, and at once when a log
// or a widget asks.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-t.C:
		}
		s.Sync(ctx)
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		t.Reset(s.nextDelay())
	}
}

func (s *Service) nextDelay() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.offline || s.failures == 0 {
		return s.refresh
	}
	d := 5 * time.Second << min(s.failures-1, 4) // 5 s, 10 s, 20 s, 40 s, 80 s
	return min(d, s.refresh)
}

// Sync runs one round now: replay the queue in order, then read today. It
// waits its turn behind a round already running, but no longer than ctx.
func (s *Service) Sync(ctx context.Context) Status {
	select {
	case s.sem <- struct{}{}:
		s.round(ctx)
		<-s.sem
	case <-ctx.Done():
	}
	return s.Status()
}

func (s *Service) round(ctx context.Context) {
	s.mu.Lock()
	cfg, epoch := s.cfg, s.epoch
	s.lastAttempt = s.now()
	s.mu.Unlock()
	if !cfg.Connected || cfg.NeedsToken {
		return
	}
	token, err := s.tokens.Get()
	if err != nil {
		s.needToken(epoch, "the stored token is missing")
		return
	}
	r := &remote{base: cfg.BaseURL, token: token, http: s.http}
	fail := func(what string, err error) {
		if unauthorized(err) {
			s.logf("strength: %s: %s", what, redact(err.Error(), token))
			s.needToken(epoch, "99-Strength no longer accepts the token")
			return
		}
		// Say so once per outage, not on every retry.
		if s.wentOffline(epoch, err) {
			s.logf("strength: offline, will keep trying: %s: %s", what, redact(err.Error(), token))
		}
	}

	for ctx.Err() == nil {
		s.mu.Lock()
		if s.epoch != epoch || len(s.queue) == 0 {
			s.mu.Unlock()
			break
		}
		o := s.queue[0]
		s.mu.Unlock()

		value, err := s.send(ctx, r, o)
		switch {
		case err == nil:
			s.sent(epoch, o, value)
		case unauthorized(err) || retryable(err):
			fail("sending "+o.Name, err)
			return
		default: // a 4xx: 99-Strength will never take this write
			s.logf("strength: dropping the write to %s: %s", o.Name, redact(err.Error(), token))
			s.dropped(epoch, o, err)
		}
	}

	t, err := r.today(ctx, s.today())
	if err != nil {
		fail("reading today", err)
		return
	}
	s.change(func() {
		if s.epoch != epoch {
			return
		}
		for _, h := range t.Habits {
			if _, ok := s.ids.IDs[h.ID]; !ok {
				s.ids.IDs[h.ID] = s.ids.Next
				s.ids.Next++
				s.save(keyIDs, s.ids)
			}
		}
		if s.offline {
			s.logf("strength: back online")
		}
		s.cache = &cached{SyncedAt: s.now(), Today: t}
		s.save(keyToday, s.cache)
		s.offline, s.lastErr, s.failures = false, "", 0
	})
}

// send performs one queued write and returns the value 99-Strength stored.
func (s *Service) send(ctx context.Context, r *remote, o op) (float64, error) {
	if o.Kind == opInc {
		l, err := r.increment(ctx, o.HabitID, o.Date, o.Delta, o.At)
		return l.Value, err
	}
	if o.Weight {
		if _, err := r.recordWeight(ctx, o.Date, o.Value); err != nil {
			return 0, err
		}
	}
	l, err := r.setLog(ctx, o.HabitID, o.Date, o.Value, o.At)
	return l.Value, err
}

func (s *Service) removeLocked(id string) {
	for i, q := range s.queue {
		if q.ID == id {
			s.queue = append(s.queue[:i:i], s.queue[i+1:]...)
			break
		}
	}
	s.save(keyQueue, s.queue)
	if ch, ok := s.waiters[id]; ok {
		close(ch)
		delete(s.waiters, id)
	}
}

func (s *Service) sent(epoch int, o op, value float64) {
	s.change(func() {
		if s.epoch != epoch {
			return
		}
		s.removeLocked(o.ID)
		if s.cache != nil && s.cache.Today.Date == o.Date {
			for i := range s.cache.Today.Habits {
				h := &s.cache.Today.Habits[i]
				if h.ID == o.HabitID {
					h.Value, h.MetMinimum, h.MetTarget = value, value >= h.Minimum, value >= h.Target
				}
			}
			s.save(keyToday, s.cache)
		}
	})
}

func (s *Service) dropped(epoch int, o op, err error) {
	s.change(func() {
		if s.epoch != epoch {
			return
		}
		s.removeLocked(o.ID)
		s.dropNote = fmt.Sprintf("99-Strength refused the change to %s (%d)", o.Name, statusOf(err))
		s.dropAt = s.now()
	})
}

// wentOffline keeps every queued write for later. Anything still queued as
// an increment becomes "set to the value it was meant to reach": 99-Strength
// has no idempotency key for increments, so an increment whose answer was
// lost could count twice if sent again, while a set cannot.
//
// It reports whether this is the start of an outage rather than more of one.
func (s *Service) wentOffline(epoch int, err error) (first bool) {
	s.change(func() {
		if s.epoch != epoch {
			return
		}
		first = !s.offline
		s.offline = true
		s.failures++
		s.lastErr = "99-Strength is not reachable"
		if st := statusOf(err); st != 0 {
			s.lastErr = fmt.Sprintf("99-Strength answered %d", st)
		}
		if len(s.queue) > 0 {
			s.queue[0].Attempts++
			s.queueAsSetsLocked()
			s.save(keyQueue, s.queue)
		}
		s.releaseLocked()
	})
	return first
}

// needToken marks the connection as needing a new token. The cache and the
// queue stay; `tcc strength connect` with a fresh token picks them up.
func (s *Service) needToken(epoch int, why string) {
	s.change(func() {
		if s.epoch != epoch {
			return
		}
		s.cfg.NeedsToken = true
		s.lastErr = why
		s.save(keyConfig, s.cfg)
		s.queueAsSetsLocked()
		s.save(keyQueue, s.queue)
		s.releaseLocked()
	})
}

// queueAsSetsLocked turns every queued increment into "set to the value it
// was meant to reach". That value is worked out now, from the last value
// 99-Strength confirmed and the writes queued before it, so it is exactly
// what the widget is showing.
func (s *Service) queueAsSetsLocked() {
	running := map[string]float64{} // habit and date -> value after the writes so far
	for i := range s.queue {
		o := &s.queue[i]
		key := o.HabitID + "|" + o.Date
		if o.Kind == opInc {
			base, ok := running[key]
			if !ok {
				base, ok = s.confirmedLocked(o.HabitID, o.Date)
			}
			if ok {
				o.Value = max(0, base+float64(o.Delta))
			}
			o.Kind, o.Delta = opSet, 0
		}
		running[key] = o.Value
	}
}

// confirmedLocked is the last value 99-Strength reported for a habit on a
// date, when the cache holds that date.
func (s *Service) confirmedLocked(habitID, date string) (float64, bool) {
	if s.cache == nil || s.cache.Today.Date != date {
		return 0, false
	}
	for _, h := range s.cache.Today.Habits {
		if h.ID == habitID {
			return h.Value, true
		}
	}
	return 0, false
}

// releaseLocked lets every waiting Log answer now with its pending value.
func (s *Service) releaseLocked() {
	for id, ch := range s.waiters {
		close(ch)
		delete(s.waiters, id)
	}
}
