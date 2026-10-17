package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"terrancc/internal/assist"
	"terrancc/internal/core"
	"terrancc/internal/events"
	"terrancc/internal/model"
	"terrancc/internal/store"
	"terrancc/internal/store/sqlite"
	"terrancc/internal/timer"
)

const tok = "secret"

type env struct {
	srv  *httptest.Server
	svc  *core.Service
	home string
}

func setup(t *testing.T) *env { return setupWith(t) }

func setupWith(t *testing.T, opts ...Option) *env {
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
	h := New(svc, timer.New(timer.DefaultConfig(), time.Now), events.NewBus(), tok, home, opts...)
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
	if len(day.Habits) != 5 || day.Habits[1].Value != 750 || day.Date == "" {
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

func TestTimerCountdownHTTP(t *testing.T) {
	e := setup(t)
	var s map[string]any
	e.do(t, "POST", "/api/timer/start", map[string]any{"minutes": 10, "label": " Get inquisitor mace "}, &s)
	if s["state"] != "running" || s["total_s"] != float64(600) || s["remaining_s"] != float64(600) || s["label"] != "Get inquisitor mace" {
		t.Fatalf("%v", s)
	}
	for _, m := range []float64{-1, 0.0001, 24 * 60} {
		if c := e.do(t, "POST", "/api/timer/start", map[string]any{"minutes": m}, nil); c != 400 {
			t.Errorf("minutes %v: got %d, want 400", m, c)
		}
	}
}

func TestAssistRunsToolsAgainstTheAPI(t *testing.T) {
	var sent []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		sent = append(sent, body)
		msg := map[string]any{"role": "assistant", "content": "Added it."}
		if len(sent) == 1 {
			msg = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c1", "type": "function",
				"function": map[string]any{"name": "add_item", "arguments": `{"title":"Get inquisitor mace","context":"osrs","when":"week"}`}}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	}))
	defer upstream.Close()
	e := setupWith(t, WithAssistant(&assist.Client{URL: upstream.URL, Model: "m", Key: func() (string, error) { return "k", nil }}))
	e.do(t, "POST", "/api/projects", map[string]any{"name": "Big Dave's Bonanza", "slug": "bdb"}, nil)

	var made model.Item
	if c := e.do(t, "POST", "/api/items", map[string]any{"title": "Exploding barrels", "project_id": "bdb", "inbox": true}, &made); c != 201 || made.ProjectID == nil {
		t.Fatalf("add by project slug: %d %+v", c, made)
	}
	e.do(t, "DELETE", fmt.Sprintf("/api/items/%d", made.ID), nil, nil)

	var r assist.Reply
	if c := e.do(t, "POST", "/api/assist", map[string]any{"session": "s1", "messages": []any{map[string]any{"role": "user", "content": "osrs short term: get inquisitor mace"}}}, &r); c != 200 {
		t.Fatalf("status %d %+v", c, r)
	}
	if r.Reply != "Added it." || len(r.Actions) != 1 || !r.Actions[0].OK {
		t.Fatalf("%+v", r)
	}
	items, _ := e.svc.Store.ListItems(context.Background(), store.ItemFilter{})
	if len(items) != 1 || items[0].Title != "Get inquisitor mace" || items[0].Context != model.ContextOSRS {
		t.Fatalf("items %+v", items)
	}
	sys := sent[0]["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "bdb") || !strings.Contains(sys, "2026") && !strings.Contains(sys, time.Now().Format("2006")) {
		t.Fatalf("system prompt lacks projects or date: %s", sys)
	}
}

func TestAssistWithoutKey(t *testing.T) {
	e := setupWith(t, WithAssistant(&assist.Client{URL: "http://127.0.0.1:1", Key: func() (string, error) { return "", errors.New("missing") }}))
	var m map[string]string
	if c := e.do(t, "POST", "/api/assist", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}, &m); c != 503 || !strings.Contains(m["error"], "OpenCode Go") {
		t.Fatalf("%d %v", c, m)
	}
	e2 := setup(t)
	if c := e2.do(t, "POST", "/api/assist", map[string]any{"messages": []any{}}, nil); c != 503 {
		t.Fatalf("no assistant configured: %d", c)
	}
}

func upload(t *testing.T, e *env, id int64, name, body string) model.Item {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write([]byte(body))
	mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/items/"+itoa(id)+"/attachments", &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload: %v %v", resp.StatusCode, err)
	}
	var it model.Item
	json.NewDecoder(resp.Body).Decode(&it)
	return it
}

func TestAttachment(t *testing.T) {
	e := setup(t)
	var it model.Item
	e.do(t, "POST", "/api/items", map[string]any{"title": "x"}, &it)
	it = upload(t, e, it.ID, `..\..\shot.png`, "png1")
	it = upload(t, e, it.ID, "shot.png", "png2")
	want := []string{"attachments/1/shot.png", "attachments/1/shot-1.png"}
	if len(it.Attachments) != 2 || it.Attachments[0] != want[0] || it.Attachments[1] != want[1] {
		t.Fatalf("%v want %v", it.Attachments, want)
	}
	if b, _ := os.ReadFile(filepath.Join(e.home, "attachments", "1", "shot.png")); string(b) != "png1" {
		t.Fatalf("file %q", b)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/items/1/attachments/shot-1.png", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("get: %v %v", resp.StatusCode, err)
	}
	if b, _ := io.ReadAll(resp.Body); string(b) != "png2" {
		t.Fatalf("served %q", b)
	}
	for _, bad := range []string{"..%2F..%2Ft.db", "missing.png"} {
		req, _ = http.NewRequest("GET", e.srv.URL+"/api/items/1/attachments/"+bad, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		if resp, _ = http.DefaultClient.Do(req); resp.StatusCode != 404 {
			t.Fatalf("%s: got %d", bad, resp.StatusCode)
		}
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

func TestShutdownEndpoint(t *testing.T) {
	home := t.TempDir()
	db, _ := sqlite.Open(filepath.Join(home, "t.db"))
	defer db.Close()
	called := make(chan struct{}, 1)
	h := New(&core.Service{Store: db, Clock: core.SystemClock{}}, timer.New(timer.DefaultConfig(), time.Now),
		events.NewBus(), tok, home, WithShutdown(func() { called <- struct{}{} }))
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/api/shutdown", "", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated shutdown: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/shutdown", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 202 {
		t.Fatalf("shutdown: %d", resp.StatusCode)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback not called")
	}
}

func TestHealthIncludesHome(t *testing.T) {
	e := setup(t)
	resp, _ := http.Get(e.srv.URL + "/api/health")
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	if m["home"] != e.home || m["pid"] == nil {
		t.Fatalf("%v", m)
	}
}

func TestCORSPreflight(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("OPTIONS", e.srv.URL+"/api/items", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "*" ||
		!strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("%d %v", resp.StatusCode, resp.Header)
	}
	// Normal responses carry the header too, including errors.
	resp, _ = http.Get(e.srv.URL + "/api/items")
	resp.Body.Close()
	if resp.StatusCode != 401 || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("%d %v", resp.StatusCode, resp.Header)
	}
}

func TestEventsTokenQuery(t *testing.T) {
	e := setup(t)
	resp, err := http.Get(e.srv.URL + "/api/events?token=" + tok)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("events with ?token: %d", resp.StatusCode)
	}
	// Only the event stream takes the token from the query.
	resp, _ = http.Get(e.srv.URL + "/api/items?token=" + tok)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("items with ?token: %d", resp.StatusCode)
	}
}

func TestUIState(t *testing.T) {
	e := setup(t)
	var got any
	if code := e.do(t, "GET", "/api/ui/boards", nil, &got); code != 200 || got != nil {
		t.Fatalf("unset: %d %v", code, got)
	}
	boards := []any{map[string]any{"id": "main", "name": "Main", "widgets": []any{}}}
	if code := e.do(t, "PUT", "/api/ui/boards", boards, nil); code != 200 {
		t.Fatalf("put %d", code)
	}
	var back []map[string]any
	if code := e.do(t, "GET", "/api/ui/boards", nil, &back); code != 200 || len(back) != 1 || back[0]["name"] != "Main" {
		t.Fatalf("get %d %v", code, back)
	}
	var note string
	e.do(t, "PUT", "/api/ui/notes", "pasted\nstuff", nil)
	if code := e.do(t, "GET", "/api/ui/notes", nil, &note); code != 200 || note != "pasted\nstuff" {
		t.Fatalf("notes: %d %q", code, note)
	}
	if code := e.do(t, "GET", "/api/ui/nope", nil, nil); code != 404 {
		t.Fatalf("unknown key %d", code)
	}
	req, _ := http.NewRequest("PUT", e.srv.URL+"/api/ui/windows", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("bad json %d", resp.StatusCode)
	}
}

// An <img> tag can't send headers, so attachment downloads take ?token= too.
func TestAttachmentTokenQuery(t *testing.T) {
	e := setup(t)
	var it model.Item
	e.do(t, "POST", "/api/items", map[string]any{"title": "x"}, &it)
	upload(t, e, it.ID, "shot.png", "png1")
	resp, err := http.Get(e.srv.URL + "/api/items/1/attachments/shot.png?token=" + tok)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "png1" {
		t.Fatalf("%d %q", resp.StatusCode, b)
	}
	resp, _ = http.Get(e.srv.URL + "/api/items/1/attachments/shot.png?token=wrong")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong token: %d", resp.StatusCode)
	}
}
