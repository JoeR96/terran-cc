package strength

// testdata/ holds one sample per 99-Strength endpoint Terran CC calls. They
// were written by hand from the DTOs in 99-Strength
// (A2S.Application/DTOs/HabitDtos.cs and BodyWeightDtos.cs,
// A2S.Api/Contracts/Requests/HabitRequests.cs) while v2 was not deployed;
// they are not recordings. Property names are camelCase, "type" is a string
// enum, dates are London yyyy-MM-dd and decimals come back with two places.
//
//	whoami.json                    GET  /users/me/api-tokens/whoami
//	habits_today.json              GET  /habits/today?date=2026-11-09
//	habit_increment.{request,response}.json  POST /habits/{id}/logs/{date}/increment
//	habit_set_log.{request,response}.json    PUT  /habits/{id}/logs/{date}
//	body_weight_put.{request,response}.json  PUT  /body-weight/{date}
//	problem_401.json               any call with an unknown or revoked token
//
// If 99-Strength changes a shape, change the file: these tests then show
// what breaks in the client and in the fake server.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"terrancc/internal/strength/strengthtest"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type call struct{ method, path, auth, body string }

// replay serves one fixture and records the call that asked for it.
func replay(t *testing.T, status int, name string) (*remote, *call) {
	t.Helper()
	got := &call{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = call{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), string(b)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(fixture(t, name))
	}))
	t.Cleanup(srv.Close)
	return &remote{base: srv.URL + "/api/v1", token: strengthtest.Token, http: srv.Client()}, got
}

const (
	waterID   = "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1001"
	walksID   = "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1002"
	weighID   = "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1003"
	mobID     = "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1004"
	creaID    = "0f6c2b7e-5d43-4c1a-9a57-2f1f4a7c1005"
	fixDate   = "2026-11-09"
	fixBearer = "Bearer " + strengthtest.Token
)

var tapTime = time.Date(2026, 11, 9, 9, 15, 2, 250_000_000, time.UTC)

func sameJSON(t *testing.T, what, got string, want []byte) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("%s: body %q is not JSON: %v", what, got, err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s: sent %s, the contract is %s", what, got, strings.TrimSpace(string(want)))
	}
}

func TestWhoAmIContract(t *testing.T) {
	r, got := replay(t, 200, "whoami.json")
	w, err := r.whoAmI(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if *got != (call{"GET", "/api/v1/users/me/api-tokens/whoami", fixBearer, ""}) {
		t.Fatalf("call: %+v", *got)
	}
	if w != (WhoAmI{UserID: "7b0e7c52-93a1-4e0c-8a3e-5a8d6f0b2c11", Email: "joe@example.com", TokenName: "Terran CC"}) {
		t.Fatalf("whoami: %+v", w)
	}
}

func TestTodayContract(t *testing.T) {
	r, got := replay(t, 200, "habits_today.json")
	d, err := r.today(context.Background(), fixDate)
	if err != nil {
		t.Fatal(err)
	}
	if *got != (call{"GET", "/api/v1/habits/today?date=2026-11-09", fixBearer, ""}) {
		t.Fatalf("call: %+v", *got)
	}
	if d.Date != fixDate || len(d.Habits) != 5 {
		t.Fatalf("today: %+v", d)
	}
	bottles, kg := "bottles", "kg"
	icon := "droplets"
	water := RemoteHabit{ID: waterID, Name: "Water (532ml bottle)", Type: "Counter", Target: 6, Minimum: 4, Unit: &bottles, Value: 3, Order: 1, Icon: &icon}
	if !reflect.DeepEqual(d.Habits[0], water) {
		t.Fatalf("water: %+v", d.Habits[0])
	}
	if w := d.Habits[2]; w.Type != "Numeric" || w.Value != 94.6 || *w.Unit != kg || !w.MetMinimum || !w.MetTarget {
		t.Fatalf("weigh-in: %+v", w)
	}
	if c := d.Habits[4]; c.Type != "Checkbox" || c.Unit != nil || c.Value != 1 || c.Name != "Creatine (3–5g)" {
		t.Fatalf("creatine: %+v", c)
	}
	last := "2026-11-08"
	if !reflect.DeepEqual(d.Streak, RemoteStreak{Current: 12, Best: 19, LastCountedDay: &last, GraceUsed: true}) {
		t.Fatalf("streak: %+v", d.Streak)
	}
}

func TestIncrementContract(t *testing.T) {
	r, got := replay(t, 200, "habit_increment.response.json")
	l, err := r.increment(context.Background(), waterID, fixDate, 1, tapTime)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "POST" || got.path != "/api/v1/habits/"+waterID+"/logs/2026-11-09/increment" || got.auth != fixBearer {
		t.Fatalf("call: %+v", *got)
	}
	sameJSON(t, "increment", got.body, fixture(t, "habit_increment.request.json"))
	if l.HabitID != waterID || l.Date != fixDate || l.Value != 4 || !l.UpdatedAtUTC.Equal(tapTime) {
		t.Fatalf("log: %+v", l)
	}
}

func TestSetLogContract(t *testing.T) {
	r, got := replay(t, 200, "habit_set_log.response.json")
	l, err := r.setLog(context.Background(), mobID, fixDate, 1, tapTime)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "PUT" || got.path != "/api/v1/habits/"+mobID+"/logs/2026-11-09" || got.auth != fixBearer {
		t.Fatalf("call: %+v", *got)
	}
	sameJSON(t, "set", got.body, fixture(t, "habit_set_log.request.json"))
	if l.HabitID != mobID || l.Value != 1 {
		t.Fatalf("log: %+v", l)
	}
}

func TestBodyWeightContract(t *testing.T) {
	r, got := replay(t, 200, "body_weight_put.response.json")
	w, err := r.recordWeight(context.Background(), fixDate, 94.6)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != "PUT" || got.path != "/api/v1/body-weight/2026-11-09" || got.auth != fixBearer {
		t.Fatalf("call: %+v", *got)
	}
	sameJSON(t, "body weight", got.body, fixture(t, "body_weight_put.request.json"))
	if w.Date != fixDate || w.Kg != 94.6 || w.RecordedAtUTC.IsZero() {
		t.Fatalf("weight: %+v", w)
	}
}

func TestProblemContract(t *testing.T) {
	r, _ := replay(t, 401, "problem_401.json")
	_, err := r.today(context.Background(), fixDate)
	if !unauthorized(err) || retryable(err) {
		t.Fatalf("a 401 must mean the token, and must not be retried: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown or revoked") {
		t.Fatalf("the problem detail should be kept: %v", err)
	}
	if strings.Contains(err.Error(), strengthtest.Token) {
		t.Fatal("error text holds the token")
	}
}

func keys(t *testing.T, raw []byte) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// The fake server is what every other test trusts, so it has to answer
// with the fixtures' shapes: same properties, on every endpoint.
func TestFakeMatchesFixtures(t *testing.T) {
	fake := strengthtest.New(strengthtest.Token)
	srv := httptest.NewServer(fake)
	defer srv.Close()
	do := func(method, path, body string) []byte {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+"/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Authorization", fixBearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return b
	}
	same := func(what string, got []byte, name string) {
		t.Helper()
		if g, w := keys(t, got), keys(t, fixture(t, name)); !reflect.DeepEqual(g, w) {
			t.Errorf("%s: the fake answers %v, the fixture has %v", what, g, w)
		}
	}
	same("whoami", do("GET", "/users/me/api-tokens/whoami", ""), "whoami.json")
	same("increment", do("POST", "/habits/"+waterID+"/logs/2026-11-09/increment", string(fixture(t, "habit_increment.request.json"))), "habit_increment.response.json")
	same("set", do("PUT", "/habits/"+mobID+"/logs/2026-11-09", string(fixture(t, "habit_set_log.request.json"))), "habit_set_log.response.json")
	same("body weight", do("PUT", "/body-weight/2026-11-09", string(fixture(t, "body_weight_put.request.json"))), "body_weight_put.response.json")

	today := do("GET", "/habits/today?date=2026-11-09", "")
	same("today", today, "habits_today.json")
	var got, want struct {
		Habits []json.RawMessage `json:"habits"`
		Streak json.RawMessage   `json:"streak"`
	}
	json.Unmarshal(today, &got)
	json.Unmarshal(fixture(t, "habits_today.json"), &want)
	if len(got.Habits) != len(want.Habits) {
		t.Fatalf("the fake has %d habits, the fixture %d", len(got.Habits), len(want.Habits))
	}
	if g, w := keys(t, got.Habits[0]), keys(t, want.Habits[0]); !reflect.DeepEqual(g, w) {
		t.Errorf("today habit: the fake answers %v, the fixture has %v", g, w)
	}
	if g, w := keys(t, got.Streak), keys(t, want.Streak); !reflect.DeepEqual(g, w) {
		t.Errorf("streak: the fake answers %v, the fixture has %v", g, w)
	}

	fake.SetToken("")
	same("401", do("GET", "/habits/today", ""), "problem_401.json")
}
