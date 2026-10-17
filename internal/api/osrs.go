package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"terrancc/internal/events"
	"terrancc/internal/osrs"
)

// OSRSConfigKey is the settings key the OSRS config is persisted under.
const OSRSConfigKey = "osrs.config"

// WikiImages is where OSRS Wiki icons are fetched from.
const WikiImages = "https://oldschool.runescape.wiki/images/"

// WithOSRS enables /api/osrs, backed by svc. wiki is the image base URL
// ("" for the real wiki).
func WithOSRS(svc *osrs.Service, wiki string) Option {
	return func(s *server) {
		s.osrs = svc
		s.wiki = wiki
		if s.wiki == "" {
			s.wiki = WikiImages
		}
		// Saves go to settings and changes reach every client as events.
		if svc.Save == nil {
			svc.Save = SaveOSRS(func(k, v string) error { return s.svc.Store.SetSetting(context.Background(), k, v) })
		}
		if svc.OnChange == nil {
			svc.OnChange = func() { s.bus.Publish(events.Event{Type: "osrs.changed"}) }
		}
		if svc.OnReady == nil {
			svc.OnReady = func(r osrs.Run) { s.bus.Publish(events.Event{Type: "osrs.ready", Data: r}) }
		}
	}
}

// iconName allows wiki file names like "Prayer_potion(4).png" and nothing
// that could leave the cache folder.
var iconName = regexp.MustCompile(`^[A-Za-z0-9_()'+,.-]+\.png$`)

func (s *server) osrsRoutes(mux *http.ServeMux) {
	need := func(w http.ResponseWriter) bool {
		if s.osrs == nil {
			writeJSON(w, 503, map[string]string{"error": "OSRS tracking is not set up in this tccd"})
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/osrs", func(w http.ResponseWriter, r *http.Request) {
		if need(w) {
			writeJSON(w, 200, s.osrs.Status())
		}
	})
	mux.HandleFunc("GET /api/osrs/config", func(w http.ResponseWriter, r *http.Request) {
		if need(w) {
			writeJSON(w, 200, s.osrs.Config())
		}
	})
	mux.HandleFunc("PUT /api/osrs/config", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		var c osrs.Config
		if err := decode(r, &c); err != nil {
			writeErr(w, err)
			return
		}
		for _, it := range c.Stock {
			if it.Key == "" || it.Sufficient < 0 || it.Target < 0 {
				writeErr(w, badRequest("each stock item needs a key and non-negative numbers"))
				return
			}
		}
		if err := s.osrs.SetConfig(c); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, s.osrs.Config())
	})
	mux.HandleFunc("GET /api/osrs/icon/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !iconName.MatchString(name) {
			writeErr(w, badRequest("not an icon name: %q", name))
			return
		}
		path := filepath.Join(s.home, "icons", name)
		if _, err := os.Stat(path); err != nil {
			if err := s.fetchIcon(name, path); err != nil {
				writeJSON(w, 404, map[string]string{"error": err.Error()})
				return
			}
		}
		w.Header().Set("Cache-Control", "max-age=604800")
		http.ServeFile(w, r, path)
	})
}

// fetchIcon downloads one wiki image into the icon cache.
func (s *server) fetchIcon(name, path string) error {
	req, _ := http.NewRequest("GET", s.wiki+name, nil)
	req.Header.Set("User-Agent", "terran-cc/"+Version)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("wiki has no %s (%d)", name, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SaveOSRS returns a Save func that stores the config in settings.
func SaveOSRS(set func(key, value string) error) func(osrs.Config) error {
	return func(c osrs.Config) error {
		b, err := json.Marshal(c)
		if err != nil {
			return err
		}
		return set(OSRSConfigKey, string(b))
	}
}
