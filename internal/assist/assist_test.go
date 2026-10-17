package assist

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeModel answers each chat request with the next canned message and
// records what it was sent.
type fakeModel struct {
	replies []map[string]any
	got     []map[string]any
	headers []http.Header
}

func (f *fakeModel) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.got = append(f.got, body)
		f.headers = append(f.headers, r.Header.Clone())
		msg := f.replies[min(len(f.got), len(f.replies))-1]
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakeCall(id, name, args string) map[string]any {
	return map[string]any{"role": "assistant", "tool_calls": []any{
		map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": args}},
	}}
}

type call struct {
	method, path string
	body         map[string]any
}

func recorder(calls *[]call) Doer {
	return func(method, path string, body any) (int, []byte) {
		c := call{method: method, path: path}
		if body != nil {
			b, _ := json.Marshal(body)
			json.Unmarshal(b, &c.body)
		}
		*calls = append(*calls, c)
		return 200, []byte(`{"id":45,"title":"Buy milk","status":"backlog"}`)
	}
}

func client(url string) *Client {
	return &Client{URL: url, Model: "kimi-k3", Key: func() (string, error) { return "k", nil }}
}

func TestRunExecutesToolsThenReplies(t *testing.T) {
	f := &fakeModel{replies: []map[string]any{
		fakeCall("c1", "add_item", `{"title":"Buy milk","when":"today","context":"irl"}`),
		{"role": "assistant", "content": "Added Buy milk to today."},
	}}
	srv := f.server(t)
	var calls []call
	r, err := client(srv.URL).Run(context.Background(), "s-1", "system", []Message{{Role: "user", Content: "buy milk today"}}, recorder(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if r.Reply != "Added Buy milk to today." {
		t.Fatalf("reply %q", r.Reply)
	}
	if len(calls) != 1 || calls[0].method != "POST" || calls[0].path != "/api/items" || calls[0].body["title"] != "Buy milk" || calls[0].body["context"] != "irl" {
		t.Fatalf("calls %+v", calls)
	}
	if len(r.Actions) != 1 || !r.Actions[0].OK || r.Actions[0].Tool != "add_item" || !strings.Contains(r.Actions[0].Summary, "#45") {
		t.Fatalf("actions %+v", r.Actions)
	}
	h := f.headers[0]
	if h.Get("Authorization") != "Bearer k" || h.Get("x-opencode-session") != "s-1" || !strings.HasPrefix(h.Get("User-Agent"), "terran-cc/") {
		t.Fatalf("headers %v", h)
	}
	// The second request carries the tool result back to the model.
	msgs := f.got[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "c1" || !strings.Contains(last["content"].(string), "Buy milk") {
		t.Fatalf("tool message %v", last)
	}
	if msgs[0].(map[string]any)["role"] != "system" || f.got[0]["model"] != "kimi-k3" || f.got[0]["tools"] == nil {
		t.Fatalf("first request %v", f.got[0])
	}
}

func TestToolsMapToAPI(t *testing.T) {
	for _, c := range []struct {
		tool, args, method, path string
	}{
		{"list_items", `{"project":"bdb","status":"review,in_progress"}`, "GET", "/api/items?project=bdb&status=review%2Cin_progress"},
		{"edit_item", `{"id":7,"when":"week"}`, "PATCH", "/api/items/7"},
		{"complete_item", `{"id":7}`, "POST", "/api/items/7/done"},
		{"add_note", `{"id":7,"text":"hi"}`, "POST", "/api/items/7/note"},
		{"log_habit", `{"habit":"water","delta":500}`, "POST", "/api/habits/water/log"},
		{"start_timer", `{"minutes":25,"task":"Raid prep"}`, "POST", "/api/timer/start"},
		{"osrs_status", `{}`, "GET", "/api/osrs"},
		{"search_notes", `{"query":"raid prep"}`, "GET", "/api/vault/search?q=raid+prep"},
		{"read_note", `{"path":"Hobbies/OSRS/goals.md"}`, "GET", "/api/vault/note?path=Hobbies%2FOSRS%2Fgoals.md"},
	} {
		var calls []call
		if _, err := runTool(c.tool, c.args, recorder(&calls)); err != nil {
			t.Errorf("%s: %v", c.tool, err)
			continue
		}
		if len(calls) != 1 || calls[0].method != c.method || calls[0].path != c.path {
			t.Errorf("%s: %+v", c.tool, calls)
		}
	}
	var calls []call
	runTool("edit_item", `{"id":7,"when":"week"}`, recorder(&calls))
	if _, has := calls[0].body["id"]; has || calls[0].body["when"] != "week" {
		t.Errorf("edit body %v", calls[0].body)
	}
	runTool("start_timer", `{"minutes":25,"task":"Raid prep"}`, recorder(&calls))
	if calls[1].body["label"] != "Raid prep" || calls[1].body["minutes"] != float64(25) {
		t.Errorf("timer body %v", calls[1].body)
	}
	if _, err := runTool("nope", `{}`, recorder(&calls)); err == nil {
		t.Error("unknown tool should fail")
	}
}

func TestHabitSummary(t *testing.T) {
	do := func(method, path string, body any) (int, []byte) {
		return 200, []byte(`{"id":2,"name":"Water","unit":"ml","value":750,"target":3000}`)
	}
	if a, _ := runTool("log_habit", `{"habit":"water","delta":250}`, do); a.Summary != "Water 750/3000 ml" {
		t.Fatalf("%q", a.Summary)
	}
}

func TestRunStopsLooping(t *testing.T) {
	f := &fakeModel{replies: []map[string]any{fakeCall("c", "list_projects", `{}`)}}
	srv := f.server(t)
	var calls []call
	_, err := client(srv.URL).Run(context.Background(), "s", "", []Message{{Role: "user", Content: "x"}}, recorder(&calls))
	if err == nil || len(f.got) != MaxRounds {
		t.Fatalf("err %v after %d requests", err, len(f.got))
	}
}

func TestRunWithoutKey(t *testing.T) {
	c := client("http://127.0.0.1:1")
	c.Key = func() (string, error) { return "", errors.New("no key") }
	_, err := c.Run(context.Background(), "s", "", []Message{{Role: "user", Content: "x"}}, nil)
	if !errors.Is(err, ErrNoKey) {
		t.Fatalf("%v", err)
	}
}

func TestOpencodeKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	os.WriteFile(p, []byte(`{"opencode":{"type":"api","key":"other"},"opencode-go":{"type":"api","key":"go-key"}}`), 0o600)
	if k, err := OpencodeKey(p)(); err != nil || k != "go-key" {
		t.Fatalf("%q %v", k, err)
	}
	os.WriteFile(p, []byte(`{"opencode":{"type":"api","key":"other"}}`), 0o600)
	if _, err := OpencodeKey(p)(); err == nil {
		t.Fatal("missing opencode-go should fail")
	}
	if _, err := OpencodeKey(filepath.Join(dir, "none.json"))(); err == nil {
		t.Fatal("missing file should fail")
	}
	_ = io.Discard
}
