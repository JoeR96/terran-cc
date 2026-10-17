// Package assist is the model in the loop behind the chat box: it sends the
// conversation to OpenCode Go (an OpenAI-compatible chat API), runs the tool
// calls the model makes against tccd's own API, and returns its reply.
package assist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultURL is OpenCode Go's OpenAI-compatible chat endpoint.
const DefaultURL = "https://opencode.ai/inference/go/openai/v1/chat/completions"

// DefaultModel answers quickly and calls tools reliably (about 7s for a
// four-tool message, against 17s for kimi-k3).
const DefaultModel = "glm-5.3-flash"

// MaxRounds caps model calls per message, so a model stuck calling tools
// can't run forever.
const MaxRounds = 8

var ErrNoKey = errors.New("no OpenCode Go key")

// Doer performs one call against tccd's API and returns the status and body.
type Doer func(method, path string, body any) (int, []byte)

type Message struct {
	Role    string `json:"role"` // user | assistant
	Content string `json:"content"`
}

// Action is one tool call the model made, for the chat to show.
type Action struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	OK      bool   `json:"ok"`
	result  string // what the model sees
}

type Reply struct {
	Reply   string   `json:"reply"`
	Actions []Action `json:"actions"`
}

type Client struct {
	URL     string
	Model   string
	Key     func() (string, error)
	Version string // sent in the User-Agent, as OpenCode Go asks
	HTTP    *http.Client
}

// OpencodeKey reads the OpenCode Go key that `opencode auth login` saved.
func OpencodeKey(path string) func() (string, error) {
	return func() (string, error) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		var auth map[string]struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(b, &auth); err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		if k := auth["opencode-go"].Key; k != "" {
			return k, nil
		}
		return "", fmt.Errorf("%s has no opencode-go key; connect OpenCode Go in opencode first", path)
	}
}

// AuthPath is where opencode keeps its keys.
func AuthPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

type wireMsg struct {
	Role       string          `json:"role"`
	Content    *string         `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func str(s string) *string { return &s }

// Run answers the last user message in history. session identifies the
// conversation to OpenCode Go for routing and prompt caching.
func (c *Client) Run(ctx context.Context, session, system string, history []Message, do Doer) (Reply, error) {
	key, err := c.Key()
	if err != nil {
		return Reply{}, fmt.Errorf("%w: %v", ErrNoKey, err)
	}
	var msgs []wireMsg
	if system != "" {
		msgs = append(msgs, wireMsg{Role: "system", Content: str(system)})
	}
	for _, m := range history {
		msgs = append(msgs, wireMsg{Role: m.Role, Content: str(m.Content)})
	}
	var out Reply
	for range MaxRounds {
		msg, err := c.complete(ctx, key, session, msgs)
		if err != nil {
			return out, err
		}
		var calls []toolCall
		json.Unmarshal(msg.ToolCalls, &calls)
		if len(calls) == 0 {
			if msg.Content != nil {
				out.Reply = strings.TrimSpace(*msg.Content)
			}
			return out, nil
		}
		msgs = append(msgs, wireMsg{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})
		for _, tc := range calls {
			a, _ := runTool(tc.Function.Name, tc.Function.Arguments, do)
			out.Actions = append(out.Actions, a)
			msgs = append(msgs, wireMsg{Role: "tool", ToolCallID: tc.ID, Content: str(a.result)})
		}
	}
	return out, fmt.Errorf("the model was still calling tools after %d rounds; stopped", MaxRounds)
}

func (c *Client) complete(ctx context.Context, key, session string, msgs []wireMsg) (wireMsg, error) {
	body, _ := json.Marshal(map[string]any{"model": c.Model, "messages": msgs, "tools": toolSpecs(), "max_tokens": 1500})
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return wireMsg{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "terran-cc/"+c.Version)
	req.Header.Set("x-opencode-session", session)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return wireMsg{}, fmt.Errorf("OpenCode Go: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return wireMsg{}, fmt.Errorf("OpenCode Go %d: %s", resp.StatusCode, strings.TrimSpace(string(b[:min(len(b), 300)])))
	}
	var r struct {
		Choices []struct {
			Message wireMsg `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &r); err != nil || len(r.Choices) == 0 {
		return wireMsg{}, fmt.Errorf("OpenCode Go sent an answer I can't read: %.200s", b)
	}
	return r.Choices[0].Message, nil
}
