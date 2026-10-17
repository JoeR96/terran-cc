package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WithVault lets the assistant search and read Joe's Obsidian vault, read
// only. dir is the vault folder.
func WithVault(dir string) Option { return func(s *server) { s.vault = dir } }

const (
	maxNoteBytes = 200 << 10
	maxHits      = 20
)

// vaultPath turns a vault-relative path into a file inside the vault, or
// "" when it would leave it.
func (s *server) vaultPath(rel string) string {
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return ""
	}
	p := filepath.Join(s.vault, filepath.FromSlash(rel))
	if r, err := filepath.Rel(s.vault, p); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return ""
	}
	return p
}

func (s *server) vaultRoutes(mux *http.ServeMux) {
	need := func(w http.ResponseWriter) bool {
		if s.vault == "" {
			writeJSON(w, 503, map[string]string{"error": "no notes vault is set up"})
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/vault/search", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		if q == "" {
			writeErr(w, badRequest("q is required"))
			return
		}
		type hit struct {
			Path  string   `json:"path"`
			Lines []string `json:"lines"`
		}
		hits := []hit{}
		filepath.WalkDir(s.vault, func(p string, d fs.DirEntry, err error) error {
			if err != nil || len(hits) >= maxHits {
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != s.vault {
					return filepath.SkipDir // .obsidian, .trash
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(p), ".md") {
				return nil
			}
			rel, _ := filepath.Rel(s.vault, p)
			rel = filepath.ToSlash(rel)
			b, err := os.ReadFile(p)
			if err != nil || len(b) > maxNoteBytes {
				return nil
			}
			var lines []string
			for _, l := range strings.Split(string(b), "\n") {
				if strings.Contains(strings.ToLower(l), q) && len(lines) < 5 {
					lines = append(lines, strings.TrimSpace(l))
				}
			}
			if len(lines) > 0 || strings.Contains(strings.ToLower(rel), q) {
				hits = append(hits, hit{rel, lines})
			}
			return nil
		})
		writeJSON(w, 200, hits)
	})
	mux.HandleFunc("GET /api/vault/note", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		p := s.vaultPath(r.URL.Query().Get("path"))
		if p == "" || !strings.EqualFold(filepath.Ext(p), ".md") {
			writeErr(w, badRequest("path must be a .md file inside the vault"))
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "no such note"})
			return
		}
		if len(b) > maxNoteBytes {
			b = b[:maxNoteBytes]
		}
		writeJSON(w, 200, map[string]string{"path": r.URL.Query().Get("path"), "text": string(b)})
	})
}
