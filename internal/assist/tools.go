package assist

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type args = map[string]any

// tool is one thing the model can do. call turns its arguments into a tccd
// API request, so the model gets the same validation and live updates as
// the app.
type tool struct {
	name, desc string
	props      args
	required   []string
	call       func(a args) (method, path string, body any)
}

func prop(typ, desc string) args { return args{"type": typ, "description": desc} }

var (
	idProp      = prop("integer", "item id, e.g. 45 for #45")
	projectProp = prop("string", "project slug, e.g. bdb")
	whenProp    = args{"type": "string", "enum": []string{"today", "week", "month", "anytime"}, "description": "when it's for. For OSRS: today, short term = week, medium term = month, long term = anytime"}
	ctxProp     = args{"type": "string", "enum": []string{"irl", "work", "osrs"}, "description": "irl = personal (me), work, or osrs (Old School RuneScape goals)"}
	kindProp    = args{"type": "string", "enum": []string{"todo", "bug", "task", "feature", "idea"}}
	statusProp  = args{"type": "string", "enum": []string{"inbox", "backlog", "up_next", "in_progress", "review", "done", "someday"}}
)

func itemPath(a args, suffix string) string {
	return fmt.Sprintf("/api/items/%v%s", num(a["id"]), suffix)
}

// num prints JSON numbers as integers ("45", not "45.0").
func num(v any) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprint(v)
}

// fields copies the named arguments into a request body, renaming project
// to project_id, which the API accepts as a slug.
func fields(a args, names ...string) args {
	b := args{}
	for _, n := range names {
		if v, ok := a[n]; ok {
			if n == "project" {
				n = "project_id"
			}
			b[n] = v
		}
	}
	return b
}

var tools = []tool{
	{"list_items", "List to-dos and tickets. Done and someday are hidden unless status asks for them. Use before editing to find ids.",
		args{"project": projectProp, "status": prop("string", "comma-separated statuses"), "when": prop("string", "comma-separated: today,week,month,anytime"),
			"context": ctxProp, "kind": prop("string", "comma-separated kinds"), "q": prop("string", "text to search titles and notes"),
			"personal": prop("boolean", "only personal to-dos (no project)")}, nil,
		func(a args) (string, string, any) {
			q := url.Values{}
			for _, k := range []string{"project", "status", "when", "context", "kind", "q"} {
				if v, ok := a[k].(string); ok && v != "" {
					q.Set(k, v)
				}
			}
			if a["personal"] == true {
				q.Set("personal", "1")
			}
			p := "/api/items"
			if len(q) > 0 {
				p += "?" + q.Encode()
			}
			return "GET", p, nil
		}},
	{"add_item", "Add a personal to-do (no project) or a project ticket (with project). A new idea for a project goes in with inbox true unless the user says where.",
		args{"title": prop("string", ""), "project": projectProp, "kind": kindProp, "when": whenProp, "context": ctxProp, "status": statusProp,
			"parent_id": prop("integer", "parent to-do or feature id, to nest it"), "notes": prop("string", "markdown"), "inbox": prop("boolean", "file in the project's Inbox")},
		[]string{"title"},
		func(a args) (string, string, any) {
			return "POST", "/api/items", fields(a, "title", "project", "kind", "when", "context", "status", "parent_id", "notes", "inbox")
		}},
	{"edit_item", "Change fields of an item. parent_id null un-nests it.",
		args{"id": idProp, "title": prop("string", ""), "project": projectProp, "kind": kindProp, "when": whenProp, "context": ctxProp, "status": statusProp,
			"parent_id": prop("integer", ""), "notes": prop("string", "replaces the notes; use add_note to append"), "priority": prop("integer", "")},
		[]string{"id"},
		func(a args) (string, string, any) {
			return "PATCH", itemPath(a, ""), fields(a, "title", "project", "kind", "when", "context", "status", "parent_id", "notes", "priority")
		}},
	{"complete_item", "Mark an item done.", args{"id": idProp}, []string{"id"},
		func(a args) (string, string, any) { return "POST", itemPath(a, "/done"), nil }},
	{"reopen_item", "Reopen a done item.", args{"id": idProp}, []string{"id"},
		func(a args) (string, string, any) { return "POST", itemPath(a, "/reopen"), nil }},
	{"delete_item", "Delete an item for good. Only when the user clearly asks to delete.", args{"id": idProp}, []string{"id"},
		func(a args) (string, string, any) { return "DELETE", itemPath(a, ""), nil }},
	{"add_note", "Append a timestamped note to an item: progress updates, links, findings.", args{"id": idProp, "text": prop("string", "")}, []string{"id", "text"},
		func(a args) (string, string, any) { return "POST", itemPath(a, "/note"), fields(a, "text") }},
	{"list_projects", "List projects with their ids and slugs.", args{}, nil,
		func(a args) (string, string, any) { return "GET", "/api/projects", nil }},
	{"add_project", "Create a project.", args{"name": prop("string", ""), "slug": prop("string", "short name"), "description": prop("string", "")}, []string{"name"},
		func(a args) (string, string, any) {
			return "POST", "/api/projects", fields(a, "name", "slug", "description")
		}},
	{"habits_today", "Show today's habits and progress.", args{}, nil,
		func(a args) (string, string, any) { return "GET", "/api/habits", nil }},
	{"log_habit", "Log a habit for today: delta adds (water in ml, protein in g), set replaces, toggle flips a check habit.",
		args{"habit": prop("string", "habit name, e.g. water, walks, protein, standing"), "delta": prop("number", ""), "set": prop("number", ""), "toggle": prop("boolean", "")},
		[]string{"habit"},
		func(a args) (string, string, any) {
			return "POST", "/api/habits/" + url.PathEscape(fmt.Sprint(a["habit"])) + "/log", fields(a, "delta", "set", "toggle")
		}},
	{"start_timer", "Start a countdown. task locks the widget onto that task until it ends.",
		args{"minutes": prop("number", "1 to 720"), "task": prop("string", "")}, []string{"minutes"},
		func(a args) (string, string, any) {
			b := fields(a, "minutes")
			if t, ok := a["task"]; ok {
				b["label"] = t
			}
			return "POST", "/api/timer/start", b
		}},
	{"stop_timer", "Stop the timer.", args{}, nil,
		func(a args) (string, string, any) { return "POST", "/api/timer/stop", nil }},
	{"osrs_status", "Joe's OSRS state, live from RuneLite and the hiscores: daily runs (bird houses, herbs, trees...) with each patch, bank stock against targets (status low/ok/full), skill targets and AFK fallbacks.", args{}, nil,
		func(a args) (string, string, any) { return "GET", "/api/osrs", nil }},
	{"search_notes", "Search Joe's Obsidian notes (read only). Returns matching note paths and lines.", args{"query": prop("string", "words to find")}, []string{"query"},
		func(a args) (string, string, any) {
			return "GET", "/api/vault/search?q=" + url.QueryEscape(fmt.Sprint(a["query"])), nil
		}},
	{"read_note", "Read one of Joe's Obsidian notes (read only).", args{"path": prop("string", "path from search_notes, e.g. Hobbies/OSRS/goals.md")}, []string{"path"},
		func(a args) (string, string, any) {
			return "GET", "/api/vault/note?path=" + url.QueryEscape(fmt.Sprint(a["path"])), nil
		}},
}

func toolSpecs() []args {
	out := make([]args, 0, len(tools))
	for _, t := range tools {
		params := args{"type": "object", "properties": t.props}
		if len(t.required) > 0 {
			params["required"] = t.required
		}
		out = append(out, args{"type": "function", "function": args{"name": t.name, "description": t.desc, "parameters": params}})
	}
	return out
}

// runTool runs one tool call. The returned Action always carries a result
// for the model, including when the call failed, so it can correct itself.
func runTool(name, rawArgs string, do Doer) (Action, error) {
	fail := func(err error) (Action, error) {
		return Action{Tool: name, Summary: err.Error(), result: "error: " + err.Error()}, err
	}
	var t *tool
	for i := range tools {
		if tools[i].name == name {
			t = &tools[i]
		}
	}
	if t == nil {
		return fail(fmt.Errorf("unknown tool %q", name))
	}
	a := args{}
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &a); err != nil {
			return fail(fmt.Errorf("arguments are not JSON: %v", err))
		}
	}
	method, path, body := t.call(a)
	code, resp := do(method, path, body)
	if code >= 400 {
		var e struct{ Error string }
		json.Unmarshal(resp, &e)
		return fail(fmt.Errorf("%s", firstNonEmpty(e.Error, string(resp))))
	}
	return Action{Tool: name, OK: true, Summary: summarize(name, a, resp), result: trim(resp)}, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// summarize turns a result into one line for the chat, e.g. "#45 Buy milk".
func summarize(name string, a args, resp []byte) string {
	var one struct {
		ID     int64    `json:"id"`
		Title  string   `json:"title"`
		Name   string   `json:"name"`
		Unit   string   `json:"unit"`
		Value  *float64 `json:"value"`
		Target float64  `json:"target"`
	}
	var many []json.RawMessage
	switch {
	case json.Unmarshal(resp, &many) == nil:
		return fmt.Sprintf("%d found", len(many))
	case name == "log_habit" && json.Unmarshal(resp, &one) == nil && one.Value != nil:
		return strings.TrimSpace(fmt.Sprintf("%s %g/%g %s", one.Name, *one.Value, one.Target, one.Unit))
	case json.Unmarshal(resp, &one) == nil && one.ID != 0:
		return strings.TrimSpace(fmt.Sprintf("#%d %s", one.ID, firstNonEmpty(one.Title, one.Name)))
	}
	if name == "delete_item" {
		return "deleted #" + num(a["id"])
	}
	return "done"
}

// trim keeps lists small for the model: just the fields it needs to refer
// to and reason about items.
func trim(resp []byte) string {
	var list []map[string]any
	if json.Unmarshal(resp, &list) != nil {
		return string(resp)
	}
	keep := []string{"id", "title", "kind", "status", "when", "context", "project_id", "parent_id", "slug", "name", "value", "target", "unit"}
	for i, m := range list {
		small := map[string]any{}
		for _, k := range keep {
			if v, ok := m[k]; ok && v != nil && v != "" {
				small[k] = v
			}
		}
		list[i] = small
	}
	b, _ := json.Marshal(list)
	return string(b)
}
