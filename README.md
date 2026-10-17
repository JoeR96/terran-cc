# Terran CC

Terran CC is Joe's command centre. It keeps project tickets, personal to-dos, daily habits and a Pomodoro timer in one place. You drive it from the terminal, from Claude Code and from the Stream Deck.

This repo holds the core: the `tccd` service and the `tcc` command. The desktop app and the Stream Deck plugin come next. The design is in `docs/superpowers/specs/2026-10-17-terran-cc-design.md`.

## Install

You need Go 1.24 or later.

```powershell
pwsh -File scripts/install.ps1
```

This builds `tcc`, `tccw` and `tccd` into your Go bin folder and installs the `terran-cc` Claude skill. Add `-DispatchTasks` to also replace your `dispatch-tasks` skill with the version that reads tickets from Terran CC. The script backs up the old one first.

You never start `tccd` yourself. The first `tcc` command starts it in the background. If you install a newer version, `tcc` replaces the old service by itself. `tcc service status`, `tcc service stop` and `tcc service restart` are there if you need them.

Your data lives in `%APPDATA%\terran-cc`: the database (`tcc.db`), the service log (`tccd.log`) and attachments. Set `TCC_HOME` to use a different folder.

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
go build -o bin/ ./cmd/...
```
