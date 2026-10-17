<script>
  import { db, saveBoards, defaultBoards, act, projectOf } from '../lib/store.svelte.js'
  import { shell } from '../lib/api.js'
  import { snap, place, compact, firstFree } from '../lib/grid.js'
  import { go } from '../lib/route.svelte.js'
  import WidgetBody from '../components/WidgetBody.svelte'
  import ItemDrawer from '../components/ItemDrawer.svelte'
  import Chime from '../components/Chime.svelte'

  let { boardId } = $props()

  const COLS = 12
  const ROW = 56
  const GAP = 8
  const TYPES = [
    { type: 'today', name: 'Today', w: 4, h: 5 },
    { type: 'week', name: 'This week', w: 4, h: 4 },
    { type: 'month', name: 'This month', w: 4, h: 4 },
    { type: 'anytime', name: 'Anytime', w: 4, h: 4 },
    { type: 'habits', name: 'Habits', w: 3, h: 3 },
    { type: 'timer', name: 'Timer', w: 3, h: 3 },
    { type: 'inprogress', name: 'In progress', w: 5, h: 4 },
    { type: 'inbox', name: 'Inbox', w: 4, h: 3 },
    { type: 'projects', name: 'Projects', w: 4, h: 4 },
  ]
  const title = w =>
    w.type === 'project' ? projectOf(w.project_id)?.name || 'Project' : TYPES.find(t => t.type === w.type)?.name || w.type

  const boards = $derived(db.boards.length ? db.boards : defaultBoards())
  const board = $derived(boards.find(b => b.id === boardId) || boards[0])

  let width = $state(0)
  const cellW = $derived(width / COLS)
  let editing = $state(false)
  let adding = $state(false)
  let preview = $state(null) // widgets while dragging
  let drawer = $state(null)
  let boardMenu = $state(false)
  let newProject = $state(null)

  const shown = $derived(preview || board.widgets)
  const rows = $derived(Math.max(8, ...shown.map(w => w.y + w.h)) + (editing ? 4 : 0))

  function save(widgets) {
    saveBoards(boards.map(b => (b.id === board.id ? { ...b, widgets } : b)))
  }

  function drag(e, w, mode) {
    if (!editing) return
    e.preventDefault()
    const sx = e.clientX, sy = e.clientY
    const el = e.currentTarget
    el.setPointerCapture(e.pointerId)
    const move = ev => {
      const dx = ev.clientX - sx, dy = ev.clientY - sy
      const next =
        mode === 'move'
          ? { ...w, x: snap(w.x * cellW + dx, cellW), y: snap(w.y * ROW + dy, ROW) }
          : { ...w, w: Math.max(2, snap(w.w * cellW + dx, cellW)), h: Math.max(2, snap(w.h * ROW + dy, ROW)) }
      preview = place(board.widgets, next, COLS)
    }
    const up = () => {
      el.removeEventListener('pointermove', move)
      el.removeEventListener('pointerup', up)
      if (preview) save(compact(preview))
      preview = null
    }
    el.addEventListener('pointermove', move)
    el.addEventListener('pointerup', up)
  }

  function addWidget(t, project_id) {
    adding = false
    const pos = firstFree(board.widgets, t.w, t.h, COLS)
    const w = { id: 'w' + Date.now().toString(36), type: t.type, ...pos, w: t.w, h: t.h, ...(project_id && { project_id }) }
    save([...board.widgets, w])
  }
  const removeWidget = w => save(compact(board.widgets.filter(x => x.id !== w.id)))
  const popOut = w => shell('open', { kind: 'widget', widget: { type: w.type, project_id: w.project_id } })

  function switchBoard(id) {
    boardMenu = false
    go(`/board/${id}`)
    shell('state', { board_id: id })
  }
  function newBoard() {
    const name = prompt('Board name', `Monitor ${boards.length + 1}`)
    if (!name) return
    const id = 'b' + Date.now().toString(36)
    saveBoards([...boards, { id, name, widgets: [] }])
    switchBoard(id)
    editing = true
  }
  function renameBoard() {
    const name = prompt('Rename board', board.name)
    if (name) saveBoards(boards.map(b => (b.id === board.id ? { ...b, name } : b)))
    boardMenu = false
  }
  function deleteBoard() {
    if (boards.length < 2 || !confirm(`Delete board "${board.name}"?`)) return
    const rest = boards.filter(b => b.id !== board.id)
    saveBoards(rest)
    switchBoard(rest[0].id)
  }
  async function createProject() {
    const name = newProject?.trim()
    newProject = null
    if (!name) return
    const p = await act.addProject({ name })
    if (p) go(`/project/${p.id}`)
  }
</script>

<Chime />
<div class="page">
  <header>
    <div class="left">
      <span class="brand">Terran CC</span>
      <div class="bsel">
        <button onclick={() => (boardMenu = !boardMenu)}>{board.name} ▾</button>
        {#if boardMenu}
          <div class="pop">
            {#each boards as b (b.id)}
              <button class="ghost" class:cur={b.id === board.id} onclick={() => switchBoard(b.id)}>{b.name}</button>
            {/each}
            <hr />
            <button class="ghost" onclick={newBoard}>+ New board</button>
            <button class="ghost" onclick={renameBoard}>Rename “{board.name}”</button>
            {#if boards.length > 1}<button class="ghost danger" onclick={deleteBoard}>Delete “{board.name}”</button>{/if}
          </div>
        {/if}
      </div>
      {#each db.projects.filter(p => !p.archived) as p (p.id)}
        <button class="ghost" onclick={() => go(`/project/${p.id}`)}>{p.name}</button>
      {/each}
      {#if newProject !== null}
        <input placeholder="Project name" bind:value={newProject} onkeydown={e => e.key === 'Enter' && createProject()} onblur={createProject} use:focusEl />
      {:else}
        <button class="ghost" onclick={() => (newProject = '')}>+ Project</button>
      {/if}
    </div>
    <div class="right">
      {#if editing}
        <div class="bsel">
          <button onclick={() => (adding = !adding)}>+ Widget</button>
          {#if adding}
            <div class="pop rightpop">
              {#each TYPES as t (t.type)}
                <button class="ghost" onclick={() => addWidget(t)}>{t.name}</button>
              {/each}
              {#if db.projects.length}<hr />{/if}
              {#each db.projects as p (p.id)}
                <button class="ghost" onclick={() => addWidget({ type: 'project', w: 4, h: 4 }, p.id)}>Project: {p.name}</button>
              {/each}
            </div>
          {/if}
        </div>
        <button class="primary" onclick={() => { editing = false; adding = false }}>Done</button>
      {:else}
        <button class="ghost" onclick={() => (editing = true)}>Edit layout</button>
      {/if}
      <button class="ghost" title="Open the side widget" onclick={() => shell('open', { kind: 'side' })}>Side widget</button>
    </div>
  </header>

  <main bind:clientWidth={width}>
    <div class="grid" class:editing style="height:{rows * ROW}px; --cw:{cellW}px; --rh:{ROW}px">
      {#each shown as w (w.id)}
        <section
          class="w"
          class:dragging={preview}
          style="left:{w.x * cellW + GAP / 2}px; top:{w.y * ROW + GAP / 2}px; width:{w.w * cellW - GAP}px; height:{w.h * ROW - GAP}px"
        >
          <h6 class:grab={editing} onpointerdown={e => drag(e, w, 'move')}>
            <span>{title(w)}</span>
            <span class="wbtn">
              {#if editing}
                <button class="ghost" title="Remove" onpointerdown={e => e.stopPropagation()} onclick={() => removeWidget(w)}>✕</button>
              {:else}
                <button class="ghost" title="Pop out" onclick={() => popOut(w)}>⤢</button>
              {/if}
            </span>
          </h6>
          <div class="wb" class:inert={editing}>
            <WidgetBody widget={w} onproject={id => go(`/project/${id}`)} onitem={it => (drawer = it.id)} />
          </div>
          {#if editing}<div class="rs" title="Resize" onpointerdown={e => drag(e, w, 'resize')}></div>{/if}
        </section>
      {/each}
      {#if !shown.length}
        <div class="empty faint">Empty board. Click “+ Widget” to add one.</div>
      {/if}
    </div>
  </main>
</div>

{#if drawer}
  <ItemDrawer id={drawer} onclose={() => (drawer = null)} />
{/if}

<script module>
  export const focusEl = el => el.focus()
</script>

<style>
  .page { height: 100vh; display: flex; flex-direction: column; }
  header { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 6px 10px; border-bottom: 1px solid var(--line); background: var(--panel); flex: none; }
  .left, .right { display: flex; align-items: center; gap: 4px; min-width: 0; }
  .left { overflow: hidden; }
  .brand { font-weight: 700; color: var(--accent); margin-right: 6px; letter-spacing: .02em; white-space: nowrap; }
  .bsel { position: relative; }
  .pop { position: absolute; top: 100%; left: 0; z-index: 20; margin-top: 4px; min-width: 180px; display: flex; flex-direction: column; background: var(--panel-2); border: 1px solid var(--line-2); border-radius: 6px; padding: 4px; box-shadow: 0 8px 24px rgba(0, 0, 0, .5); max-height: 70vh; overflow: auto; }
  .rightpop { left: auto; right: 0; }
  .pop button { text-align: left; color: var(--text); }
  .pop .cur { color: var(--accent); }
  .pop .danger { color: var(--red); }
  .pop hr { border: 0; border-top: 1px solid var(--line); margin: 4px 0; width: 100%; }
  main { flex: 1; overflow: auto; padding: 4px; }
  .grid { position: relative; }
  .grid.editing { background-image: linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px); background-size: var(--cw) var(--rh); background-position: -1px -1px; }
  .w { position: absolute; background: var(--panel-2); border: 1px solid var(--line-2); border-radius: 8px; padding: 8px 10px; display: flex; flex-direction: column; transition: left .12s, top .12s, width .12s, height .12s; }
  .w.dragging { transition: none; }
  .w h6 { flex: none; }
  .grab { cursor: grab; }
  .wbtn button { padding: 0 4px; font-size: 11px; }
  .wb { flex: 1; min-height: 0; overflow: auto; }
  .wb.inert { pointer-events: none; opacity: .75; }
  .rs { position: absolute; right: 0; bottom: 0; width: 14px; height: 14px; cursor: nwse-resize; background: linear-gradient(135deg, transparent 50%, var(--muted) 50%); border-bottom-right-radius: 8px; }
  .empty { position: absolute; top: 40px; width: 100%; text-align: center; }
  header input { width: 150px; }
</style>
