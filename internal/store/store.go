// Package store defines the persistence boundary. SQLite implements it today;
// Postgres can replace it when the service moves to the VPS.
package store

import (
	"context"
	"errors"

	"terrancc/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type ItemFilter struct {
	ProjectID *int64
	Personal  bool // only items with no project
	Status    []model.Status
	When      []model.When
	Kind      []model.Kind
	Context   *model.Context
	ParentID  *int64
	Query     string // case-insensitive title match
}

type Store interface {
	CreateProject(ctx context.Context, p *model.Project) error
	GetProject(ctx context.Context, id int64) (model.Project, error)
	GetProjectBySlug(ctx context.Context, slug string) (model.Project, error)
	ListProjects(ctx context.Context, includeArchived bool) ([]model.Project, error)
	UpdateProject(ctx context.Context, p model.Project) error

	CreateItem(ctx context.Context, it *model.Item) error
	GetItem(ctx context.Context, id int64) (model.Item, error)
	ListItems(ctx context.Context, f ItemFilter) ([]model.Item, error)
	UpdateItem(ctx context.Context, it model.Item) error
	DeleteItem(ctx context.Context, id int64) error

	CreateHabit(ctx context.Context, h *model.Habit) error
	ListHabits(ctx context.Context, includeArchived bool) ([]model.Habit, error)
	GetHabit(ctx context.Context, id int64) (model.Habit, error)
	UpdateHabit(ctx context.Context, h model.Habit) error

	// GetHabitValue returns 0 when nothing was logged that day.
	GetHabitValue(ctx context.Context, habitID int64, date string) (float64, error)
	SetHabitValue(ctx context.Context, habitID int64, date string, v float64) error
	// HabitValuesSince returns date → habit id → value for dates >= since.
	HabitValuesSince(ctx context.Context, since string) (map[string]map[int64]float64, error)

	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error

	WithTx(ctx context.Context, fn func(Store) error) error
	Close() error
}
