# Terran CC Desktop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Windows desktop app for Terran CC: snap-grid dashboards (one per monitor), the tall always-on-top side widget, pop-out widgets, project pages and a tray icon, all talking to the running tccd.

**Architecture:** `desktop/` is its own Go module (`terrancc/desktop`, replace `terrancc => ../`) so the core stays free of Wails. The Wails shell owns windows only: it starts tccd with `client.EnsureDaemon`, injects `window.__TCC__ = {base, token, win}` into every window, and serves a tiny `/shell/*` HTTP API from the asset-server middleware (open/close windows, save state). The Svelte frontend calls tccd directly over HTTP (CORS) and listens to `/api/events?token=` for live updates. Boards and window states live in tccd under `/api/ui/{boards|windows}`, so the layout survives a move to the VPS.

**Tech Stack:** Wails `v3.0.0-beta.30`, Svelte 5 + Vite, vitest, Go.

**Spec:** `docs/superpowers/specs/2026-10-17-terran-cc-design.md`

## Global Constraints

- Wails pinned to `v3.0.0-beta.30`. No `wails3` CLI needed: build with `npm run build` then `go build -ldflags -H=windowsgui`.
- All rules stay in tccd. The frontend only calls the API; the shell only manages windows.
- Dark mockup look: background `#14161b`, panels `#1a1d24`/`#1f232c`, borders `#2a2f3b`/`#343a47`, text `#d6d9e0`, muted `#8b93a7`, accent `#f08a5d`, green `#a6e3a1`. Kind tags: bug `#4a2326/#ff9b9b`, task `#23334a/#9cc4ff`, feature `#2b3f2a/#a6e3a1`, idea `#3d3524/#f5d58a`.
- Side widget sections in fixed order: Timer (one line when idle) → Habits (+ step, click number to type) → Today (tick, + add) → This week → This month. Sections that don't fit fold into one "▸ Week · Month" line.
- Board shape `{id, name, widgets:[{id, type, x, y, w, h, project_id?}]}`; window shape `{id, kind: dashboard|widget|side, board_id?, widget?, monitor, bounds}`.
- A window whose monitor is gone reopens on the primary.

## Review Focus

1. tccd not running when the app starts → app starts it (EnsureDaemon) instead of showing an empty window.
2. tccd restarts while the app is open → SSE reconnects and data reloads.
3. Saved window bounds on a monitor that no longer exists → window lands on the primary, fully visible.
4. Day changes at midnight with the widget open → habits and Today reload for the new day.
5. Two dashboard windows editing boards → each sees the other's saved board (ui.changed event).

---

### Task 1: tccd — CORS, event token, UI state

**Files:** `internal/api/server.go`, `internal/api/ui.go`, `internal/api/api_test.go`

- CORS: every response carries `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Headers: Authorization, Content-Type`, `Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS`; `OPTIONS` answers 204 without a token. (Bearer token, no cookies, so `*` grants nothing without the token.)
- `GET /api/events?token=<t>` is accepted in place of the header (EventSource can't set headers). Only on that path.
- `GET /api/ui/{key}` returns stored JSON (`null` when unset); `PUT /api/ui/{key}` stores a JSON body and publishes `ui.changed` with `Data: key`. Keys allowed: `boards`, `windows`; others 404.
- Tests: `TestCORSPreflight`, `TestEventsTokenQuery`, `TestUIState` (round trip, unknown key 404, invalid JSON 400).

### Task 2: frontend scaffold and logic

**Files:** `desktop/frontend/{package.json,vite.config.js,index.html,src/main.js,src/lib/*.js,src/lib/*.test.js}`

- `api.js`: `conn()` from `window.__TCC__` or `?base=&token=` (dev in a browser); `api(method, path, body)`; `listen(onEvent)` EventSource with reconnect and a reload callback on reconnect.
- `grid.js`: `snap(px, cell)`, `collides(a, b)`, `place(widgets, moved)` pushes overlapped widgets down, `compact(widgets)` floats widgets up. Tested.
- `fit.js`: `fitSections(heights, available)` → which sections show and which fold. Tested.
- `format.js`: `mmss(s)`, `habitValue(h)` ("1.8/3L", "112/180g", "1/2"), `dayLabel(date)`. Tested.

### Task 3: side widget

`#/side`: header (date, ⋯ menu, close), Timer, Habits, Today, Week, Month, folding as height allows. Everything editable in place.

### Task 4: dashboard

`#/board/<id>`: 12-column grid, edit mode (drag to move, corner to resize, snapping), add/remove widgets, board switcher (new/rename/delete), pop-out per widget. Widgets: today, week, month, habits, timer, project, inbox, inprogress.

### Task 5: project page and pop-outs

`#/project/<id>`: Inbox strip, Backlog / Up next / In progress lanes, Review lane, Recently done with evidence; tabs Board, Backlog, Ideas, Someday, Done. Ticket drawer to edit, move status, sign off Review → Done. `#/widget/<type>`: one widget filling the window.

### Task 6: Wails shell

**Files:** `desktop/main.go`, `desktop/shell.go`, `desktop/placement.go`, `desktop/placement_test.go`

- `placeOnScreen(saved Rect, monitor string, screens []Screen) (Rect, string)`: keep bounds if the monitor exists and the rect overlaps it, else centre on primary, clamped to its work area. Tested.
- `/shell/open`, `/shell/close`, `/shell/state` via asset middleware; window states saved (debounced) on move/resize/close to `/api/ui/windows`; all reopen at launch; default first run: one dashboard + side widget.
- Tray: Dashboard, New dashboard window, Side widget, Start/pause timer, Quit. Single instance. On `timer.phase`: Windows notification + flash.

### Task 7: install, docs, smoke test

`scripts/install.ps1` builds the frontend and `tcc-desktop.exe` into the Go bin folder and adds a Start Menu shortcut; README gains a Desktop section. Launch it and check the windows on screen.
