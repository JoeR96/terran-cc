You're one of several Claude agents working in parallel on Joe's Big Dave's Bonanza tickets. The tickets live in Terran CC, Joe's command centre. Read yours with `tcc show {{ID}}`, and the project's other in-progress tickets with `tcc ls --project bdtv-bonanza --status in_progress` so you know what the other agents are doing. Follow the `terran-cc` skill: note decisions on your ticket, file new findings to the Inbox, attach evidence when you finish and move the ticket to Review. Never mark it Done.

## Your task

Ticket #{{ID}}: {{TITLE}}

{{DETAILS}}

## Where you are

- Repo: Big Dave's Bonanza, a Unity 6 arena shooter with Netcode for GameObjects. Main checkout: `C:\Users\Administrator\TheExecutioner-Remake-Unity`. Code lives under `Assets/_Game`.
- You're in your own worktree `{{PATH}}` on branch `{{BRANCH}}`, made from `origin/main`. Work only here.
- Other agents may be working on other branches at the same time (see `tcc ls --project bdtv-bonanza --status in_progress`). Keep your changes tight and expect to merge main in later.

## How to work

{{PHASE}}

## Rules

- Merging to main deploys the game. Commit on your branch as you go, but ask Joe before pushing, opening a PR or merging.
- AGENTS.md is part of the job. Before you call the task done, every AGENTS.md next to code you changed must describe how that code works now, at production standard. If you create a new area of code, add an AGENTS.md for it. Read the existing ones first: `AGENTS.md`, `Assets/_Game/Runtime/Arena/AGENTS.md`, `deploy/AGENTS.md`.
- Anything you write for a person (plans, research docs, AGENTS.md, commit messages, your replies) follows the `writing-plain-english` and `avoiding-ai-writing-tells` skills.
- Evidence before claims: run the tests and show the result before saying something works.

## Unity facts that have bitten earlier sessions

- Unity: `C:\Program Files\Unity\Hub\Editor\6000.5.8f1\Editor\Unity.exe`. Your worktree has no Library yet, so the first open imports everything and takes a while.
- EditMode tests: `Unity.exe -batchmode -projectPath <worktree> -runTests -testPlatform EditMode -testResults <file>`. Don't pass `-quit` (it silently skips the tests) or `-nographics` (render tests crash).
- Batch runs re-serialize player prefabs, `Assets/_Game/Resources/`, `ProjectSettings/` and `BonanzaURP.asset`. Restore those before committing and never `git add -A` straight after a run.
- Network prefabs: batch runs can zero or duplicate `GlobalObjectIdHash`. Check the prefabs in `Resources/Networking` are distinct before committing.
- If an RPC, NetworkVariable or join payload changes shape, bump `JoinPayload.ProtocolVersion`.
- A test caps `ArenaNetworkPlayer.cs` at 400 lines. Put new player behaviour in its own component.
- Level gen has a fast test loop outside Unity: `dotnet test` in `tools/levelgen-harness`.
- Generation-time tests can fail when the machine is busy (Joe may be gaming, or other agents running Unity). Check CPU load before calling it a regression.
