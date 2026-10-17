package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/model"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/timer"
)

const tok = "secret"

type env struct {
	srv  *httptest.Server
	svc  *core.Service
	home string
}

func setup(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	db, err := sqlite.Open(filepath.Join(home, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &core.Service{Store: db, Clock: core.SystemClock{}}
	if err := svc.SeedHabits(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := New(svc, timer.New(timer.DefaultConfig(), time.Now), events.NewBus(), tok, home)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, svc: svc, home: home}
}

func (e *env) do(t *testing.T, method, path string, body any, out any) int {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestHealthNoAuth(t *testing.T) {
	e := setup(t)
	resp, err := http.Get(e.srv.URL + "/api/health")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	if m["ok"] != true || m["version"] == "" {
		t.Fatalf("%v", m)
	}
}

func TestAuthRequired(t *testing.T) {
	e := setup(t)
	resp, _ := http.Get(e.srv.URL + "/api/items")
	if resp.StatusCode != 401 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/items", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Fatalf("wrong token got %d", resp.StatusCode)
	}
}

func TestItemLifecycleHTTP(t *testing.T) {
	e := setup(t)
	var p model.Project
	if c := e.do(t, "POST", "/api/projects", map[string]any{"name": "BDTV Bonanza", "repo_path": `C:\repo`}, &p); c != 201 {
		t.Fatalf("project %d", c)
	}
	var it model.Item
	if c := e.do(t, "POST", "/api/items", map[string]any{"title": "Fix bot melee", "project_id": p.ID, "kind": "bug"}, &it); c != 201 {
		t.Fatalf("item %d", c)
	}
	path := "/api/items/" + itoa(it.ID)
	e.do(t, "POST", path+"/start", map[string]any{"branch": "fix/melee", "session": "bdb-t2"}, &it)
	if it.Status != model.StatusInProgress || it.Branch != "fix/melee" {
		t.Fatalf("start %+v", it)
	}
	e.do(t, "POST", path+"/note", map[string]any{"text": "bots swing guns"}, &it)
	e.do(t, "POST", path+"/evidence", map[string]any{"evidence": []map[string]string{{"type": "commit", "value": "abc123"}}}, &it)
	e.do(t, "POST", path+"/review", nil, &it)
	var got model.Item
	e.do(t, "GET", path, nil, &got)
	if got.Status != model.StatusReview || len(got.Evidence) != 1 || !strings.Contains(got.Notes, "bots swing guns") {
		t.Fatalf("final %+v", got)
	}
	var list []model.Item
	e.do(t, "GET", "/api/items?project=bdtv-bonanza&status=review,done", nil, &list)
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	if c := e.do(t, "DELETE", path, nil, nil); c != 204 {
		t.Fatalf("delete %d", c)
	}
}

func TestPatchItem(t *testing.T) {
	e := setup(t)
	var p model.Project
	e.do(t, "POST", "/api/projects", map[string]any{"name": "P"}, &p)
	var it model.Item
	e.do(t, "POST", "/api/items", map[string]any{"title": "todo"}, &it)
	path := "/api/items/" + itoa(it.ID)
	e.do(t, "PATCH", path, map[string]any{"project_id": "p", "when": "today", "model": "Opus 5.5"}, &it)
	if it.ProjectID == nil || *it.ProjectID != p.ID || it.Kind != model.KindTask || it.When != model.WhenToday || it.Model != "Opus 5.5" {
		t.Fatalf("patch %+v", it)
	}
	e.do(t, "PATCH", path, map[string]any{"project_id": nil}, &it)
	if it.ProjectID != nil {
		t.Fatalf("clear project %+v", it)
	}
}

func TestProjectBySlug(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", "/api/projects", map[string]any{"name": "BDTV Bonanza"}, nil)
	var p model.Project
	if c := e.do(t, "GET", "/api/projects/bdtv-bonanza", nil, &p); c != 200 || p.Name != "BDTV Bonanza" {
		t.Fatalf("%d %+v", c, p)
	}
	e.do(t, "PATCH", "/api/projects/bdtv-bonanza", map[string]any{"repo_path": `D:\x`}, &p)
	if p.RepoPath != `D:\x` {
		t.Fatalf("%+v", p)
	}
	if c := e.do(t, "POST", "/api/projects", map[string]any{"name": "BDTV Bonanza"}, nil); c != 409 {
		t.Fatalf("dup %d", c)
	}
}

func TestErrorMapping(t *testing.T) {
	e := setup(t)
	var m map[string]string
	if c := e.do(t, "GET", "/api/items/999", nil, &m); c != 404 || !strings.Contains(m["error"], "#999") {
		t.Fatalf("%d %v", c, m)
	}
	if c := e.do(t, "POST", "/api/items", map[string]any{"title": "x", "when": "tomorrow"}, &m); c != 400 || !strings.Contains(m["error"], "when") {
		t.Fatalf("%d %v", c, m)
	}
	if c := e.do(t, "GET", "/api/items/abc", nil, &m); c != 400 {
		t.Fatalf("bad id %d", c)
	}
	if c := e.do(t, "POST", "/api/items", "not an object", &m); c != 400 {
		t.Fatalf("bad json %d", c)
	}
	if c := e.do(t, "GET", "/api/items?project=nope", nil, &m); c != 404 {
		t.Fatalf("unknown project %d", c)
	}
}

func TestHabitLogHTTP(t *testing.T) {
	e := setup(t)
	var d model.HabitDay
	if c := e.do(t, "POST", "/api/habits/water/log", map[string]any{}, &d); c != 200 || d.Value != 250 {
		t.Fatalf("%d %+v", c, d)
	}
	e.do(t, "POST", "/api/habits/water/log", map[string]any{"delta": 500}, &d)
	if d.Value != 750 {
		t.Fatalf("%+v", d)
	}
	var day struct {
		Date   string           `json:"date"`
		Streak int              `json:"streak"`
		Habits []model.HabitDay `json:"habits"`
	}
	e.do(t, "GET", "/api/habits", nil, &day)
	if len(day.Habits) != 4 || day.Habits[1].Value != 750 || day.Date == "" {
		t.Fatalf("%+v", day)
	}
	var m map[string]string
	if c := e.do(t, "POST", "/api/habits/wa/log", map[string]any{}, &m); c != 400 || !strings.Contains(m["error"], "ambiguous") {
		t.Fatalf("%d %v", c, m)
	}
}

func TestTimerToggleHTTP(t *testing.T) {
	e := setup(t)
	var s map[string]any
	e.do(t, "POST", "/api/timer/toggle", nil, &s)
	if s["state"] != "running" {
		t.Fatalf("%v", s)
	}
	e.do(t, "PUT", "/api/timer/config", map[string]any{"focus_min": 50, "short_min": 10, "long_min": 20, "long_every": 4}, &s)
	e.do(t, "POST", "/api/timer/stop", nil, &s)
	if s["remaining_s"] != float64(3000) {
		t.Fatalf("config not applied: %v", s)
	}
	v, ok, _ := e.svc.Store.GetSetting(context.Background(), "timer.config")
	if !ok || !strings.Contains(v, `"focus_min":50`) {
		t.Fatalf("config not persisted: %q", v)
	}
}

func TestAttachment(t *testing.T) {
	e := setup(t)
	var it model.Item
	e.do(t, "POST", "/api/items", map[string]any{"title": "x"}, &it)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", `..\..\shot.png`)
	fw.Write([]byte("png"))
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/items/"+itoa(it.ID)+"/attachments", &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp.StatusCode, err)
	}
	json.NewDecoder(resp.Body).Decode(&it)
	want := filepath.Join(e.home, "attachments", itoa(it.ID), "shot.png")
	if len(it.Attachments) != 1 || it.Attachments[0] != want {
		t.Fatalf("%v want %s", it.Attachments, want)
	}
	if b, _ := os.ReadFile(want); string(b) != "png" {
		t.Fatalf("file %q", b)
	}
}

func TestSSE(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	got := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event: item.changed") {
				got <- sc.Text()
				return
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	e.do(t, "POST", "/api/items", map[string]any{"title": "x"}, nil)
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("no item.changed event")
	}
}
