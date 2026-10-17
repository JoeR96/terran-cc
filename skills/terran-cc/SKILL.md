---
name: terran-cc
description: Use when working on a Terran CC ticket or to-do, when Joe mentions a ticket id like #12 or "my tickets/to-do list/habits", when you start or finish work that came from Terran CC, or when you find a new bug or task worth tracking while working on one of Joe's projects.
---

# Terran CC

Terran CC is Joe's command centre: project tickets, personal to-dos, daily habits and a Pomodoro timer. You use it through the `tcc` command. `tcc` starts its service (`tccd`) by itself when needed.

Add `--json` to any command when you need to read the result. Errors then come back as `{"error": "..."}`. Exit code 2 means "not found".

## Before you start a ticket

1. Read it: `tcc show <id>`. Check its notes, depends-on and evidence.
2. If it depends on items that aren't done, tell Joe before you start.
3. Mark it started and record where the work happens:
   `tcc start <id> --branch <branch> --worktree <path> --session <session or herdr agent name> --session-model <model id>`

## While you work

- Record decisions and findings on the ticket: `tcc note <id> "<one or two lines>"`.
- Found something new that needs doing? File it in the project's Inbox, not the backlog:
  `tcc add "<title>" --project <slug> --kind bug|task|feature --inbox --notes "<what you saw>"`.
  Joe triages the Inbox.
- Don't change other tickets' status, priority or order unless Joe asks.

## When you finish

1. Attach the evidence. Use one flag per item, and repeat flags as needed:
   `tcc evidence <id> --commit <sha> --pr <number or url> --test "<command> → <result>" --screenshot <path>`
2. Upload screenshots you want Joe to see: `tcc attach <id> <file>`.
3. Move the ticket to Review: `tcc review <id>`.
4. Tell Joe it's in Review, with a one-line summary.

**Never run `tcc done`.** Joe moves tickets from Review to Done after checking the evidence.

## Other things Joe may ask for

| Ask | Command |
|-----|---------|
| What's on today? | `tcc status` |
| Add a to-do | `tcc todo "<title>"` (add `--when week`, `--context work`) |
| What's next on a project? | `tcc next --project <slug>` |
| List a project's tickets | `tcc ls --project <slug>` (add `--all` for done ones) |
| Log a habit | `tcc habit log water +250`, `tcc habit log walks`, `tcc habit log creatine toggle` |
| Start a focus session | `tcc timer start --item <id>` |
| Projects | `tcc project ls`, `tcc project show <slug>` |

Statuses: `inbox → backlog → up_next → in_progress → review → done`, plus `someday`. Kinds: `todo`, `bug`, `task`, `feature`, `idea`.
