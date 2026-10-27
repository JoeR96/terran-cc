package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WithVault lets the side widget search, browse and edit Joe's Obsidian
// vault and show its images. dir is the vault folder.
func WithVault(dir string) Option { return func(s *server) { s.vault = dir } }

const (
	maxNoteBytes     = 200 << 10
	maxFullNoteBytes = 8 << 20
	maxHits          = 20
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
	mux.HandleFunc("GET /api/vault/tree", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		type entry struct {
			Path string `json:"path"`
			Name string `json:"name"`
			Dir  bool   `json:"dir"`
		}
		var entries []entry
		filepath.WalkDir(s.vault, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == s.vault {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(s.vault, p)
			if err != nil {
				return nil
			}
			entries = append(entries, entry{Path: filepath.ToSlash(rel), Name: name, Dir: d.IsDir()})
			return nil
		})
		if entries == nil {
			entries = []entry{}
		}
		writeJSON(w, 200, entries)
	})
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
		full := r.URL.Query().Get("full") == "1"
		limit := maxNoteBytes
		if full {
			limit = maxFullNoteBytes
		}
		if len(b) > limit {
			if full {
				writeErr(w, badRequest("note is too large to edit here"))
				return
			}
			b = b[:limit]
		}
		writeJSON(w, 200, map[string]string{"path": r.URL.Query().Get("path"), "text": string(b)})
	})
	mux.HandleFunc("PUT /api/vault/note", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		rel := r.URL.Query().Get("path")
		p := s.vaultPath(rel)
		if p == "" || !strings.EqualFold(filepath.Ext(p), ".md") {
			writeErr(w, badRequest("path must be a .md file inside the vault"))
			return
		}
		parent := filepath.Dir(p)
		if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
			writeErr(w, badRequest("folder does not exist"))
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		if len(body.Text) > maxFullNoteBytes {
			writeErr(w, badRequest("note is too large"))
			return
		}
		if err := os.WriteFile(p, []byte(body.Text), 0o644); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"path": rel})
	})
	mux.HandleFunc("GET /api/vault/file", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		p := s.vaultPath(r.URL.Query().Get("path"))
		ct, ok := vaultImageType(p)
		if p == "" || !ok {
			writeErr(w, badRequest("path must be an image inside the vault"))
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "no such file"})
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(200)
		w.Write(b)
	})
}

func vaultImageType(p string) (string, bool) {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".svg":
		return "image/svg+xml", true
	case ".bmp":
		return "image/bmp", true
	case ".avif":
		return "image/avif", true
	default:
		return "", false
	}
}
