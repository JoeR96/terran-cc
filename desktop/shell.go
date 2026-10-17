package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// winState is one open window, saved to tccd under ui/windows so every
// window reopens where it was.
type winState struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"` // dashboard | widget | side
	BoardID   string     `json:"board_id,omitempty"`
	ProjectID int64      `json:"project_id,omitempty"` // dashboard opened on a project page
	Widget    *widgetRef `json:"widget,omitempty"`
	Monitor   string     `json:"monitor"`
	Bounds    rect       `json:"bounds"`
	OnTop     *bool      `json:"on_top,omitempty"`
}

type widgetRef struct {
	Type      string `json:"type"`
	ProjectID int64  `json:"project_id,omitempty"`
}

type entry struct {
	st winState
	w  *application.WebviewWindow
}

type shell struct {
	app         *application.App
	base, token string

	mu       sync.Mutex
	wins     map[string]*entry
	order    []string
	quitting bool
	noSave   bool // the saved list couldn't be read; don't overwrite it
	save     *time.Timer
	seq      int

	sideOpening bool
}

func newShell(base, token string) *shell {
	return &shell{base: base, token: token, wins: map[string]*entry{}}
}

// url is the frontend route for st. top tells the frameless windows whether
// they start always-on-top, so their pin button starts in the right state.
func (s *shell) url(st winState, top bool) string {
	t := "&top=0"
	if top {
		t = "&top=1"
	}
	switch st.Kind {
	case "side":
		return "/#/side?win=" + st.ID + t
	case "widget":
		u := "/#/widget/" + st.Widget.Type + "?win=" + st.ID + t
		if st.Widget.ProjectID != 0 {
			u += "&project=" + strconv.FormatInt(st.Widget.ProjectID, 10)
		}
		return u
	}
	if st.ProjectID != 0 {
		return fmt.Sprintf("/#/project/%d?win=%s&board=%s", st.ProjectID, st.ID, st.BoardID)
	}
	return "/#/board/" + st.BoardID + "?win=" + st.ID
}

func (s *shell) screens() []screen {
	var out []screen
	for _, sc := range s.app.Screen.GetAll() {
		w := sc.WorkArea
		out = append(out, screen{ID: sc.ID, Work: rect{w.X, w.Y, w.Width, w.Height}, Primary: sc.IsPrimary})
	}
	return out
}

// defaultBounds puts a new window somewhere sensible on the primary screen.
func (s *shell) defaultBounds(kind string) rect {
	p := s.app.Screen.GetPrimary()
	if p == nil {
		return rect{100, 100, 1200, 800}
	}
	w := p.WorkArea
	switch kind {
	case "side":
		return rect{w.X + w.Width - 270, w.Y, 270, w.Height}
	case "widget":
		return rect{w.X + w.Width - 620, w.Y + 80, 320, 380}
	}
	return rect{w.X + 60, w.Y + 40, min(1400, w.Width-120), min(900, w.Height-80)}
}

// open creates a window for st, or focuses the side widget if one is open.
//
// s.mu is never held across a Wails window call: those wait for the main
// thread, and the main thread (tray clicks) takes s.mu, so holding it would
// deadlock.
func (s *shell) open(st winState) {
	s.mu.Lock()
	if st.Kind == "side" {
		if s.sideOpening {
			s.mu.Unlock()
			return
		}
		for _, e := range s.wins {
			if e.st.Kind == "side" {
				s.mu.Unlock()
				e.w.Show().Focus()
				return
			}
		}
		s.sideOpening = true
		defer func() {
			s.mu.Lock()
			s.sideOpening = false
			s.mu.Unlock()
		}()
	}
	if st.ID == "" {
		s.seq++
		st.ID = fmt.Sprintf("%s-%d-%d", st.Kind, time.Now().Unix()%100000, s.seq)
	}
	s.mu.Unlock()

	if st.Kind == "dashboard" && st.BoardID == "" {
		st.BoardID = "main"
	}
	if st.Kind == "widget" && st.Widget == nil {
		st.Widget = &widgetRef{Type: "today"}
	}
	if st.Bounds.W == 0 {
		st.Bounds = s.defaultBounds(st.Kind)
		st.Monitor = ""
		if p := s.app.Screen.GetPrimary(); p != nil {
			st.Monitor = p.ID
		}
	}
	st.Bounds, st.Monitor = placeOnScreen(st.Bounds, st.Monitor, s.screens())
	onTop := st.Kind != "dashboard"
	if st.OnTop != nil {
		onTop = *st.OnTop
	}

	opts := application.WebviewWindowOptions{
		Name:               st.ID,
		Title:              "Terran CC",
		URL:                s.url(st, onTop),
		Width:              st.Bounds.W,
		Height:             st.Bounds.H,
		AlwaysOnTop:        onTop,
		Frameless:          st.Kind != "dashboard",
		BackgroundColour:   application.NewRGB(0x14, 0x16, 0x1b),
		InitialPosition:    application.WindowXY,
		MinWidth:           200,
		MinHeight:          120,
		DevToolsEnabled:    true,
		ZoomControlEnabled: true, // Ctrl+wheel to scale for high-DPI monitors
	}
	if sc := s.app.Screen.GetByID(st.Monitor); sc != nil {
		opts.Screen = sc
		opts.X = st.Bounds.X - sc.WorkArea.X
		opts.Y = st.Bounds.Y - sc.WorkArea.Y
	} else {
		opts.X, opts.Y = st.Bounds.X, st.Bounds.Y
	}
	if st.Kind == "side" {
		opts.Title = "Terran CC · side"
	}
	w := s.app.Window.NewWithOptions(opts)
	s.mu.Lock()
	s.wins[st.ID] = &entry{st: st, w: w}
	s.order = append(s.order, st.ID)
	s.mu.Unlock()

	track := func(*application.WindowEvent) { s.track(st.ID) }
	w.OnWindowEvent(events.Common.WindowDidMove, track)
	w.OnWindowEvent(events.Common.WindowDidResize, track)
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.quitting {
			return // keep it in the saved list so it reopens next launch
		}
		delete(s.wins, st.ID)
		s.scheduleSave()
	})
	s.mu.Lock()
	s.scheduleSave()
	s.mu.Unlock()
}

// track records a window's new bounds and monitor.
func (s *shell) track(id string) {
	s.mu.Lock()
	e := s.wins[id]
	s.mu.Unlock()
	if e == nil || e.w.IsMinimised() || e.w.IsMaximised() {
		return
	}
	b := e.w.Bounds()
	mon := ""
	if sc, err := e.w.GetScreen(); err == nil && sc != nil {
		mon = sc.ID
	}
	s.mu.Lock()
	if b.Width > 0 {
		e.st.Bounds = rect{b.X, b.Y, b.Width, b.Height}
	}
	if mon != "" {
		e.st.Monitor = mon
	}
	s.scheduleSave()
	s.mu.Unlock()
}

// scheduleSave writes the window list half a second after the last change.
// Callers hold s.mu.
func (s *shell) scheduleSave() {
	if s.save != nil {
		s.save.Stop()
	}
	s.save = time.AfterFunc(500*time.Millisecond, s.saveNow)
}

func (s *shell) saveNow() {
	s.mu.Lock()
	list := []winState{}
	for _, id := range s.order {
		if e, ok := s.wins[id]; ok {
			list = append(list, e.st)
		}
	}
	skip := s.noSave
	s.mu.Unlock()
	if skip {
		return
	}
	if err := s.putUI("windows", list); err != nil {
		log.Printf("saving windows: %v", err)
	}
}

func (s *shell) apiReq(method, path string, body any) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, s.base+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	return (&http.Client{Timeout: 5 * time.Second}).Do(req)
}

func (s *shell) putUI(key string, v any) error {
	resp, err := s.apiReq("PUT", "/api/ui/"+key, v)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("tccd answered %s", resp.Status)
	}
	return nil
}

// savedWindows reads the saved window list. An empty list (nothing saved)
// is not an error; a failed or garbled read is.
func (s *shell) savedWindows() ([]winState, error) {
	resp, err := s.apiReq("GET", "/api/ui/windows", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tccd answered %s", resp.Status)
	}
	var list []winState
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("saved windows are not valid: %w", err)
	}
	return list, nil
}

// restore reopens the saved windows, or a dashboard and the side widget on
// first run. If the list can't be read it opens the defaults but won't save
// over the list this session.
func (s *shell) restore() {
	list, err := s.savedWindows()
	for i := 0; err != nil && i < 3; i++ {
		time.Sleep(500 * time.Millisecond)
		list, err = s.savedWindows()
	}
	if err != nil {
		log.Printf("reading saved windows: %v; not saving window changes this session", err)
		s.mu.Lock()
		s.noSave = true
		s.mu.Unlock()
	}
	if len(list) == 0 {
		s.open(winState{Kind: "dashboard"})
		s.open(winState{Kind: "side"})
		return
	}
	for _, st := range list {
		s.open(st)
	}
}

// focusDashboard shows the first dashboard window, opening one if needed.
func (s *shell) focusDashboard() {
	s.mu.Lock()
	var w *application.WebviewWindow
	for _, id := range s.order {
		if e, ok := s.wins[id]; ok && e.st.Kind == "dashboard" {
			w = e.w
			break
		}
	}
	s.mu.Unlock()
	if w == nil {
		s.open(winState{Kind: "dashboard"})
		return
	}
	w.Show()
	if w.IsMinimised() {
		w.UnMinimise()
	}
	w.Focus()
}

func (s *shell) flashAll() {
	s.mu.Lock()
	var ws []*application.WebviewWindow
	for _, e := range s.wins {
		ws = append(ws, e.w)
	}
	s.mu.Unlock()
	for _, w := range ws {
		w.Flash(true)
	}
}

func (s *shell) quit() {
	s.mu.Lock()
	s.quitting = true
	if s.save != nil {
		s.save.Stop()
	}
	s.mu.Unlock()
	s.saveNow()
	s.app.Quit()
}

// middleware serves /shell/* to the frontend: the tccd connection and
// window actions. Everything else goes to the bundled assets.
func (s *shell) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/shell/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		action := strings.TrimPrefix(r.URL.Path, "/shell/")
		if action != "conn" {
			log.Printf("shell %s win=%s spec=%s", action, r.URL.Query().Get("win"), r.URL.Query().Get("spec"))
		}
		if action == "conn" {
			json.NewEncoder(w).Encode(map[string]string{"base": s.base, "token": s.token})
			return
		}
		id := r.URL.Query().Get("win")
		var spec struct {
			winState
			On *bool `json:"on"`
		}
		if q := r.URL.Query().Get("spec"); q != "" {
			if err := json.Unmarshal([]byte(q), &spec); err != nil {
				http.Error(w, `{"error":"bad spec"}`, 400)
				return
			}
		}
		s.mu.Lock()
		e := s.wins[id]
		s.mu.Unlock()
		switch action {
		case "open":
			st := spec.winState
			st.ID, st.Bounds, st.Monitor, st.OnTop = "", rect{}, "", nil
			go s.open(st)
		case "close":
			if e != nil {
				go e.w.Close()
			}
		case "minimise":
			if e != nil {
				go e.w.Minimise()
			}
		case "top":
			if e != nil && spec.On != nil {
				on := *spec.On
				go e.w.SetAlwaysOnTop(on)
				s.mu.Lock()
				e.st.OnTop = &on
				s.scheduleSave()
				s.mu.Unlock()
			}
		case "state":
			if e != nil {
				s.mu.Lock()
				// The frontend reports every route change: a board, or a
				// project page (project_id set) opened from that board.
				if spec.BoardID != "" {
					e.st.BoardID = spec.BoardID
				}
				e.st.ProjectID = spec.ProjectID
				s.scheduleSave()
				s.mu.Unlock()
			}
		default:
			http.Error(w, `{"error":"unknown shell action"}`, 404)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})
}
