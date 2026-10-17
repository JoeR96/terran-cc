// Package model holds the types shared by the service, the API and its clients.
package model

import (
	"slices"
	"time"
)

type Kind string

const (
	KindTodo    Kind = "todo"
	KindBug     Kind = "bug"
	KindTask    Kind = "task"
	KindFeature Kind = "feature"
	KindIdea    Kind = "idea"
)

var Kinds = []Kind{KindTodo, KindBug, KindTask, KindFeature, KindIdea}

func (k Kind) Valid() bool { return slices.Contains(Kinds, k) }

type Status string

const (
	StatusInbox      Status = "inbox"
	StatusBacklog    Status = "backlog"
	StatusUpNext     Status = "up_next"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusDone       Status = "done"
	StatusSomeday    Status = "someday"
)

var Statuses = []Status{StatusInbox, StatusBacklog, StatusUpNext, StatusInProgress, StatusReview, StatusDone, StatusSomeday}

func (s Status) Valid() bool { return slices.Contains(Statuses, s) }

type When string

const (
	WhenToday   When = "today"
	WhenWeek    When = "week"
	WhenMonth   When = "month"
	WhenAnytime When = "anytime"
)

var Whens = []When{WhenToday, WhenWeek, WhenMonth, WhenAnytime}

func (w When) Valid() bool { return slices.Contains(Whens, w) }

type Context string

const (
	ContextWork Context = "work"
	ContextIRL  Context = "irl"
)

func (c Context) Valid() bool { return c == ContextWork || c == ContextIRL }

type HabitType string

const (
	HabitCount  HabitType = "count"
	HabitAmount HabitType = "amount"
	HabitCheck  HabitType = "check"
)

func (h HabitType) Valid() bool { return h == HabitCount || h == HabitAmount || h == HabitCheck }

type Project struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	RepoPath    string    `json:"repo_path"`
	Archived    bool      `json:"archived"`
	CreatedAt   time.Time `json:"created_at"`
}

// EvidenceTypes lists the kinds of proof that can be attached to an item.
var EvidenceTypes = []string{"commit", "pr", "screenshot", "test", "note"}

type Evidence struct {
	Type  string    `json:"type"`
	Value string    `json:"value"`
	At    time.Time `json:"at"`
}

func (e Evidence) Valid() bool { return slices.Contains(EvidenceTypes, e.Type) }

// Item is both a personal to-do (no project) and a project ticket.
type Item struct {
	ID               int64      `json:"id"`
	Title            string     `json:"title"`
	Notes            string     `json:"notes"`
	Kind             Kind       `json:"kind"`
	ProjectID        *int64     `json:"project_id"`
	ParentID         *int64     `json:"parent_id"`
	Status           Status     `json:"status"`
	When             When       `json:"when"`
	Context          Context    `json:"context"`
	Priority         int        `json:"priority"`
	Position         int        `json:"position"`
	WhyFirst         string     `json:"why_first"`
	DependsOn        []int64    `json:"depends_on"`
	Branch           string     `json:"branch"`
	Worktree         string     `json:"worktree"`
	Session          string     `json:"session"`
	SessionModel     string     `json:"session_model"`
	SessionStartedAt *time.Time `json:"session_started_at"`
	Model            string     `json:"model"`
	Effort           string     `json:"effort"`
	Evidence         []Evidence `json:"evidence"`
	Attachments      []string   `json:"attachments"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DoneAt           *time.Time `json:"done_at"`
}

type Habit struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	Unit      string    `json:"unit"`
	Type      HabitType `json:"type"`
	Target    float64   `json:"target"`
	Step      float64   `json:"step"`
	Order     int       `json:"order"`
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"created_at"`
}

// HabitDay is a habit together with its value on one date.
type HabitDay struct {
	Habit
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Hit   bool    `json:"hit"`
}
