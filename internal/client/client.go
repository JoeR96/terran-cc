// Package client is the typed HTTP client for tccd, used by the CLI and any
// other Go client. EnsureDaemon starts tccd when it isn't running.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"terrancc/internal/config"
	"terrancc/internal/core"
	"terrancc/internal/model"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(addr, token string) *Client {
	return &Client{base: "http://" + addr, token: token, http: &http.Client{Timeout: 10 * time.Second}}
}

// APIError is a non-2xx response from tccd.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

func (c *Client) send(req *http.Request, out any) error {
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return &APIError{Status: resp.StatusCode, Message: e.Error}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) do(method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, out)
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func (c *Client) Health() (map[string]any, error) {
	var m map[string]any
	return m, c.do("GET", "/api/health", nil, &m)
}

// --- projects ---

func (c *Client) ListProjects(archived bool) ([]model.Project, error) {
	var ps []model.Project
	path := "/api/projects"
	if archived {
		path += "?archived=1"
	}
	return ps, c.do("GET", path, nil, &ps)
}

func (c *Client) GetProject(ref string) (model.Project, error) {
	var p model.Project
	return p, c.do("GET", "/api/projects/"+url.PathEscape(ref), nil, &p)
}

func (c *Client) AddProject(body map[string]any) (model.Project, error) {
	var p model.Project
	return p, c.do("POST", "/api/projects", body, &p)
}

func (c *Client) EditProject(ref string, patch map[string]any) (model.Project, error) {
	var p model.Project
	return p, c.do("PATCH", "/api/projects/"+url.PathEscape(ref), patch, &p)
}

// --- items ---

func (c *Client) ListItems(q url.Values) ([]model.Item, error) {
	var items []model.Item
	return items, c.do("GET", "/api/items?"+q.Encode(), nil, &items)
}

func (c *Client) GetItem(itemID int64) (model.Item, error) {
	var it model.Item
	return it, c.do("GET", "/api/items/"+id(itemID), nil, &it)
}

func (c *Client) AddItem(n core.NewItem) (model.Item, error) {
	var it model.Item
	return it, c.do("POST", "/api/items", n, &it)
}

func (c *Client) EditItem(itemID int64, patch map[string]any) (model.Item, error) {
	var it model.Item
	return it, c.do("PATCH", "/api/items/"+id(itemID), patch, &it)
}

// Action runs start, review, done or reopen on an item.
func (c *Client) Action(itemID int64, action string, body any) (model.Item, error) {
	var it model.Item
	return it, c.do("POST", "/api/items/"+id(itemID)+"/"+action, body, &it)
}

func (c *Client) Note(itemID int64, text string) (model.Item, error) {
	var it model.Item
	return it, c.do("POST", "/api/items/"+id(itemID)+"/note", map[string]string{"text": text}, &it)
}

func (c *Client) Evidence(itemID int64, ev []model.Evidence) (model.Item, error) {
	var it model.Item
	return it, c.do("POST", "/api/items/"+id(itemID)+"/evidence", map[string]any{"evidence": ev}, &it)
}

func (c *Client) Attach(itemID int64, path string) (model.Item, error) {
	var it model.Item
	f, err := os.Open(path)
	if err != nil {
		return it, err
	}
	defer f.Close()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return it, err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return it, err
	}
	mw.Close()
	req, err := http.NewRequest("POST", c.base+"/api/items/"+id(itemID)+"/attachments", &buf)
	if err != nil {
		return it, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return it, c.send(req, &it)
}

func (c *Client) DeleteItem(itemID int64) error {
	return c.do("DELETE", "/api/items/"+id(itemID), nil, nil)
}

// --- habits ---

type HabitsDay struct {
	Date   string           `json:"date"`
	Streak int              `json:"streak"`
	Habits []model.HabitDay `json:"habits"`
}

func (c *Client) Habits(date string) (HabitsDay, error) {
	var d HabitsDay
	path := "/api/habits"
	if date != "" {
		path += "?date=" + url.QueryEscape(date)
	}
	return d, c.do("GET", path, nil, &d)
}

func (c *Client) LogHabit(ref string, op core.LogOp) (model.HabitDay, error) {
	var d model.HabitDay
	return d, c.do("POST", "/api/habits/"+url.PathEscape(ref)+"/log", op, &d)
}

func (c *Client) AddHabit(h model.Habit) (model.Habit, error) {
	var out model.Habit
	return out, c.do("POST", "/api/habits", h, &out)
}

func (c *Client) EditHabit(ref string, patch map[string]any) (model.Habit, error) {
	var out model.Habit
	return out, c.do("PATCH", "/api/habits/"+url.PathEscape(ref), patch, &out)
}

// --- timer (snapshots stay as JSON maps; clients only display them) ---

func (c *Client) Timer() (map[string]any, error) {
	var m map[string]any
	return m, c.do("GET", "/api/timer", nil, &m)
}

func (c *Client) TimerAction(action string, body any) (map[string]any, error) {
	var m map[string]any
	return m, c.do("POST", "/api/timer/"+action, body, &m)
}

func (c *Client) TimerConfig(cfg map[string]any) (map[string]any, error) {
	var m map[string]any
	return m, c.do("PUT", "/api/timer/config", cfg, &m)
}

// --- daemon ---

func healthy(home string) (*Client, bool) {
	d, err := config.ReadDaemon(home)
	if err != nil || d.Addr == "" {
		return nil, false
	}
	c := New(d.Addr, d.Token)
	c.http.Timeout = time.Second
	if _, err := c.Health(); err != nil {
		return nil, false
	}
	c.http.Timeout = 10 * time.Second
	return c, true
}

// EnsureDaemon returns a client for the running tccd, starting tccdPath
// (detached) if needed and waiting up to timeout for it to answer.
func EnsureDaemon(home, tccdPath string, timeout time.Duration) (*Client, error) {
	if c, ok := healthy(home); ok {
		return c, nil
	}
	cmd := exec.Command(tccdPath, "--home", home)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start the terran-cc service (%s): %w", tccdPath, err)
	}
	go cmd.Wait() // reap it if it exits early
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c, ok := healthy(home); ok {
			return c, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("terran-cc service did not start (see " + filepath.Join(home, "tccd.log") + ")")
}
