// Package strengthtest is a stand-in for the 99-Strength API: the five
// routes Terran CC calls, with the same JSON shapes and the same write rules
// as the real handlers (IncrementHabitCommandHandler, UpsertHabitLogCommandHandler,
// RecordBodyWeightCommandHandler). Tests use it through httptest; the
// fakeserver command runs it on a port for trying the widget by hand.
package strengthtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Token is a well-formed personal API token that is not real.
const Token = "nns_pat_FAKEfakeFAKEfakeFAKEfakeFAKEfakeFAKEfake123"

// dec prints like a Postgres numeric(9,2) read through System.Text.Json: 6.00.
type dec float64

func (d dec) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf("%.2f", float64(d))), nil }

type Habit struct {
	ID      string
	Name    string
	Type    string // Counter | Numeric | Checkbox
	Target  float64
	Minimum float64
	Unit    string // "" is null
	Order   int
	Icon    string // "" is null
}

type logged struct {
	value float64
	at    time.Time
}

// Request is one call the fake received.
type Request struct {
	Method, Path, Body, Authorization string
	Status                            int
}

type Fake struct {
	mu      sync.Mutex
	token   string
	habits  []Habit
	logs    map[string]map[string]*logged // date -> habit id -> log
	weights map[string]float64            // date -> kg
	reqs    []Request
	down    int // answer this status instead of working (0 = work)
	downIn  int // go down (503) after this many more writes; 0 = never
	lose    int // apply the next N writes, then cut the connection before answering
	slow    time.Duration
	streak  int // counted days before today
	Email   string
	Now     func() time.Time
}

// SeedHabits is the suggested plan's five habits (CutTo85Nov2026.Habits()).
func SeedHabits() []Habit {
	return []Habit{
		{ID: "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1001", Name: "Water (532ml bottle)", Type: "Counter", Target: 6, Minimum: 4, Unit: "bottles", Order: 1, Icon: "droplets"},
		{ID: "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1002", Name: "Walks", Type: "Counter", Target: 2, Minimum: 1, Order: 2, Icon: "footprints"},
		{ID: "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1003", Name: "Morning weigh-in", Type: "Numeric", Target: 1, Minimum: 1, Unit: "kg", Order: 3, Icon: "scale"},
		{ID: "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1004", Name: "Mobility A done", Type: "Checkbox", Target: 1, Minimum: 1, Order: 4, Icon: "stretch-horizontal"},
		{ID: "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1005", Name: "Creatine (3–5g)", Type: "Checkbox", Target: 1, Minimum: 1, Order: 5, Icon: "pill"},
	}
}

// New is a fake that accepts token and has the seed habits with nothing logged.
func New(token string) *Fake {
	return &Fake{
		token: token, habits: SeedHabits(), logs: map[string]map[string]*logged{}, weights: map[string]float64{},
		streak: 12, Email: "joe@example.com", Now: time.Now,
	}
}

func (f *Fake) SetHabits(hs []Habit) { f.mu.Lock(); f.habits = hs; f.mu.Unlock() }

// SetToken changes the token the fake accepts; "" revokes every token.
func (f *Fake) SetToken(t string) { f.mu.Lock(); f.token = t; f.mu.Unlock() }

// Down makes every call answer status (503, say) until Down(0).
func (f *Fake) Down(status int) { f.mu.Lock(); f.down, f.downIn = status, 0; f.mu.Unlock() }

// DownAfter lets n more writes through and then answers 503 until Down(0).
func (f *Fake) DownAfter(n int) { f.mu.Lock(); f.downIn = n; f.mu.Unlock() }

// LoseNext applies the next n writes but cuts the connection instead of
// answering: the client cannot know whether they happened.
func (f *Fake) LoseNext(n int) { f.mu.Lock(); f.lose = n; f.mu.Unlock() }

// Slow delays every answer.
func (f *Fake) Slow(d time.Duration) { f.mu.Lock(); f.slow = d; f.mu.Unlock() }

// Put sets a value directly, as if the phone had logged it.
func (f *Fake) Put(habitID, date string, value float64, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.day(date)[habitID] = &logged{value, at.UTC()}
}

// Value is what the fake has stored (0 when nothing was logged).
func (f *Fake) Value(habitID, date string) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l := f.logs[date][habitID]; l != nil {
		return l.value
	}
	return 0
}

// Weight is the stored body weight for a date, and whether there is one.
func (f *Fake) Weight(date string) (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kg, ok := f.weights[date]
	return kg, ok
}

// state is what Save writes and Load reads back.
type state struct {
	Logs    map[string]map[string]savedLog `json:"logs"`
	Weights map[string]float64             `json:"weights"`
}

type savedLog struct {
	Value float64   `json:"value"`
	At    time.Time `json:"at"`
}

// Save returns everything logged so far, so a restarted fake can carry on.
func (f *Fake) Save() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := state{Logs: map[string]map[string]savedLog{}, Weights: f.weights}
	for date, day := range f.logs {
		st.Logs[date] = map[string]savedLog{}
		for id, l := range day {
			st.Logs[date][id] = savedLog{l.value, l.at}
		}
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return b
}

// Load replaces what is logged with what Save returned.
func (f *Fake) Load(b []byte) error {
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs, f.weights = map[string]map[string]*logged{}, map[string]float64{}
	for date, day := range st.Logs {
		for id, l := range day {
			f.day(date)[id] = &logged{l.Value, l.At}
		}
	}
	for date, kg := range st.Weights {
		f.weights[date] = kg
	}
	return nil
}

// Requests returns every call so far, oldest first.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.reqs...)
}

// Writes returns the calls that were not GETs.
func (f *Fake) Writes() []Request {
	var out []Request
	for _, r := range f.Requests() {
		if r.Method != http.MethodGet {
			out = append(out, r)
		}
	}
	return out
}

func (f *Fake) day(date string) map[string]*logged {
	if f.logs[date] == nil {
		f.logs[date] = map[string]*logged{}
	}
	return f.logs[date]
}

func (f *Fake) habit(id string) *Habit {
	for i := range f.habits {
		if strings.EqualFold(f.habits[i].ID, id) {
			return &f.habits[i]
		}
	}
	return nil
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func problem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"status": status, "title": title, "detail": detail})
}

func ok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func offset(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000+00:00") }

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec := &recorder{ResponseWriter: w, status: 200}
	f.mu.Lock()
	slow := f.slow
	f.mu.Unlock()
	if slow > 0 {
		select {
		case <-time.After(slow):
		case <-r.Context().Done():
		}
	}
	lost := f.serve(rec, r, body)
	f.mu.Lock()
	if lost {
		rec.status = 0
	}
	f.reqs = append(f.reqs, Request{Method: r.Method, Path: r.URL.RequestURI(), Body: string(body), Authorization: r.Header.Get("Authorization"), Status: rec.status})
	f.mu.Unlock()
	if lost {
		if hj, ok := w.(http.Hijacker); ok {
			if c, _, err := hj.Hijack(); err == nil {
				c.Close()
			}
		}
	}
}

// serve handles one call. It returns true when the write was applied but
// the answer must be lost.
func (f *Fake) serve(w http.ResponseWriter, r *http.Request, body []byte) (lost bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down != 0 {
		problem(w, f.down, "Service Unavailable", "The fake is down.")
		return false
	}
	path, okPrefix := strings.CutPrefix(r.URL.Path, "/api/v1")
	if !okPrefix {
		problem(w, 404, "Not Found", "")
		return false
	}
	if f.token == "" || r.Header.Get("Authorization") != "Bearer "+f.token {
		problem(w, 401, "Unauthenticated", "The API token is unknown or revoked.")
		return false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	write := r.Method != http.MethodGet
	if write && f.downIn > 0 {
		if f.downIn--; f.downIn == 0 {
			defer func() { f.down = 503 }() // this write still goes through
		}
	}
	loseIt := func() bool {
		if write && f.lose > 0 {
			f.lose--
			return true
		}
		return false
	}

	switch {
	case r.Method == "GET" && path == "/users/me/api-tokens/whoami":
		ok(w, map[string]any{"userId": "7b0e7c52-93a1-4e0c-8a3e-5a8d6f0b2c11", "email": f.Email, "tokenName": "Terran CC"})

	case r.Method == "GET" && path == "/habits/today":
		date := r.URL.Query().Get("date")
		if date == "" {
			date = f.Now().Format("2006-01-02")
		}
		if !validDate(date) {
			problem(w, 400, "Bad Request", "The value '"+date+"' is not valid.")
			return false
		}
		ok(w, f.today(date))

	case r.Method == "PUT" && len(parts) == 4 && parts[0] == "habits" && parts[2] == "logs":
		var req struct {
			Value           *float64   `json:"value"`
			ClientTimestamp *time.Time `json:"clientTimestamp"`
		}
		h := f.habit(parts[1])
		if json.Unmarshal(body, &req) != nil || req.Value == nil || req.ClientTimestamp == nil || !validDate(parts[3]) {
			problem(w, 400, "Bad Request", "One or more validation errors occurred.")
			return false
		}
		if h == nil {
			problem(w, 404, "Not Found", "Habit not found.")
			return false
		}
		if *req.Value < 0 {
			problem(w, 422, "Business Rule Violation", "Habit value cannot be negative.")
			return false
		}
		day := f.day(parts[3])
		l := day[h.ID]
		at := req.ClientTimestamp.UTC()
		if l == nil {
			l = &logged{*req.Value, at}
			day[h.ID] = l
		} else if !at.Before(l.at) { // last write wins; an older one is ignored
			l.value, l.at = *req.Value, at
		}
		if loseIt() {
			return true
		}
		ok(w, logDTO(h.ID, parts[3], l))

	case r.Method == "POST" && len(parts) == 5 && parts[0] == "habits" && parts[2] == "logs" && parts[4] == "increment":
		var req struct {
			Delta           *int       `json:"delta"`
			ClientTimestamp *time.Time `json:"clientTimestamp"`
		}
		h := f.habit(parts[1])
		if json.Unmarshal(body, &req) != nil || req.Delta == nil || req.ClientTimestamp == nil || !validDate(parts[3]) {
			problem(w, 400, "Bad Request", "One or more validation errors occurred.")
			return false
		}
		if h == nil {
			problem(w, 404, "Not Found", "Habit not found.")
			return false
		}
		day := f.day(parts[3])
		l := day[h.ID]
		at := req.ClientTimestamp.UTC()
		switch {
		case l == nil: // the real handler starts at 1 or 0 whatever the delta
			l = &logged{0, at}
			if *req.Delta > 0 {
				l.value = 1
			}
			day[h.ID] = l
		default:
			if at.After(l.at) { // the stored timestamp never moves backwards
				l.at = at
			}
			switch {
			case h.Type == "Checkbox" && *req.Delta > 0:
				l.value = 1
			case h.Type == "Checkbox":
				l.value = 0
			default:
				l.value = max(0, l.value+float64(*req.Delta))
			}
		}
		if loseIt() {
			return true
		}
		ok(w, logDTO(h.ID, parts[3], l))

	case r.Method == "PUT" && len(parts) == 2 && parts[0] == "body-weight":
		var req struct {
			Kg *float64 `json:"kg"`
		}
		if json.Unmarshal(body, &req) != nil || req.Kg == nil || !validDate(parts[1]) {
			problem(w, 400, "Bad Request", "One or more validation errors occurred.")
			return false
		}
		if *req.Kg < 30 || *req.Kg > 300 {
			problem(w, 422, "Business Rule Violation", "Body weight must be between 30 and 300 kg.")
			return false
		}
		f.weights[parts[1]] = *req.Kg
		if loseIt() {
			return true
		}
		ok(w, map[string]any{"date": parts[1], "kg": dec(*req.Kg), "recordedAtUtc": offset(f.Now())})

	default:
		problem(w, 404, "Not Found", "")
	}
	return false
}

func logDTO(habitID, date string, l *logged) map[string]any {
	return map[string]any{"habitId": habitID, "date": date, "value": dec(l.value), "updatedAtUtc": offset(l.at)}
}

// today builds HabitsTodayDto. The streak is simplified: a fixed run of
// earlier days, plus today once every habit has reached its minimum.
func (f *Fake) today(date string) map[string]any {
	habits := []map[string]any{}
	all := len(f.habits) > 0
	for _, h := range f.habits {
		l := f.logs[date][h.ID]
		v := 0.0
		if l != nil {
			v = l.value
		}
		metMin, metTarget := l != nil && v >= h.Minimum, l != nil && v >= h.Target
		all = all && metMin
		habits = append(habits, map[string]any{
			"id": h.ID, "name": h.Name, "type": h.Type, "target": dec(h.Target), "minimum": dec(h.Minimum), "unit": ptr(h.Unit),
			"value": dec(v), "metMinimum": metMin, "metTarget": metTarget, "order": h.Order, "icon": ptr(h.Icon),
		})
	}
	current := f.streak
	var last *string
	if d, err := time.Parse("2006-01-02", date); err == nil && f.streak > 0 {
		y := d.AddDate(0, 0, -1).Format("2006-01-02")
		last = &y
	}
	if all {
		current++
		last = &date
	}
	return map[string]any{
		"date": date, "habits": habits,
		"streak": map[string]any{"current": current, "best": max(current, 19), "lastCountedDay": last, "todayCounted": all, "graceUsed": false},
	}
}
