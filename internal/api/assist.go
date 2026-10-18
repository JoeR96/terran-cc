package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"terrancc/internal/assist"
)

// WithAssistant enables POST /api/assist, the chat box's model in the loop.
func WithAssistant(c *assist.Client) Option { return func(s *server) { s.assistant = c } }

func (s *server) assistRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/assist", func(w http.ResponseWriter, r *http.Request) {
		if s.assistant == nil {
			writeJSON(w, 503, map[string]string{"error": "the assistant is not set up in this tccd"})
			return
		}
		var body struct {
			Session  string           `json:"session"`
			Messages []assist.Message `json:"messages"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if len(body.Messages) == 0 {
			writeErr(w, badRequest("messages is empty"))
			return
		}
		if body.Session == "" {
			body.Session = "tcc-chat"
		}
		reply, err := s.assistant.Run(r.Context(), body.Session, s.assistPrompt(r), body.Messages, s.inProcess(mux))
		switch {
		case errors.Is(err, assist.ErrNoKey):
			writeJSON(w, 503, map[string]string{"error": "OpenCode Go isn't connected: " + err.Error()})
		case err != nil:
			// Actions already taken still reach the chat with the error.
			writeJSON(w, 502, map[string]any{"error": err.Error(), "actions": reply.Actions})
		default:
			writeJSON(w, 200, reply)
		}
	})
}

// inProcess lets the assistant call this API directly, past the token check,
// so its changes are validated and broadcast like any other client's.
func (s *server) inProcess(mux *http.ServeMux) assist.Doer {
	return func(method, path string, body any) (int, []byte) {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}
}

func (s *server) assistPrompt(r *http.Request) string {
	now := s.svc.Clock.Now().Local()
	var projects []string
	if ps, err := s.svc.Store.ListProjects(r.Context(), false); err == nil {
		for _, p := range ps {
			projects = append(projects, fmt.Sprintf("%s (%s)", p.Slug, p.Name))
		}
	}
	return fmt.Sprintf(`You are the assistant inside Terran CC, Joe's command centre: personal to-dos, project tickets, daily habits and a countdown timer.
Today is %s.
Projects: %s.

How things are organised:
- Personal to-dos have no project. context is irl ("me"), work, osrs (Old School RuneScape goals) or tech (technology to learn or revise for interviews). when is today, week or month; OSRS uses today, short term = week, medium term = month, long term = anytime.
- To-dos and tickets nest with parent_id, e.g. "Prep for raids S4" > "Get inquisitor mace". Features group a project's tickets.
- Project tickets move inbox > backlog > up_next > in_progress > review > done. A ticket with when today/week/month is a goal for that period.

How to work:
- Do what Joe asks with the tools, then answer in one or two short sentences saying what changed, with #ids.
- Look things up with list_items or list_projects before editing; never guess ids.
- A new idea for a project goes to that project's inbox. Use add_note for progress updates on existing tickets.
- For OSRS questions (what's ready, what's low, how far to a level) use osrs_status. Big Chopski is an ironman.
- Joe's older notes live in Obsidian: use search_notes and read_note to look things up there. They are read only.
- Only delete when Joe clearly says delete. If a request is ambiguous, ask one short question instead of guessing.`,
		now.Format("Monday 2 January 2006, 15:04"), strings.Join(projects, ", "))
}
