# OSRS in Terran CC: design

Date: 2026-10-17. Status: agreed with Joe in chat, piece by piece.

## Why

Joe plays an ironman, Big Chopski. His OSRS to-dos were spread across Obsidian notes. Three questions take him into the game, or into RuneLite's side panel, just to answer:

- Which runs are ready?
- Am I running low on supplies?
- How far off are my targets?

Terran CC should answer all three on the side widget, kept up to date with no typing.

## Goals

- **Daily runs.** Each run shows a live state from RuneLite: ready, a countdown, or ✓ done today. It ticks itself when Joe plants or seeds, and sends a Windows notification when it becomes ready. The runs are:
  - bird houses
  - herb run
  - potato cacti
  - whiteberries
  - Hespori
  - farming contract
  - tree run: regular, fruit, hardwood and calquat
- **Stock.** Each tracked item shows have / sufficient / target, read from the bank export. Colours: red below sufficient, amber between sufficient and target, green at or over target. Potions count in doses, and raw food shows beside cooked without counting towards it. The default list is the one agreed in chat. Sufficient defaults to 5% of the target, and every number is editable.
- **AFK methods.** A fixed list of Joe's fallback activities, shown with the stock: bloodwood sap, anglerfish, amethyst, shark lures.
- **Skill targets.** Each target records start XP when it is set, current XP from the hiscores, and the goal level. They are derived from the Base 90s goal plus any extra target.
- **Progress window.** A window with OSRS Wiki icons and start / current / finish bars for runs, skills, stock and gear goals.
  - It opens from a status strip on the OSRS heading: one coloured tick per run and per stock item.
  - It is also available as a dashboard widget (`osrs`).
- **Always current.** It is fed by file watching, plus a clock tick for ready times. Every change reaches every window over the existing SSE stream.

## Non-goals

- No RuneLite plugin of our own. We read what RuneLite already writes.
- No Grand Exchange or prices.
- No support for several accounts in the UI. The account is a setting, default `big chopski`.
- No XP tracking inside a session. The hiscores lag until logout or a world hop, and that is accepted.

## Sources (all local, read-only)

- **Timers.** `%USERPROFILE%\.runelite\profiles2\$rsprofile--1.properties`, under the keys `timetracking.rsprofile.<key>.*`.
  - `<key>` is found from `rsprofile.rsprofile.<key>.displayName`. When the name has two keys, use the one whose time-tracking stamps are newest.
  - `<region>.<varbit>=<value>:<unix>` describes a farming patch.
  - `birdhouse.<varp>=<value>:<unix>` describes a bird house. It is seeded when the value is a multiple of 3 and above 0, and ready 50 minutes after the stamp.
  - `contract=<produce contract value>` is the farming contract.
  - `farmTickOffset` and `farmTickOffsetPrecision` hold the farm-tick offset.
  - `autoweed` is the autoweed setting.
  - Java properties escape `:` as `\:`.
- **Bank.** `%USERPROFILE%\.runelite\Data Exports\container_bank.json`, written by the Data Export plugin. It is JSON lines, one `{id, quantity, name}` per line.
- **XP.** `https://secure.runescape.com/m=hiscore_oldschool/index_lite.json?player=<name>`, polled every 10 minutes with User-Agent `terran-cc/<version>`.

## Timer rules (ported from RuneLite, BSD-2, notice kept)

- **Patch state.** `PatchImplementation.forVarbitValue(value)` gives produce, crop state (growing, harvestable, diseased, dead, filling) and stage.
- **Produce.** Each produce has a tickrate in minutes, stages, a regrow tickrate and harvest stages.
- **Tick time.** `getTickTime(rate, ticks, t)` aligns `t` plus the offset to a rate boundary, adds the ticks, then subtracts the offset. The offset applies when precision ≥ the rate or ≥ 40: `(offset % rate) * 60` seconds.
- **Done estimate.** `getTickTime(rate, stages-1-stage, tickTime(t))`.
- **Patch list.** `FarmingWorld` lists regions and patches. The table converts to Go with a script kept next to it.
- **Which patches belong to which run:**
  - Herb run: HERB patches.
  - Whiteberries: BUSH patches.
  - Potato cacti: CACTUS patches.
  - Hespori: the HESPORI patch.
  - Tree run: TREE, FRUIT_TREE, HARDWOOD_TREE and CALQUAT patches.
  - Contract: the Farming Guild patch whose type matches the contract produce.
- **Run state:**
  - Ready when no patch in the run is still growing and at least one is harvestable or empty.
  - Otherwise, ready_at is the latest done estimate.
  - Unknown when no patch has ever been seen.
- **Done today.** At least one patch in the run was planted on or after local midnight. Planted time is the stamp minus the stages already grown × the tickrate. Bird houses count as done today when they were seeded today.

## API (tccd)

- `GET /api/osrs` returns `{account, updated: {timers, bank, skills}, runs[], stock[], afk[], skills[], warnings[]}`.
- `PUT /api/osrs/config` changes the account, the stock items (key, label, names or dose patterns, sufficient, target), the AFK list and the skill targets. It is stored in the setting `osrs.config`.
- SSE `osrs.changed` fires when any source changes. `osrs.ready` fires, with the run key, when a run becomes ready.
- `GET /api/osrs/icon/{name}` fetches an OSRS Wiki image once and caches it under `TCC_HOME/icons`. Icon names come from the wiki's file names.

## UI

- **The OSRS group** in the to-do tree gains two sections:
  - **Daily runs:** one row per run, with its icon, state chip, ✓ and countdown.
  - **Stock:** one row per item, with its icon, have / sufficient / target and a coloured mini bar.
  - Both sit above the goal sections.
- **Status strip.** The OSRS heading gets a row of ticks, one per run and stock item, coloured by state. Clicking the strip opens the progress window as a pop-out, widget type `osrs`.
- **Progress window / widget `osrs`:**
  - runs as cards
  - skill bars: grey for the start, orange for the gain, the level and XP to go
  - stock bars with a sufficient tick
  - gear goals, from OSRS to-dos with sub to-dos: pieces done out of pieces total
- **Desktop shell.** On `osrs.ready` it shows a Windows notification such as "Herb run ready".

## Testing

- Go unit tests for:
  - the properties parser, including `\:` escapes and picking the profile key
  - the bank parser, including dose sums and raw versus cooked
  - tick time and done estimates, against values worked out by hand from RuneLite's formula
  - spot checks of patch states taken from RuneLite's tables
  - run aggregation and done-today
  - stock colours
- An API test with fixture files in a temp RuneLite dir.
- Frontend logic tests for the strip and bar maths.

## Left to the builder

- Icon file names per item.
- The exact poll interval: 2 s for the file check, 30 s for the clock tick.
- Layout details, within the mockups shown in chat.
