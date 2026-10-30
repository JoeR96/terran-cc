package api

import (
	"errors"
	"fmt"
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

// editablePath is vaultPath for routes that change the vault: it also keeps
// out of hidden folders like .obsidian and .trash, and needs the parent
// folder to exist.
func (s *server) editablePath(rel string) (string, error) {
	p := s.vaultPath(rel)
	if p == "" {
		return "", badRequest("path must be inside the vault")
	}
	for _, bit := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(bit, ".") {
			return "", badRequest("path can't be in a hidden folder")
		}
	}
	if fi, err := os.Stat(filepath.Dir(p)); err != nil || !fi.IsDir() {
		return "", badRequest("folder does not exist")
	}
	return p, nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// trashPath is a free name for p in the vault's .trash folder, where
// Obsidian puts deleted files too.
func (s *server) trashPath(p string) (string, error) {
	trash := filepath.Join(s.vault, ".trash")
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return "", err
	}
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		ext = ""
	}
	stem := strings.TrimSuffix(base, ext)
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s %d%s", stem, i, ext)
		}
		if t := filepath.Join(trash, name); !exists(t) {
			return t, nil
		}
	}
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
	mux.HandleFunc("POST /api/vault/note", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		rel := r.URL.Query().Get("path")
		p, err := s.editablePath(rel)
		if err == nil && !strings.EqualFold(filepath.Ext(p), ".md") {
			err = badRequest("path must be a .md file")
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			writeJSON(w, 409, map[string]string{"error": "a file with that name already exists"})
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		f.Close()
		writeJSON(w, 200, map[string]string{"path": rel})
	})
	mux.HandleFunc("POST /api/vault/dir", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		rel := r.URL.Query().Get("path")
		p, err := s.editablePath(rel)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := os.Mkdir(p, 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				writeJSON(w, 409, map[string]string{"error": "that already exists"})
				return
			}
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"path": rel})
	})
	mux.HandleFunc("POST /api/vault/move", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := decode(r, &body); err != nil {
			writeErr(w, err)
			return
		}
		from, err := s.editablePath(body.From)
		if err != nil {
			writeErr(w, err)
			return
		}
		to, err := s.editablePath(body.To)
		if err != nil {
			writeErr(w, err)
			return
		}
		fi, err := os.Stat(from)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "no such file"})
			return
		}
		if !fi.IsDir() && !strings.EqualFold(filepath.Ext(from), filepath.Ext(to)) {
			writeErr(w, badRequest("a rename can't change the file type"))
			return
		}
		if in, err := filepath.Rel(from, to); err == nil && in != "." && !strings.HasPrefix(in, "..") {
			writeErr(w, badRequest("can't move a folder into itself"))
			return
		}
		// A case-only rename finds itself on Windows, so it isn't a clash.
		if exists(to) && !strings.EqualFold(from, to) {
			writeJSON(w, 409, map[string]string{"error": "a file with that name already exists"})
			return
		}
		if err := os.Rename(from, to); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"path": body.To})
	})
	mux.HandleFunc("DELETE /api/vault/entry", func(w http.ResponseWriter, r *http.Request) {
		if !need(w) {
			return
		}
		p, err := s.editablePath(r.URL.Query().Get("path"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if !exists(p) {
			writeJSON(w, 404, map[string]string{"error": "no such file"})
			return
		}
		t, err := s.trashPath(p)
		if err == nil {
			err = os.Rename(p, t)
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"trash": filepath.ToSlash(strings.TrimPrefix(t, s.vault+string(filepath.Separator)))})
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
