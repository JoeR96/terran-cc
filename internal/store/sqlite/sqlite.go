// Package sqlite implements store.Store on a single SQLite file.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"terrancc/internal/model"
	"terrancc/internal/store"
)

//go:embed migrations/*.sql
var migrations embed.FS

type querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// DB is a store.Store. Inside WithTx, db is nil and q is the transaction.
type DB struct {
	db *sql.DB
	q  querier
}

var _ store.Store = (*DB)(nil)

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &DB{db: db, q: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *DB) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DB) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for i, name := range names {
		version := i + 1
		if version <= current {
			continue
		}
		body, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *DB) WithTx(ctx context.Context, fn func(store.Store) error) error {
	if s.db == nil { // already inside a transaction
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&DB{q: tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// --- encoding helpers ---

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func tsPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func parseTSPtr(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	t := parseTS(ns.String)
	return &t
}

func jsonText(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func checkAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	return err
}

type scanner interface{ Scan(dest ...any) error }

// --- projects ---

const projectCols = `id, slug, name, description, repo_path, archived, created_at`

func scanProject(r scanner) (model.Project, error) {
	var p model.Project
	var created string
	err := r.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.RepoPath, &p.Archived, &created)
	p.CreatedAt = parseTS(created)
	return p, err
}

func (s *DB) CreateProject(ctx context.Context, p *model.Project) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	res, err := s.q.ExecContext(ctx,
		`INSERT INTO projects (slug, name, description, repo_path, archived, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.Slug, p.Name, p.Description, p.RepoPath, p.Archived, ts(p.CreatedAt))
	if isUnique(err) {
		return fmt.Errorf("%w: project slug %q already exists", store.ErrConflict, p.Slug)
	}
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (s *DB) GetProject(ctx context.Context, id int64) (model.Project, error) {
	p, err := scanProject(s.q.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
	return p, notFound(err)
}

func (s *DB) GetProjectBySlug(ctx context.Context, slug string) (model.Project, error) {
	p, err := scanProject(s.q.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE slug = ?`, slug))
	return p, notFound(err)
}

func (s *DB) ListProjects(ctx context.Context, includeArchived bool) ([]model.Project, error) {
	q := `SELECT ` + projectCols + ` FROM projects`
	if !includeArchived {
		q += ` WHERE archived = 0`
	}
	rows, err := s.q.QueryContext(ctx, q+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *DB) UpdateProject(ctx context.Context, p model.Project) error {
	res, err := s.q.ExecContext(ctx,
		`UPDATE projects SET slug = ?, name = ?, description = ?, repo_path = ?, archived = ? WHERE id = ?`,
		p.Slug, p.Name, p.Description, p.RepoPath, p.Archived, p.ID)
	if isUnique(err) {
		return fmt.Errorf("%w: project slug %q already exists", store.ErrConflict, p.Slug)
	}
	return checkAffected(res, err)
}

// --- items ---

const itemCols = `id, title, notes, kind, project_id, parent_id, status, "when", context, priority, position,
	why_first, depends_on, branch, worktree, session, session_model, session_started_at, model, effort,
	evidence, attachments, created_at, updated_at, done_at`

func scanItem(r scanner) (model.Item, error) {
	var it model.Item
	var pid, parent sql.NullInt64
	var deps, ev, att, created, updated string
	var started, done sql.NullString
	err := r.Scan(&it.ID, &it.Title, &it.Notes, &it.Kind, &pid, &parent, &it.Status, &it.When, &it.Context,
		&it.Priority, &it.Position, &it.WhyFirst, &deps, &it.Branch, &it.Worktree, &it.Session, &it.SessionModel,
		&started, &it.Model, &it.Effort, &ev, &att, &created, &updated, &done)
	if err != nil {
		return it, err
	}
	it.ProjectID, it.ParentID = idPtr(pid), idPtr(parent)
	it.DependsOn, it.Evidence, it.Attachments = []int64{}, []model.Evidence{}, []string{}
	json.Unmarshal([]byte(deps), &it.DependsOn)
	json.Unmarshal([]byte(ev), &it.Evidence)
	json.Unmarshal([]byte(att), &it.Attachments)
	it.SessionStartedAt, it.DoneAt = parseTSPtr(started), parseTSPtr(done)
	it.CreatedAt, it.UpdatedAt = parseTS(created), parseTS(updated)
	return it, nil
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func itemArgs(it *model.Item) []any {
	return []any{it.Title, it.Notes, it.Kind, it.ProjectID, it.ParentID, it.Status, it.When, it.Context,
		it.Priority, it.Position, it.WhyFirst, jsonText(orEmpty(it.DependsOn)), it.Branch, it.Worktree, it.Session,
		it.SessionModel, tsPtr(it.SessionStartedAt), it.Model, it.Effort, jsonText(orEmpty(it.Evidence)),
		jsonText(orEmpty(it.Attachments)), ts(it.CreatedAt), ts(it.UpdatedAt), tsPtr(it.DoneAt)}
}

func (s *DB) CreateItem(ctx context.Context, it *model.Item) error {
	if it.CreatedAt.IsZero() {
		it.CreatedAt = time.Now()
	}
	if it.UpdatedAt.IsZero() {
		it.UpdatedAt = it.CreatedAt
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO items (title, notes, kind, project_id, parent_id, status, "when",
		context, priority, position, why_first, depends_on, branch, worktree, session, session_model,
		session_started_at, model, effort, evidence, attachments, created_at, updated_at, done_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, itemArgs(it)...)
	if err != nil {
		return err
	}
	it.ID, err = res.LastInsertId()
	return err
}

func (s *DB) GetItem(ctx context.Context, id int64) (model.Item, error) {
	it, err := scanItem(s.q.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE id = ?`, id))
	return it, notFound(err)
}

func inClause[T ~string](col string, vals []T, where *[]string, args *[]any) {
	if len(vals) == 0 {
		return
	}
	marks := make([]string, len(vals))
	for i, v := range vals {
		marks[i] = "?"
		*args = append(*args, string(v))
	}
	*where = append(*where, col+" IN ("+strings.Join(marks, ",")+")")
}

func (s *DB) ListItems(ctx context.Context, f store.ItemFilter) ([]model.Item, error) {
	var where []string
	var args []any
	if f.ProjectID != nil {
		where, args = append(where, "project_id = ?"), append(args, *f.ProjectID)
	}
	if f.Personal {
		where = append(where, "project_id IS NULL")
	}
	if f.ParentID != nil {
		where, args = append(where, "parent_id = ?"), append(args, *f.ParentID)
	}
	if f.Context != nil {
		where, args = append(where, "context = ?"), append(args, string(*f.Context))
	}
	if f.Query != "" {
		where, args = append(where, "title LIKE ?"), append(args, "%"+f.Query+"%")
	}
	inClause("status", f.Status, &where, &args)
	inClause(`"when"`, f.When, &where, &args)
	inClause("kind", f.Kind, &where, &args)
	q := `SELECT ` + itemCols + ` FROM items`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := s.q.QueryContext(ctx, q+` ORDER BY position, priority DESC, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *DB) UpdateItem(ctx context.Context, it model.Item) error {
	if it.UpdatedAt.IsZero() {
		it.UpdatedAt = time.Now()
	}
	args := append(itemArgs(&it), it.ID)
	res, err := s.q.ExecContext(ctx, `UPDATE items SET title = ?, notes = ?, kind = ?, project_id = ?, parent_id = ?,
		status = ?, "when" = ?, context = ?, priority = ?, position = ?, why_first = ?, depends_on = ?, branch = ?,
		worktree = ?, session = ?, session_model = ?, session_started_at = ?, model = ?, effort = ?, evidence = ?,
		attachments = ?, created_at = ?, updated_at = ?, done_at = ? WHERE id = ?`, args...)
	return checkAffected(res, err)
}

func (s *DB) DeleteItem(ctx context.Context, id int64) error {
	return checkAffected(s.q.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id))
}

// --- habits ---

const habitCols = `id, name, icon, unit, type, target, step, "order", archived, created_at`

func scanHabit(r scanner) (model.Habit, error) {
	var h model.Habit
	var created string
	err := r.Scan(&h.ID, &h.Name, &h.Icon, &h.Unit, &h.Type, &h.Target, &h.Step, &h.Order, &h.Archived, &created)
	h.CreatedAt = parseTS(created)
	return h, err
}

func (s *DB) CreateHabit(ctx context.Context, h *model.Habit) error {
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now()
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO habits (name, icon, unit, type, target, step, "order", archived, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, h.Name, h.Icon, h.Unit, h.Type, h.Target, h.Step, h.Order, h.Archived, ts(h.CreatedAt))
	if err != nil {
		return err
	}
	h.ID, err = res.LastInsertId()
	return err
}

func (s *DB) ListHabits(ctx context.Context, includeArchived bool) ([]model.Habit, error) {
	q := `SELECT ` + habitCols + ` FROM habits`
	if !includeArchived {
		q += ` WHERE archived = 0`
	}
	rows, err := s.q.QueryContext(ctx, q+` ORDER BY "order", id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Habit{}
	for rows.Next() {
		h, err := scanHabit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *DB) GetHabit(ctx context.Context, id int64) (model.Habit, error) {
	h, err := scanHabit(s.q.QueryRowContext(ctx, `SELECT `+habitCols+` FROM habits WHERE id = ?`, id))
	return h, notFound(err)
}

func (s *DB) UpdateHabit(ctx context.Context, h model.Habit) error {
	return checkAffected(s.q.ExecContext(ctx, `UPDATE habits SET name = ?, icon = ?, unit = ?, type = ?, target = ?,
		step = ?, "order" = ?, archived = ? WHERE id = ?`, h.Name, h.Icon, h.Unit, h.Type, h.Target, h.Step, h.Order, h.Archived, h.ID))
}

func (s *DB) GetHabitValue(ctx context.Context, habitID int64, date string) (float64, error) {
	var v float64
	err := s.q.QueryRowContext(ctx, `SELECT value FROM habit_logs WHERE habit_id = ? AND date = ?`, habitID, date).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

func (s *DB) SetHabitValue(ctx context.Context, habitID int64, date string, v float64) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO habit_logs (habit_id, date, value) VALUES (?, ?, ?)
		ON CONFLICT (habit_id, date) DO UPDATE SET value = excluded.value`, habitID, date, v)
	return err
}

func (s *DB) HabitValuesSince(ctx context.Context, since string) (map[string]map[int64]float64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT habit_id, date, value FROM habit_logs WHERE date >= ?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[int64]float64{}
	for rows.Next() {
		var id int64
		var date string
		var v float64
		if err := rows.Scan(&id, &date, &v); err != nil {
			return nil, err
		}
		if out[date] == nil {
			out[date] = map[int64]float64{}
		}
		out[date][id] = v
	}
	return out, rows.Err()
}

// --- settings ---

func (s *DB) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
