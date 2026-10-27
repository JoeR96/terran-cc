package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"

	"terrancc/internal/term"
)

// WithTerminal serves the side widget's Claude session over /api/term.
func WithTerminal(t *term.Session) Option { return func(s *server) { s.term = t } }

// termRoutes: GET /api/term is a websocket. The server sends the session's
// output as binary frames (scrollback first); the client sends JSON text
// frames: {"t":"i","d":"..."} to type, {"t":"r","c":cols,"r":rows} to resize.
func (s *server) termRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/term", func(w http.ResponseWriter, r *http.Request) {
		if s.term == nil {
			writeJSON(w, 503, map[string]string{"error": "no terminal in this tccd"})
			return
		}
		// The token check has already passed; the desktop's own origin is
		// a wails:// URL, so origins can't be matched.
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		c.SetReadLimit(1 << 20)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		defer c.CloseNow()

		out, stop := s.term.Subscribe()
		defer stop()
		if b := s.term.Scrollback(); len(b) > 0 {
			c.Write(ctx, websocket.MessageBinary, b)
		}
		if err := s.term.Ensure(); err != nil {
			c.Write(ctx, websocket.MessageBinary, []byte("\r\n\x1b[31m"+err.Error()+"\x1b[0m\r\n"))
		}
		go func() {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-out:
					if c.Write(ctx, websocket.MessageBinary, b) != nil {
						return
					}
				}
			}
		}()
		for {
			_, msg, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m struct {
				T    string `json:"t"`
				D    string `json:"d"`
				Cols int    `json:"c"`
				Rows int    `json:"r"`
			}
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			switch m.T {
			case "i":
				s.term.Input([]byte(m.D))
			case "r":
				s.term.Resize(m.Cols, m.Rows)
			}
		}
	})
}
