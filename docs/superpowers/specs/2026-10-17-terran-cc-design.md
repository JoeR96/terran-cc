# Terran CC — design

Date: 2026-10-17
Status: approved in conversation ("nothing, lets build it, go ahead autonomously")

## Why

Joe tracks everything in Obsidian notes today: rough brain dumps get tidied by hand
into task boards, ordered work lists, "ideas to decide", someday lists and a
completed table with commit/PR evidence. Getting work to Claude means copy-pasting
from Obsidian to start a worktree (see `Ideas/Workflow optimization.md`). Habits and
personal to-dos have no home at all.

Terran CC ("command centre") is one place for project tickets, personal to-dos,
daily habits and a Pomodoro timer. It runs on the Windows desktop across a
six-monitor setup, takes input from the terminal, Claude Code and the Stream Deck,
and is built so it can later move to the VPS and grow a mobile PWA.

## Goals

1. Basic project tracking that follows Joe's existing lifecycle:
   Inbox → Backlog → Up next → In progress → Review → Done (with evidence),
   plus Ideas and Someday.
2. A personal to-do list with "when" buckets: Today, This week, This month,
   Anytime. Each item is tagged `work` or `irl`.
3. Daily habit tracking with targets, quick logging and a streak.
4. A Pomodoro timer that can focus on a ticket, a to-do or nothing.
5. A desktop app: a snap-to-grid dashboard with a saved layout per monitor, plus a
   tall, narrow, always-on-top side widget.
6. A `tcc` CLI that does everything the app does, with JSON output for Claude.
7. Claude Code skills so Claude picks up tickets, records branch/worktree/session,
   files findings to the Inbox, attaches evidence and moves work to Review.
8. A Stream Deck live countdown key; other buttons run `tcc` commands.

## Non-goals (for now)

- Mobile support. No PWA, no responsive phone layout. But see Constraints.
- Obsidian integration. Terran CC has its own database. No sync, no import.
- MCP server. Comes later when the service moves to the VPS (thin layer over the
  same API, no logic of its own).
- Timer history. The timer counts down and forgets. No "time spent" stats.
- Multi-user, accounts, sharing.
- The rest of the Stream Deck front page (Claude weekly, Hue, Stremio, etc.). That
  is a separate project brainstormed after this one. Only the Pomodoro key is here.

## Anti-goals

- Jira. If it needs configuration screens, workflows-of-workflows or more than a
  handful of fields to file a ticket, it has failed.
- Chaos. Main views stay small and organised; secondary lists live in tabs.
- Screen hog. Habit and to-do tracking must work in a narrow strip.
- Logic duplicated across clients. Desktop, CLI, Stream Deck and (later) MCP and
  mobile all go through the one service.

## Constraints

- Windows 11, Go 1.24, Node 24 available. Single repo: `terran-cc`.
- Mobile-ready architecture: API-first service; frontend is web tech (Svelte) so
  components can be reused in a PWA; the service must be movable to the VPS
  (Dokploy) with SQLite → Postgres as a later option. Keep storage behind an
  interface and keep the API free of desktop-only assumptions.
- The service must run with every window closed, so CLI, Stream Deck and Claude
  always work.

## Architecture (decided)

```
            ┌──────────── tccd (Go service, tray) ────────────┐
            │ SQLite store · timer · midnight reset · events  │
            │ HTTP JSON API on 127.0.0.1 + live event stream  │
            └───────▲──────────▲───────────▲──────────▲───────┘
                    │          │           │          │
     Desktop (Wails v3 +    tcc CLI    Stream Deck   Claude Code
     Svelte): dashboard     (Go)       plugin (live  (skills → tcc)
     + side widget windows             countdown)
```

- **tccd**: Go. Owns all state and rules. Local HTTP API with a token stored in the
  user config dir; live updates pushed over Server-Sent Events or WebSocket
  (builder's choice).
- **Desktop**: Wails v3 with Svelte, pinned to `v3.0.0-beta.30` (v3 is still beta
  as of 2026-10-17; all logic lives in tccd so the shell is swappable). Two window types: the dashboard (snap grid,
  one saved layout per monitor) and the side widget (always on top). Builder
  verifies Wails v3 is stable enough for multi-window + always-on-top; fallback is
  Electron with the same Svelte frontend talking to the same API.
- **tcc**: Go CLI talking to tccd. Human output by default, `--json` for machines.
- **Stream Deck plugin**: Elgato SDK plugin with a live countdown key (shows time
  left, press to start/pause, long-press to skip). Other buttons are plain
  "run program" actions calling `tcc`.
- **Claude skills**: in the repo, installable into `~/.claude/skills`.

## Data model

**Item** — one type for both to-dos and tickets. A ticket is an item with a project.
- `title`, `notes` (markdown), `attachments` (image files, e.g. pasted screenshots)
- `kind`: `todo` | `bug` | `task` | `feature` | `idea`
- `project_id` (nullable — null means a personal to-do)
- `parent_id` (nullable — sub-tasks of a feature)
- `status`: `inbox` | `backlog` | `up_next` | `in_progress` | `review` | `done` | `someday`
  (personal to-dos only use open/done in practice; same field)
- `when`: `today` | `week` | `month` | `anytime` (personal to-dos; tickets may also
  be pulled into Today)
- `context`: `work` | `irl`
- `priority` (backlog sort) and `position` (Up next order), `why_first` text,
  `depends_on` (item ids)
- Claude/work fields: `branch`, `worktree`, `session`
- `evidence`: list of {type: commit | pr | screenshot | test | note, value, at}
- Promote: a to-do can be moved into a project and becomes a ticket.

**Project**: name, short description, archived flag.

**Habit**: name, icon, type `count` (walks, target 2) | `amount` (water 3 L step
250 ml; protein 180 g) | `check` (creatine + supplements), target, unit, step,
order. Seeded with Joe's four.

**HabitLog**: habit, date, value. Days reset at local midnight. Streak = consecutive
days where every habit hit target.

**Timer**: in memory only. Focus 25 / short break 5 / long break 15, long break
every 4 — configurable. Optional focus target (item id). Toast + sound at the end
of each phase.

**Layout**: per monitor (identified by the OS display id/name and resolution) — list
of widgets with grid position and size.

## Screens

**Dashboard** — snap grid of widgets. Widgets: Today list, Habits, Timer, Project
summary (per project), Inbox count, In progress (with Claude session), Week/Month.
Any widget can pop out to its own window.

**Project page** — Inbox strip on top ("N captures waiting · Triage"), then three
lanes: Backlog (by priority, kind tags, sub-task counts), Up next (ordered, why
first, depends on), In progress (branch/worktree/Claude session) with Recently done
(commit/PR) under it. Tabs: Board, Backlog, Ideas, Someday, Done. Review shows on
the board so Joe can sign off.

**Side widget** — tall, narrow, always on top. Sections stack in fixed order and
appear as height allows: Timer (collapses to one line when idle) → Habits (+ to
add a step, click the number to type) → Today (tick, "+ add") → This week → This
month. Sections that don't fit fold into one "▸ Week · Month" line.

Mockups from the session are in `.superpowers/brainstorm/` (board-layout-v2,
project-view, today-view, side-widget).

## Claude workflow

- Start: `tcc start <id> --branch … --worktree … --session …` → In progress.
- During: `tcc note <id>`, `tcc add --inbox --project …` for new findings.
- Finish: `tcc evidence <id> --commit … --pr … --screenshot …` then
  `tcc review <id>` → Review. Joe moves Review → Done. Claude never marks Done.
- Skills tell Claude when to do each of these. The existing `dispatch-tasks`
  skill is rewired to read tickets from Terran CC instead of the Obsidian table
  (last step of the build).

## Decided by Claude (Joe can override)

- **Windows and layouts.** Joe can open several dashboard windows, normally one per
  monitor. Each window shows one named board (a layout of widgets) and remembers its
  monitor, bounds and board; all reopen on launch. If its monitor is gone it opens on
  the primary. Popped-out widgets and the side widget also remember their bounds and
  reopen. Layout data shape: `Board {id, name, widgets[]}` and
  `WindowState {id, kind: dashboard|widget|side, board_id?, widget?, monitor, bounds}`.
- **Claude session is stored, not tracked live.** Item fields `session` (label/id),
  `session_model`, `session_started_at`. "N sessions live" = number of in-progress
  items with a session set. No heartbeat or process tracking in v1.
- **Dispatch fields.** Project gets an optional `repo_path`. Items get optional
  `model` and `effort` (free text) used by `dispatch-tasks`. Two-phase plan/build
  choices stay in the skill (it asks). Dispatch candidates are tickets in Up next.
  These fields are hidden behind "more" in the UI so filing a ticket stays a title.
- **Personal to-do status.** Personal to-dos use `backlog` as "open" and `done`.
  Ideas tab = `kind=idea`. Review is a lane on the project board after In progress.
- Unfinished to-dos roll forward: they stay in their bucket; nothing is "missed".
- Claude moves finished tickets to Review, never Done.
- Water target 3 L, step 250 ml (configurable).
- Pomodoro 25/5/15/4.
- MCP later, CLI + skills now.

## Left to the builder

Package layout, API shape, migrations, SSE vs WebSocket, styling details within the
dark mockup look, how the CLI finds the service, tray behaviour, installer/startup
on login, test strategy.
