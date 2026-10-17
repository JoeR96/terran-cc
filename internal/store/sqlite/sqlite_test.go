package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"terrancc/internal/model"
	"terrancc/internal/store"
)

var ctx = context.Background()

func open(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ptr[T any](v T) *T { return &v }

func TestProjectCRUD(t *testing.T) {
	db := open(t)
	p := model.Project{Slug: "bdtv-bonanza", Name: "BDTV Bonanza", RepoPath: `C:\repo`}
	if err := db.CreateProject(ctx, &p); err != nil || p.ID == 0 {
		t.Fatalf("create: %v id=%d", err, p.ID)
	}
	got, err := db.GetProjectBySlug(ctx, "bdtv-bonanza")
	if err != nil || got.Name != "BDTV Bonanza" || got.RepoPath != `C:\repo` {
		t.Fatalf("by slug: %+v %v", got, err)
	}
	dup := model.Project{Slug: "bdtv-bonanza", Name: "x"}
	if err := db.CreateProject(ctx, &dup); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	got.Archived = true
	if err := db.UpdateProject(ctx, got); err != nil {
		t.Fatal(err)
	}
	if l, _ := db.ListProjects(ctx, false); len(l) != 0 {
		t.Fatalf("archived listed: %v", l)
	}
	if l, _ := db.ListProjects(ctx, true); len(l) != 1 {
		t.Fatalf("include archived: %v", l)
	}
}

func TestItemRoundTrip(t *testing.T) {
	db := open(t)
	now := time.Date(2026, 10, 17, 14, 2, 0, 0, time.UTC)
	it := model.Item{
		Title: "Fix bot melee 🔫", Notes: "n", Kind: model.KindBug, Status: model.StatusBacklog,
		When: model.WhenToday, Context: model.ContextWork, Priority: 3, Position: 1, WhyFirst: "w",
		DependsOn: []int64{1, 2}, Branch: "b", Worktree: `D:\w t`, Session: "s", SessionModel: "opus",
		SessionStartedAt: &now, Model: "m", Effort: "high",
		Evidence:    []model.Evidence{{Type: "commit", Value: "abc", At: now}},
		Attachments: []string{"a.png"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.CreateItem(ctx, &it); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetItem(ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != it.Title || got.Worktree != it.Worktree || len(got.DependsOn) != 2 || got.DependsOn[1] != 2 ||
		len(got.Evidence) != 1 || got.Evidence[0].Value != "abc" || got.SessionStartedAt == nil ||
		!got.SessionStartedAt.Equal(now) || got.ProjectID != nil || got.DoneAt != nil || got.Effort != "high" ||
		len(got.Attachments) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestListItemsFilters(t *testing.T) {
	db := open(t)
	p := model.Project{Slug: "p", Name: "P"}
	db.CreateProject(ctx, &p)
	mk := func(title string, pid *int64, st model.Status, w model.When) {
		it := model.Item{Title: title, ProjectID: pid, Status: st, When: w, Kind: model.KindTask, Context: model.ContextWork}
		if err := db.CreateItem(ctx, &it); err != nil {
			t.Fatal(err)
		}
	}
	mk("Alpha", &p.ID, model.StatusBacklog, model.WhenAnytime)
	mk("Beta", &p.ID, model.StatusDone, model.WhenToday)
	mk("gamma", nil, model.StatusBacklog, model.WhenToday)

	cases := []struct {
		name string
		f    store.ItemFilter
		want int
	}{
		{"project", store.ItemFilter{ProjectID: &p.ID}, 2},
		{"personal", store.ItemFilter{Personal: true}, 1},
		{"status", store.ItemFilter{Status: []model.Status{model.StatusBacklog}}, 2},
		{"when", store.ItemFilter{When: []model.When{model.WhenToday}}, 2},
		{"query", store.ItemFilter{Query: "GAM"}, 1},
		{"all", store.ItemFilter{}, 3},
	}
	for _, c := range cases {
		got, err := db.ListItems(ctx, c.f)
		if err != nil || len(got) != c.want {
			t.Errorf("%s: got %d (%v) want %d", c.name, len(got), err, c.want)
		}
	}
}

func TestGetMissing(t *testing.T) {
	db := open(t)
	if _, err := db.GetItem(ctx, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("item: %v", err)
	}
	if _, err := db.GetProject(ctx, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("project: %v", err)
	}
	if _, err := db.GetHabit(ctx, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("habit: %v", err)
	}
	if err := db.UpdateItem(ctx, model.Item{ID: 99, Title: "x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
}

func TestHabitValues(t *testing.T) {
	db := open(t)
	h := model.Habit{Name: "Water", Type: model.HabitAmount, Target: 3000, Step: 250}
	if err := db.CreateHabit(ctx, &h); err != nil {
		t.Fatal(err)
	}
	if v, err := db.GetHabitValue(ctx, h.ID, "2026-10-17"); err != nil || v != 0 {
		t.Fatalf("missing value: %v %v", v, err)
	}
	db.SetHabitValue(ctx, h.ID, "2026-10-16", 500)
	db.SetHabitValue(ctx, h.ID, "2026-10-17", 250)
	db.SetHabitValue(ctx, h.ID, "2026-10-17", 750)
	if v, _ := db.GetHabitValue(ctx, h.ID, "2026-10-17"); v != 750 {
		t.Fatalf("got %v", v)
	}
	m, err := db.HabitValuesSince(ctx, "2026-10-17")
	if err != nil || len(m) != 1 || m["2026-10-17"][h.ID] != 750 {
		t.Fatalf("since: %v %v", m, err)
	}
}

func TestSettings(t *testing.T) {
	db := open(t)
	if _, ok, _ := db.GetSetting(ctx, "k"); ok {
		t.Fatal("unexpected setting")
	}
	db.SetSetting(ctx, "k", "1")
	db.SetSetting(ctx, "k", "2")
	if v, ok, _ := db.GetSetting(ctx, "k"); !ok || v != "2" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestWithTxRollback(t *testing.T) {
	db := open(t)
	boom := errors.New("boom")
	err := db.WithTx(ctx, func(s store.Store) error {
		it := model.Item{Title: "x", Kind: model.KindTodo, Status: model.StatusBacklog, When: model.WhenAnytime, Context: model.ContextIRL}
		if err := s.CreateItem(ctx, &it); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if l, _ := db.ListItems(ctx, store.ItemFilter{}); len(l) != 0 {
		t.Fatalf("rollback failed: %d items", len(l))
	}
}

func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for g := 0; g < 20; g++ {
		db := a
		if g%2 == 1 {
			db = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				it := model.Item{Title: "x", Kind: model.KindTodo, Status: model.StatusBacklog, When: model.WhenAnytime, Context: model.ContextIRL}
				if err := db.CreateItem(ctx, &it); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if l, _ := a.ListItems(ctx, store.ItemFilter{}); len(l) != 200 {
		t.Fatalf("got %d rows", len(l))
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	for i := 0; i < 2; i++ {
		db, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		db.Close()
	}
}
