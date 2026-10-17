# Terran CC Core (service + CLI + Claude skills) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working `tccd` service and `tcc` CLI that hold projects, tickets, to-dos, habits and a Pomodoro timer, plus Claude Code skills that drive them.

**Architecture:** `tccd` is a Go HTTP service on 127.0.0.1 that owns a SQLite database (behind a `store.Store` interface), the in-memory timer and an event bus exposed over SSE. `tcc` is a Go CLI that talks to it through `internal/client` and starts it if it isn't running. Business rules live in `internal/core`, never in handlers or the CLI.

**Tech Stack:** Go 1.24, `modernc.org/sqlite` (pure Go, no cgo), `github.com/spf13/cobra`, stdlib `net/http` (Go 1.22+ pattern routing), SSE.

**Spec:** `docs/superpowers/specs/2026-10-17-terran-cc-design.md`

Later plans (not this one): desktop app (Wails v3 + Svelte, tray, toasts, boards/windows API), Stream Deck plugin.

## Global Constraints

- Module path `terrancc`; binaries `tccd` and `tcc`; repo `terran-cc`.
- One item type for to-dos and tickets. `kind`: `todo|bug|task|feature|idea`. `status`: `inbox|backlog|up_next|in_progress|review|done|someday`. `when`: `today|week|month|anytime`. `context`: `work|irl`.
- Personal to-dos use `backlog` as "open" and `done`.
- Claude moves finished tickets to Review, never Done (enforced in the skill text; the CLI still lets Joe run `tcc done`).
- Timer counts down and forgets: state in memory only, no history table. Defaults focus 25 / short 5 / long 15 minutes, long break every 4.
- Seeded habits: Walks (count, target 2, step 1, 🚶), Water (amount, target 3000 ml, step 250, 💧), Protein (amount, target 180 g, step 20, 🥩), Creatine + supplements (check, 💊).
- Days reset at local midnight. Streak = consecutive days where every habit hit target.
- Service binds 127.0.0.1 only and requires `Authorization: Bearer <token>`. No logic in clients. API must not assume a desktop (it moves to the VPS later).
- Storage behind `store.Store` so Postgres can replace SQLite later.
- Claude session is stored, not tracked: `session`, `session_model`, `session_started_at`.
- Dispatch fields: project `repo_path`; item `model`, `effort` (free text); candidates are `up_next` tickets.

## Review Focus

1. **Midnight while running.** A habit logged after midnight lands on the new day starting from 0, and an unfinished today doesn't break yesterday's streak. Pinned in Task 4 (`TestLogUsesLocalDate`, `TestStreakTodayIncomplete`).
2. **Service not running or stale `daemon.json`.** `tcc` (and Stream Deck buttons) start `tccd` and succeed within 5 s, or fail with a clear message — never hang. Pinned in Task 7 (`TestEnsureStartsDaemon`, `TestStaleDaemonFile`).
3. **Concurrent writers** (CLI, UI, Claude at once). No `database is locked` errors. WAL + `busy_timeout=5000`, pinned in Task 2 (`TestConcurrentWrites`).
4. **Bad references** — unknown id, unknown project slug, ambiguous habit name. Clear message, non-zero exit, `{"error": ...}` in `--json`. Pinned in Task 8 (`TestUnknownItemExitCode`, `TestAmbiguousHabit`).
5. **Dependency cycles and deleting a depended-on item.** Cycles rejected; deleting removes it from others' `depends_on`. Pinned in Task 3 (`TestDependsOnCycle`, `TestDeleteCleansDependsOn`).

---

## File Structure

```
go.mod
cmd/tccd/main.go                 service entry: config, open store, seed, serve, write daemon.json
cmd/tcc/main.go                  CLI entry
internal/model/model.go          types + enum validation
internal/config/config.go        home dir, daemon.json, token
internal/store/store.go          Store interface + filters + errors
internal/store/sqlite/sqlite.go  SQLite implementation
internal/store/sqlite/migrations/001_init.sql
internal/core/items.go           item rules (defaults, transitions, deps, evidence, notes)
internal/core/habits.go          habit logging + streak
internal/core/clock.go           Clock interface
internal/timer/timer.go          Pomodoro state machine
internal/events/bus.go           pub/sub
internal/api/server.go           routes, auth, JSON helpers
internal/api/items.go, habits.go, timer.go, projects.go, events.go
internal/client/client.go        typed HTTP client + EnsureDaemon
internal/cli/*.go                cobra commands, one file per noun
skills/terran-cc/SKILL.md        Claude workflow skill
skills/dispatch-tasks/           rewired copy of Joe's skill
scripts/install.ps1              go install binaries + copy skills into ~/.claude/skills
README.md
```

---

### Task 1: Scaffold, model types, config

**Files:**
- Create: `go.mod`, `internal/model/model.go`, `internal/config/config.go`
- Test: `internal/model/model_test.go`, `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type Kind, Status, When, Context, HabitType string` with consts (`KindTodo`, … `StatusInbox` … `WhenToday` … `ContextWork`, `ContextIRL`, `HabitCount`, `HabitAmount`, `HabitCheck`) and `func (x T) Valid() bool` on each.
  - `type Project struct { ID int64; Slug, Name, Description, RepoPath string; Archived bool; CreatedAt time.Time }` (json tags snake_case).
  - `type Item struct { ID int64; Title, Notes string; Kind Kind; ProjectID, ParentID *int64; Status Status; When When; Context Context; Priority, Position int; WhyFirst string; DependsOn []int64; Branch, Worktree, Session, SessionModel string; SessionStartedAt *time.Time; Model, Effort string; Evidence []Evidence; Attachments []string; CreatedAt, UpdatedAt time.Time; DoneAt *time.Time }`
  - `type Evidence struct { Type string /* commit|pr|screenshot|test|note */; Value string; At time.Time }` and `func (e Evidence) Valid() bool`.
  - `type Habit struct { ID int64; Name, Icon, Unit string; Type HabitType; Target, Step float64; Order int; Archived bool; CreatedAt time.Time }`
  - `type HabitDay struct { Habit; Date string; Value float64; Hit bool }`
  - `config.Home() (string, error)` — `$TCC_HOME` else `os.UserConfigDir()/terran-cc`, created if missing.
  - `type Daemon struct { Addr, Token string; PID int }`; `config.WriteDaemon(home string, d Daemon) error`; `config.ReadDaemon(home string) (Daemon, error)` (file `daemon.json`).
  - `config.Token(home string) (string, error)` — reads `token`, or creates 32 random bytes hex with 0600.

- [ ] **Step 1:** `go mod init terrancc`; write tests: `TestEnumsValid` (every const valid, `"nope"` invalid for each type, `Evidence{Type:"commit"}` valid, `{Type:"x"}` invalid); `TestHomeUsesEnv` (`t.Setenv("TCC_HOME", t.TempDir())` → returned path equals it and exists); `TestTokenStable` (two calls return same 64-char hex); `TestDaemonRoundTrip`.
- [ ] **Step 2:** `go test ./...` → FAIL (undefined).
- [ ] **Step 3:** Implement the types and functions above.
- [ ] **Step 4:** `go test ./...` → PASS.
- [ ] **Step 5:** Commit `feat: model types and config`.

### Task 2: Store interface + SQLite

**Files:**
- Create: `internal/store/store.go`, `internal/store/sqlite/sqlite.go`, `internal/store/sqlite/migrations/001_init.sql`
- Test: `internal/store/sqlite/sqlite_test.go`

**Interfaces:**
- Consumes: `model.*`.
- Produces:
  - `var ErrNotFound = errors.New("not found")`, `ErrConflict`.
  - `type ItemFilter struct { ProjectID *int64; Personal bool /* project_id IS NULL */; Status []model.Status; When []model.When; Kind []model.Kind; Context *model.Context; ParentID *int64; Query string /* title LIKE */ }`
  - `type Store interface {`
    `CreateProject(ctx, *model.Project) error; GetProject(ctx, id int64) (model.Project, error); GetProjectBySlug(ctx, slug string) (model.Project, error); ListProjects(ctx, includeArchived bool) ([]model.Project, error); UpdateProject(ctx, model.Project) error;`
    `CreateItem(ctx, *model.Item) error; GetItem(ctx, id int64) (model.Item, error); ListItems(ctx, ItemFilter) ([]model.Item, error); UpdateItem(ctx, model.Item) error; DeleteItem(ctx, id int64) error;`
    `CreateHabit(ctx, *model.Habit) error; ListHabits(ctx, includeArchived bool) ([]model.Habit, error); GetHabit(ctx, id int64) (model.Habit, error); UpdateHabit(ctx, model.Habit) error;`
    `GetHabitValue(ctx, habitID int64, date string) (float64, error) /* 0 if none */; SetHabitValue(ctx, habitID int64, date string, v float64) error; HabitValuesSince(ctx, since string) (map[string]map[int64]float64, error) /* date→habit→value */;`
    `GetSetting(ctx, key string) (string, bool, error); SetSetting(ctx, key, value string) error;`
    `WithTx(ctx, func(Store) error) error; Close() error }`
  - `sqlite.Open(path string) (*sqlite.DB, error)` — pragmas `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`; runs embedded migrations tracked in `schema_version`.
- Schema: `projects`, `items` (depends_on, evidence, attachments as JSON text columns), `habits`, `habit_logs (habit_id, date TEXT, value REAL, PRIMARY KEY(habit_id,date))`, `settings (key PRIMARY KEY, value)`. Slug unique. ListItems orders by `position, priority DESC, id`.

- [ ] **Step 1:** Tests with a temp-file DB: `TestProjectCRUD` (slug lookup, duplicate slug → `ErrConflict`); `TestItemRoundTrip` (all fields incl. `DependsOn=[1,2]`, `Evidence`, nil pointers survive); `TestListItemsFilters` (project, Personal, status set, when set, Query case-insensitive); `TestGetMissing` → `ErrNotFound`; `TestHabitValues` (missing → 0, set/get, `HabitValuesSince`); `TestConcurrentWrites` (20 goroutines × 10 `CreateItem` on two `Open`ed handles of the same file → no error, 200 rows); `TestMigrationsIdempotent` (open twice).
- [ ] **Step 2:** `go test ./internal/store/...` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./internal/store/...` → PASS.
- [ ] **Step 5:** Commit `feat: sqlite store`.

### Task 3: Item rules (`internal/core`)

**Files:**
- Create: `internal/core/clock.go`, `internal/core/items.go`
- Test: `internal/core/items_test.go`

**Interfaces:**
- Consumes: `store.Store`, `model.*`.
- Produces:
  - `type Clock interface { Now() time.Time }`; `SystemClock{}`; test `FixedClock`.
  - `type Service struct { Store store.Store; Clock Clock; OnChange func(kind string, id int64) }` — `OnChange("item"|"project"|"habit", id)` called after every successful write (nil-safe).
  - `type NewItem struct { Title, Notes string; Kind model.Kind; ProjectID, ParentID *int64; Status model.Status; When model.When; Context model.Context; Priority int; Inbox bool }`
  - `(s *Service) AddItem(ctx, NewItem) (model.Item, error)`
  - `type ItemPatch struct` — pointer field per editable field: `Title, Notes, WhyFirst, Branch, Worktree, Model, Effort *string; Kind *model.Kind; Status *model.Status; When *model.When; Context *model.Context; Priority, Position *int; ProjectID **int64; ParentID **int64; DependsOn *[]int64`.
  - `(s *Service) EditItem(ctx, id int64, ItemPatch) (model.Item, error)`
  - `StartItem(ctx, id int64, StartInfo{Branch, Worktree, Session, SessionModel string}) (model.Item, error)`
  - `ReviewItem(ctx, id) (model.Item, error)`, `DoneItem(ctx, id)`, `ReopenItem(ctx, id)`
  - `AppendNote(ctx, id int64, text string) (model.Item, error)`
  - `AddEvidence(ctx, id int64, ev []model.Evidence) (model.Item, error)`
  - `DeleteItem(ctx, id int64) error`
  - `AddProject(ctx, name, slug, desc, repo string) (model.Project, error)`, `EditProject(ctx, id int64, ProjectPatch{Name, Description, RepoPath *string; Archived *bool})`
  - `var ErrInvalid` — wrapped with a message (`fmt.Errorf("%w: when must be today|week|month|anytime", ErrInvalid)`).
- Rules:
  - Title trimmed, required, ≤ 300 chars. Enum fields validated.
  - Defaults: `Kind` = `task` if project else `todo`. `Status` = `inbox` if `Inbox`, else `backlog`. `When` = `anytime`. `Context` = `work` if project else `irl`. Parent must exist and share project.
  - Slug: lowercased, non-alnum → `-`, trimmed; generated from name if empty.
  - Promote: patching `ProjectID` from nil to a project on a `todo` sets `Kind=task`.
  - `StartItem`: status → `in_progress`, sets non-empty info fields, `SessionStartedAt=now` when `Session` given. Not allowed from `done` (`ErrInvalid`).
  - `ReviewItem`: allowed from any status except `done`; → `review`. `DoneItem`: → `done`, `DoneAt=now`. `ReopenItem`: done → `backlog`, `DoneAt=nil`.
  - Patching `Status` to `done` sets `DoneAt`; away from `done` clears it.
  - `AppendNote`: appends `"\n\n**<YYYY-MM-DD HH:MM>** <text>"` (no leading blank lines if notes empty).
  - `AddEvidence`: each must be `Valid()`, `At` defaults to now.
  - `DependsOn`: ids must exist, no self, no cycle (DFS over stored `DependsOn`).
  - `DeleteItem`: inside `WithTx`, removes id from every item's `DependsOn`, re-parents children to nil.

- [ ] **Step 1:** Tests (fresh sqlite in temp dir, `FixedClock`): `TestAddDefaults` (personal → todo/backlog/anytime/irl; project → task/backlog/work; `Inbox:true` → inbox); `TestAddValidation` (empty title, bad when → `errors.Is(err, ErrInvalid)`); `TestPromoteTodo`; `TestStartReviewDone` (fields + timestamps; start from done → ErrInvalid; reopen); `TestAppendNote` (exact format with fixed clock `2026-10-17 14:02`); `TestEvidenceInvalid`; `TestDependsOnCycle` (1→2, then 2→1 rejected; self rejected; missing id rejected); `TestDeleteCleansDependsOn`; `TestSlugify` (`"BDTV Bonanza"` → `bdtv-bonanza`); `TestOnChangeCalled`.
- [ ] **Step 2:** `go test ./internal/core/...` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat: item rules`.

### Task 4: Habits + streak

**Files:**
- Create: `internal/core/habits.go`
- Test: `internal/core/habits_test.go`

**Interfaces:**
- Consumes: `Service`, `store.Store`.
- Produces:
  - `(s *Service) Today() string` — `s.Clock.Now().Local()` formatted `2006-01-02`.
  - `(s *Service) HabitsFor(ctx, date string) ([]model.HabitDay, error)` — non-archived, by `Order`; `Hit = Value >= Target` (check: target 1).
  - `type LogOp struct { Delta *float64; Set *float64; Toggle bool }`
  - `(s *Service) LogHabit(ctx, habitID int64, op LogOp) (model.HabitDay, error)` — today's date; result clamped ≥ 0; `check` habits: Toggle flips 0/1, Delta/Set clamp to 0..1. No op given → Delta = Step (check: Toggle).
  - `(s *Service) FindHabit(ctx, ref string) (model.Habit, error)` — numeric id, else case-insensitive exact name, else unique prefix; 0 matches → `store.ErrNotFound`; >1 → `ErrInvalid` "ambiguous: Water, Walks".
  - `(s *Service) AddHabit(ctx, model.Habit) (model.Habit, error)`, `EditHabit(ctx, id, HabitPatch{Name, Icon, Unit *string; Target, Step *float64; Order *int; Archived *bool})`
  - `(s *Service) Streak(ctx) (int, error)`
  - `(s *Service) SeedHabits(ctx) error` — inserts the four Global Constraints habits only when there are no habits at all.
- Streak algorithm: walk back from today, max 3650 days. A day counts if every non-archived habit with `CreatedAt` date ≤ that day hit its current target; days with no such habits stop the walk. If today isn't complete, start from yesterday without breaking (today never breaks a streak).

- [ ] **Step 1:** Tests: `TestSeedOnce` (4 habits, second call no-op); `TestLogDeltaDefaultStep` (water +250 → 250); `TestLogClamp` (−500 on 250 → 0); `TestCheckToggle`; `TestLogUsesLocalDate` (clock at 23:59 log, move clock to 00:01 → today value 0, yesterday 250); `TestStreak` (3 full days + today full → 4); `TestStreakTodayIncomplete` (3 full days + today partial → 3); `TestStreakIgnoresNewHabit` (habit created today doesn't break past days); `TestFindHabit` (`"wa"` ambiguous between Walks/Water, `"wat"` → Water, `"3"` → id 3, `"zzz"` → ErrNotFound).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat: habits and streak`.

### Task 5: Pomodoro timer

**Files:**
- Create: `internal/timer/timer.go`
- Test: `internal/timer/timer_test.go`

**Interfaces:**
- Produces:
  - `type Phase string` (`focus`, `short_break`, `long_break`); `type State string` (`idle`, `running`, `paused`).
  - `type Config struct { Focus, Short, Long time.Duration; LongEvery int; AutoStart bool }`; `DefaultConfig()` = 25m/5m/15m/4/false.
  - `type Snapshot struct { Phase Phase; State State; Remaining time.Duration /* json as seconds int "remaining_s" */; Cycle int /* completed focus sessions */; ItemID *int64; Config Config }`
  - `New(cfg Config, now func() time.Time) *Timer` (mutex-guarded)
  - Methods returning `Snapshot`: `Start(itemID *int64)` (idle→running for current phase; if itemID non-nil sets it), `Pause()`, `Resume()`, `Toggle()` (idle→Start(nil), running→Pause, paused→Resume), `Skip()`, `Stop()` (→ idle focus, cycle 0, item nil), `Snapshot()`, `SetConfig(Config)` (applies to next phase).
  - `Advance() (Snapshot, bool)` — called each second by the server; when a running phase reaches 0 it moves to the next phase and returns `true`. Next phase: after focus `Cycle++` then `long_break` if `Cycle%LongEvery==0` else `short_break`; after a break → `focus`. New phase is `running` if `AutoStart`, else `idle` with full duration. `ItemID` is kept across phases until `Stop`.
  - `Skip()` ends the current phase the same way (counts as completed for focus).
  - Remaining computed from a stored end time while running, so pausing/resuming is exact.

- [ ] **Step 1:** Tests with a fake `now`: `TestStartCountsDown` (advance fake clock 60 s → remaining 24m); `TestPauseResume` (pause, +10 min, resume → remaining unchanged by the pause); `TestPhaseEnd` (+25 m → `Advance` true, phase short_break, idle, cycle 1); `TestLongBreakEvery4`; `TestAutoStart`; `TestToggle` cycle; `TestStopResets`; `TestItemKept`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat: pomodoro timer`.

### Task 6: Event bus + HTTP API

**Files:**
- Create: `internal/events/bus.go`, `internal/api/server.go`, `internal/api/items.go`, `internal/api/projects.go`, `internal/api/habits.go`, `internal/api/timer.go`, `internal/api/events.go`
- Test: `internal/events/bus_test.go`, `internal/api/api_test.go`

**Interfaces:**
- Consumes: `core.Service`, `timer.Timer`, `store`.
- Produces:
  - `events.Event{ Type string; ID int64 `json:",omitempty"`; Data any `json:",omitempty"` }`; `Bus.Publish(Event)`, `Bus.Subscribe() (<-chan Event, func())` — buffered 64, drops for slow subscribers.
  - Event types: `item.changed`, `project.changed`, `habit.changed`, `timer.tick` (Data = Snapshot, every second while running), `timer.phase` (Data = Snapshot, on phase end — clients toast/sound).
  - `api.New(svc *core.Service, t *timer.Timer, bus *events.Bus, token string) http.Handler`; `api.RunTicker(ctx, t, bus)` — 1 s ticker calling `Advance`.
  - Timer config persisted under settings key `timer.config` (JSON) and loaded at startup by `cmd/tccd`.
  - Routes (all JSON, all under bearer auth except `GET /api/health`):
    - `GET /api/health` → `{"ok":true,"version":"..."}`
    - `GET /api/projects`, `POST /api/projects`, `GET /api/projects/{ref}` (id or slug), `PATCH /api/projects/{ref}`
    - `GET /api/items?project=&personal=1&status=a,b&when=&kind=&context=&parent=&q=`, `POST /api/items`, `GET|PATCH|DELETE /api/items/{id}`
    - `POST /api/items/{id}/start|review|done|reopen`, `POST /api/items/{id}/note {"text"}`, `POST /api/items/{id}/evidence {"evidence":[...]}`, `POST /api/items/{id}/attachments` (multipart `file`, saved to `<home>/attachments/<id>/<basename>`, appended to `Attachments`)
    - `GET /api/habits?date=` → `{"date","streak","habits":[HabitDay]}`, `POST /api/habits`, `PATCH /api/habits/{ref}`, `POST /api/habits/{ref}/log {"delta"|"set"|"toggle"}`
    - `GET /api/timer`, `POST /api/timer/start {"item_id"?}|pause|resume|toggle|skip|stop`, `PUT /api/timer/config`
    - `GET /api/events` — SSE, `event: <type>\ndata: <json>\n\n`, 15 s `: ping` comments.
  - Error mapping: `ErrNotFound`→404, `ErrInvalid`→400, `ErrConflict`→409, else 500; body `{"error":"<message>"}`. Missing/bad token → 401.
  - `Service.OnChange` wired to publish the matching `*.changed` event.

- [ ] **Step 1:** Tests using `httptest`: `TestHealthNoAuth`; `TestAuthRequired` (401 without token); `TestItemLifecycleHTTP` (POST item → start → evidence → review → GET shows status review, evidence 1); `TestProjectBySlug`; `TestErrorMapping` (GET /api/items/999 → 404 `{"error":...}`; bad when → 400); `TestHabitLogHTTP` (by name `water`); `TestTimerToggleHTTP`; `TestSSE` (subscribe, POST an item, read `event: item.changed` within 1 s); `TestBusDropsSlow`.
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Commit `feat: http api and events`.

### Task 7: Client + `tccd`

**Files:**
- Create: `internal/client/client.go`, `cmd/tccd/main.go`
- Test: `internal/client/client_test.go`

**Interfaces:**
- Consumes: API routes from Task 6, `config`.
- Produces:
  - `client.New(addr, token string) *Client`; typed methods mirroring the routes: `Health`, `ListProjects`, `GetProject(ref)`, `AddProject`, `EditProject`, `ListItems(url.Values)`, `GetItem`, `AddItem(core.NewItem)`, `EditItem(id, map[string]any)`, `Action(id, "start"|"review"|"done"|"reopen", body any)`, `Note`, `Evidence`, `Attach(id, path)`, `DeleteItem`, `Habits(date)`, `LogHabit(ref, body)`, `AddHabit`, `EditHabit`, `Timer()`, `TimerAction(name, body)`, `TimerConfig(cfg)`. Errors from non-2xx are `*client.APIError{Status int; Message string}`.
  - `client.EnsureDaemon(home string, tccdPath string, timeout time.Duration) (*Client, error)` — read `daemon.json`; if `Health` OK return; otherwise start `tccdPath` detached (Windows: `CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS`, no console window) and poll `daemon.json` + `Health` every 100 ms until `timeout` (CLI uses 5 s). Timeout error: `terran-cc service did not start (see <home>/tccd.log)`.
  - `cmd/tccd`: flags `--addr` (default `127.0.0.1:47821`, env `TCC_ADDR`), `--home`; logs to `<home>/tccd.log`; opens `<home>/tcc.db`; `SeedHabits`; loads timer config; if the addr is in use by a healthy tccd, exit 0; writes `daemon.json` after listening; removes it on clean shutdown (Ctrl+C / SIGTERM).

- [ ] **Step 1:** Tests: `TestClientRoundTrip` (client against `httptest` server from Task 6: add project, add item, list); `TestAPIError` (404 → `*APIError` Status 404); `TestEnsureStartsDaemon` (build `tccd` into temp dir with `go build`, `TCC_HOME` + `TCC_ADDR=127.0.0.1:0`-style free port, EnsureDaemon returns healthy client < 5 s; kill afterwards); `TestStaleDaemonFile` (write daemon.json pointing at a closed port → EnsureDaemon still starts a fresh one).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Manual check: `go run ./cmd/tccd` then `curl http://127.0.0.1:47821/api/health` → `{"ok":true,...}`.
- [ ] **Step 6:** Commit `feat: client and tccd`.

### Task 8: `tcc` CLI

**Files:**
- Create: `cmd/tcc/main.go`, `internal/cli/root.go`, `items.go`, `projects.go`, `habits.go`, `timer.go`, `status.go`, `output.go`
- Test: `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `client.Client`, `client.EnsureDaemon`.
- Produces: `cli.NewRoot(getClient func() (*client.Client, error), out, errOut io.Writer) *cobra.Command`. `tccd` path = `tccd.exe` next to the running `tcc` executable, else `tccd` on PATH.
- Global flag `--json`: print the API JSON exactly; errors as `{"error":"..."}` on stdout. Exit codes: 0 ok, 1 error, 2 not found.
- Commands (ids accept `12` or `#12`; `--project` accepts slug or id):
  - `tcc status` — date, streak, habits (`🚶 Walks 1/2`), timer, Today to-dos, in-progress tickets.
  - `tcc add <title> [--project --kind --inbox --when --context --priority --parent --notes]`
  - `tcc todo <title> [--when today(default) --context]`
  - `tcc ls [--project --personal --status --when --kind --context --all]` — default hides `done` and `someday`. Human output: `#12  bug   backlog  Fix bot melee attacks  [bdtv-bonanza]`.
  - `tcc next [--project]` — `up_next` ordered by position.
  - `tcc show <id>` — all fields, notes, evidence.
  - `tcc edit <id> [--title --notes --kind --status --when --context --priority --position --why-first --depends-on 3,4 --project (empty string = personal) --parent --branch --worktree --model --effort]`
  - `tcc start <id> [--branch --worktree --session --session-model]`, `tcc review <id>`, `tcc done <id>`, `tcc reopen <id>`, `tcc rm <id>`
  - `tcc note <id> <text>`; `tcc evidence <id> [--commit --pr --screenshot --test --note]` (each repeatable); `tcc attach <id> <file>`
  - `tcc project ls|add <name> [--slug --repo --desc]|edit <ref> [...]|show <ref>` (show = board summary counts by status)
  - `tcc habit [ls]`, `tcc habit log <ref> [+N|-N|=N|toggle]`, `tcc habit add <name> --type --target --unit --step --icon`, `tcc habit edit <ref> ...`
  - `tcc timer [status]|start [--item]|pause|resume|toggle|skip|stop|config [--focus 25 --short 5 --long 15 --every 4 --auto-start]`
  - Numbers in `habit log` and timer config are plain numbers (`+250`, `--focus 25` = minutes).

- [ ] **Step 1:** Tests run the root command against an in-process `httptest` server (Task 6) via `getClient`: `TestAddAndLs`; `TestJSONOutput` (`ls --json` parses as `[]model.Item`); `TestUnknownItemExitCode` (`show 999` → exit 2, `--json` → `{"error":...}`); `TestAmbiguousHabit` (`habit log wa` → exit 1, message contains `ambiguous`); `TestHabitLogSyntax` (`+250`, `-100`, `=1800`, `toggle`); `TestEditProjectEmptyMeansPersonal`; `TestStartReviewFlow`; `TestHashID` (`#12` accepted).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** PASS.
- [ ] **Step 5:** Manual check with a real daemon: `go build -o bin/ ./cmd/...`; `bin/tcc status` starts tccd and prints 4 habits; `bin/tcc habit log water` → `💧 Water 250/3000 ml`; `bin/tcc timer toggle` → running 24:59.
- [ ] **Step 6:** Commit `feat: tcc cli`.

### Task 9: Claude skills, install, README

**Files:**
- Create: `skills/terran-cc/SKILL.md`, `skills/dispatch-tasks/SKILL.md` (+ copy of `brief-common.md`), `scripts/install.ps1`, `README.md`

**Interfaces:**
- Consumes: CLI from Task 8.
- `skills/terran-cc/SKILL.md` — frontmatter `name: terran-cc`, description triggering when working on a Terran CC ticket, when Joe mentions a ticket id, or when finishing work that came from Terran CC. Body: the Claude workflow from the spec — `tcc show` before starting; `tcc start <id> --branch --worktree --session <herdr name or session id> --session-model <model id>`; `tcc note` for decisions; `tcc add --project <slug> --inbox` for new findings (never into backlog directly); on finish `tcc evidence` with commit(s)/PR/screenshots/test command and `tcc review <id>`; **never `tcc done`**; use `--json` when parsing.
- `skills/dispatch-tasks/SKILL.md` — same steps as Joe's current skill, but step 2 reads `tcc next --project <slug> --json` (fallback `tcc ls --status up_next`), repo from `tcc project show <slug> --json` `repo_path`, model/effort from the item's `model`/`effort` (text may be `Plan: X high. Build: Y medium.` → two-phase), branch/worktree from the item if set, otherwise derived as now. After launching each agent run `tcc start <id> --branch … --worktree … --session <name> --session-model <id>`. Brief tells the agent to follow the `terran-cc` skill. Keep everything else from the current skill verbatim.
- `scripts/install.ps1` — `go install ./cmd/tcc ./cmd/tccd`; copy `skills/*` to `$HOME\.claude\skills\` (back up an existing `dispatch-tasks` to `dispatch-tasks.bak-<date>` first); print where binaries went.
- `README.md` — what it is, install, CLI cheatsheet, Stream Deck button commands (`tcc timer toggle`, `tcc habit log water`, `tcc habit log walks`), layout of the repo.

- [ ] **Step 1:** Write the files.
- [ ] **Step 2:** Verify: `pwsh -File scripts/install.ps1` succeeds; `tcc --help` lists commands; `~/.claude/skills/terran-cc/SKILL.md` exists; `~/.claude/skills/dispatch-tasks.bak-*` exists.
- [ ] **Step 3:** Commit `feat: claude skills, install script, readme`.
