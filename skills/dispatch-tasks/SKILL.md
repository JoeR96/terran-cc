---
name: dispatch-tasks
description: Use when Joe asks to start, kick off, launch or dispatch tickets from Terran CC (Joe's command centre, e.g. "dispatch the up next tickets on bdtv-bonanza") as separate herdr agents, each with its own worktree, model and thinking level.
---

# Dispatch Tasks

## Overview

Each ticket becomes its own Claude agent in herdr: its own git worktree and herdr workspace, started with the model and effort from the ticket, and given a brief as its first prompt. Two-phase tickets ("Plan: X. Build: Y.") start on the plan model. When Joe approves the plan, the agent switches itself to the build model. Joe never changes models by hand.

The mechanics live in `%APPDATA%\herdr\dispatch-task.ps1`. This skill decides what to pass it. Tickets live in Terran CC; read them with `tcc` (see the `terran-cc` skill).

## Steps

1. Check herdr is up: `herdr status` and `herdr agent list`. If the server isn't running, stop and tell Joe.
2. Read the candidates: `tcc next --project <slug> --json`. These are the project's Up next tickets, in order. If Joe named ticket ids, read each with `tcc show <id> --json` instead. Ask which to start if Joe didn't say. Skip tickets that are done, in review or already in progress, and tell Joe about any whose `depends_on` tickets aren't done.
3. Read the project: `tcc project show <slug> --json`. Its `repo_path` is the repo to pass the script. If it's empty, ask Joe and save it with `tcc project edit <slug> --repo <path>`.
4. For each ticket, work out:
   - name: `<slug-prefix>-t<id>-<word>`, lowercase, max 32 chars, unique in `herdr agent list`
   - worktree path and branch: the ticket's `worktree` and `branch` if set; otherwise derive them as before and save them with `tcc edit <id> --branch … --worktree …`
   - start model and effort: from the ticket's `model` and `effort`. `model` may read `Plan: X high. Build: Y medium.`; that makes it two-phase, and the plan values are the start values
   - build model and effort: two-phase tickets only
   If `model` or `effort` is empty, ask Joe. Map names to ids: Fable 5.1 = `claude-fable-5-1`, Opus 5.5 = `claude-opus-5-5`, Sonnet 5.5 = `claude-sonnet-5-5`. Effort is low, medium, high, xhigh or max. "Fable or Opus" means Fable.
5. Write each brief from `brief-common.md` in this folder into `%APPDATA%\herdr\briefs\<name>.md`. Fill every `{{...}}`. `{{ID}}`, `{{TITLE}}` and `{{DETAILS}}` come from the ticket (details = its notes). `{{PHASE}}` is one of the two blocks under "Phase blocks" below. Add anything task-specific the agent needs (known causes, file names, ordering constraints).
6. Show Joe the plan (ticket, name, worktree, start model/effort, build model/effort) and wait for a go.
7. Run the script once per ticket, one at a time. Each run makes a worktree, which takes a while on a Unity repo.
   `pwsh -NoProfile -File "$env:APPDATA\herdr\dispatch-task.ps1" -Name <name> -Repo <repo_path> -Branch <branch> -Path <path> -Label "#<id> <short title>" -Model <id> -Effort <level> -BriefFile <brief>`
   Pass `-DryRun` first if anything is new.
8. After each agent starts, record it on the ticket:
   `tcc start <id> --branch <branch> --worktree <path> --session <name> --session-model <start model id>`
9. Check with `herdr agent list` that each agent shows up, then report the names to Joe.

## Phase blocks

Two-phase:

> This task has two phases. You start on the planning model.
> 1. Research the code and write the plan to `{{PLAN}}` in your worktree.
> 2. Tell Joe the plan is ready with a short summary, then stop. Don't write production code yet.
> 3. When Joe approves, run this exact command, then end your turn with one line saying you're switching to the build model:
>    `Start-Process pwsh -WindowStyle Hidden -ArgumentList '-NoProfile','-File',"$env:APPDATA\herdr\dispatch-task.ps1",'-Advance','-Name','{{NAME}}','-Model','{{BUILD_MODEL}}','-Effort','{{BUILD_EFFORT}}'`
>    It restarts this same conversation on the build model and tells you to build. If Joe asks for changes instead, update the plan and ask again.
> 4. Build from the plan.

Single-phase:

> Work out the approach, tell Joe in a few lines what you're going to do, then do it. Stop and ask only if the task turns out bigger than its ticket says.

## Common mistakes

- Typing `/model` into a running agent. It also saves that model as Joe's global default. The script's `-Advance` exits and resumes with `--model` and `--effort` flags instead.
- Passing a bare `--` through a PowerShell function: PowerShell drops it. The script quotes it.
- Reusing a worktree path that exists. The script refuses; pick a new path or ask Joe.
- Launching all tickets in one command. Run them one at a time so a failure stops at one ticket.
- Running `tcc done` for an agent. Agents move tickets to Review; Joe marks them Done.
