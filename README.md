# Terran CC

Terran CC is Joe's command centre. It keeps project tickets, personal to-dos, daily habits and a Pomodoro timer in one place. You drive it from the terminal, from Claude Code and from the Stream Deck.

This repo holds the `tccd` service, the `tcc` command and the desktop app. The Stream Deck plugin comes next. The design is in `docs/superpowers/specs/2026-10-17-terran-cc-design.md`.

## Install

You need Go 1.25 or later and Node 24.

```powershell
pwsh -File scripts/install.ps1
```

This builds `tcc`, `tccw`, `tccd` and `tcc-desktop` into your Go bin folder, adds Terran CC to the Start Menu and to sign-in startup, and installs the `terran-cc` Claude skill. Add `-DispatchTasks` to also replace your `dispatch-tasks` skill with the version that reads tickets from Terran CC. The script backs up the old one first.

You never start `tccd` yourself. The first `tcc` command starts it in the background. If you install a newer version, `tcc` replaces the old service by itself. `tcc service status`, `tcc service stop` and `tcc service restart` are there if you need them.

Your data lives in `%APPDATA%\terran-cc`: the database (`tcc.db`), the service log (`tccd.log`) and attachments. Set `TCC_HOME` to use a different folder.

## Desktop app

Open **Terran CC** from the Start Menu (or run `tcc-desktop`). It starts the service if needed and puts an icon in the tray. Closing every window keeps it running in the tray; use the tray menu to bring windows back or to quit.

- **Dashboard**: a snap grid of widgets. Click **Edit layout** to drag widgets by their header, resize them from the corner, add or remove them. Use the board menu (top left) to make one board per monitor. Each dashboard window remembers its board, monitor and size.
- **Side widget**: the always-on-top strip. Timer on top (pick a time, or 🔒 to lock in on a task), then habits. Below that:
  - **To-dos**: a tree of Me, Work and OSRS by section, with sub to-dos. Drag to reorder or move between sections.
  - **Goals**: project tickets set for today, this week or this month.
  - **All projects**: a project › feature › ticket tree. Drag to reorder projects or refile tickets.
  - **Assistant**: a chat box.

  Made wider than 560 px, the habits run across the top and the rest splits into two columns.
- **Assistant**: a chat box with a model in the loop, using OpenCode Go. Tell it things like "idea for bonanza: exploding barrels" or "osrs short term: prep for raids s4". It changes to-dos, tickets, habits and the timer through the same API as the app, and lists what it did.
  - The key is read from opencode's `auth.json`, so connect OpenCode Go in opencode first. You can also set `TCC_OPENCODE_KEY`.
  - The model is `glm-5.3-flash`. Change it with `TCC_ASSIST_MODEL`.
- **Pop-outs**: the ⤢ on any widget opens it in its own small always-on-top window. 📌 toggles always-on-top.
- **Project pages**: click a project in the top bar. Drag cards between lanes; drop a card on another in Up next to reorder. Click a card to edit it, paste a screenshot to attach it, and sign off Review items.
- **Ctrl + mouse wheel** zooms a window, handy on 4K screens.

Windows reopen where you left them. A window whose monitor is gone opens on the primary.

The frontend can also run in a browser for development: `cd desktop/frontend; npm run dev`, then open `http://localhost:5173/?token=<token from daemon.json>`.

## Everyday commands

```text
tcc status                          today: habits, timer, to-dos, work in progress
tcc todo "Book barber"              personal to-do for today
tcc todo "Renew insurance" --when month
tcc add "Fix bot melee" --project bdtv-bonanza --kind bug
tcc add "Stairs look odd" --project bdtv-bonanza --inbox
tcc ls --project bdtv-bonanza       open tickets (add --all for done ones)
tcc next --project bdtv-bonanza     Up next, in order
tcc show 12                         everything about #12
tcc edit 12 --status up_next --position 1 --why-first "biggest change"
tcc done 12

tcc habit                           today's habits and the streak
tcc habit log water                 add one step (250 ml)
tcc habit log protein +40
tcc habit log walks
tcc habit log creatine toggle

tcc timer start --item 12           focus on #12
tcc timer toggle                    start, pause or resume
tcc timer config --focus 50
```

Add `--json` to any command for machine-readable output.

## Projects

```text
tcc project add "BDTV Bonanza" --repo C:\Users\Administrator\TheExecutioner-Remake-Unity
tcc project ls
tcc project show bdtv-bonanza
```

A ticket moves through `inbox → backlog → up_next → in_progress → review → done`. `someday` holds parked work. Claude moves its finished tickets to Review. You check the evidence and mark them Done.

## Stream Deck buttons

Use the Stream Deck's built-in "Open" (System) action and point it at `tccw.exe` in your Go bin folder, with these arguments. `tccw` is the same as `tcc` but runs without a console window, so nothing flashes on screen when you press a button.

| Button | Arguments |
|--------|-----------|
| Start / pause the timer | `timer toggle` |
| Skip to the next phase | `timer skip` |
| Water +250 ml | `habit log water` |
| Walk done | `habit log walks` |
| Supplements taken | `habit log creatine toggle` |

A plugin with a live countdown key is planned.

## Repo layout

```text
cmd/tccd          the service
cmd/tcc           the command line
desktop/          the desktop app (Wails shell + Svelte frontend in desktop/frontend)
internal/core     the rules (every client goes through these)
internal/store    storage interface; sqlite/ implements it
internal/api      HTTP JSON API and the live event stream
internal/timer    Pomodoro state machine
internal/client   Go client used by tcc
internal/cli      tcc commands
skills/           Claude Code skills
scripts/          install script
```

## Development

```powershell
go test ./...
go -C desktop test ./...
cd desktop/frontend; npm test
```
