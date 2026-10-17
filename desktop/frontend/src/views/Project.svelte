<script>
  import { db, act, projectOf, itemTitle } from '../lib/store.svelte.js'
  import { go } from '../lib/route.svelte.js'
  import { shell } from '../lib/api.js'
  import ItemDrawer from '../components/ItemDrawer.svelte'
  import Chime from '../components/Chime.svelte'

  let { projectId, boardId = 'main' } = $props()

  const p = $derived(projectOf(projectId))
  const mine = $derived(db.items.filter(i => i.project_id === projectId))
  const by = st => mine.filter(i => i.status === st)
  const inbox = $derived(by('inbox'))
  const backlog = $derived(by('backlog').filter(i => i.kind !== 'idea').sort((a, b) => b.priority - a.priority || a.id - b.id))
  const ideas = $derived(mine.filter(i => i.kind === 'idea' && i.status !== 'done'))
  const upNext = $derived(by('up_next').sort((a, b) => a.position - b.position || a.id - b.id))
  const inProgress = $derived(by('in_progress'))
  const review = $derived(by('review'))
  const someday = $derived(by('someday'))
  const done = $derived(by('done').sort((a, b) => (b.done_at || '').localeCompare(a.done_at || '')))
  const subCount = id => mine.filter(i => i.parent_id === id).length
  const evShort = it =>
    (it.evidence || [])
      .filter(e => e.type === 'commit' || e.type === 'pr')
      .map(e => (e.type === 'pr' ? 'PR ' + e.value.replace(/^PR\s*/i, '') : e.value.slice(0, 7)))
      .join(' · ')

  let tab = $state('board')
  let triage = $state(false)
  let drawer = $state(null)
  let capture = $state('')
  let addLane = $state(null)
  let addDraft = $state('')
  let dragId = $state(null)
  let editDesc = $state(false)

  async function quickCapture() {
    const t = capture.trim()
    if (!t) return
    capture = ''
    await act.addItem({ title: t, project_id: projectId, status: 'inbox', kind: 'task' })
  }
  async function addTo(status) {
    const t = addDraft.trim()
    addDraft = ''
    addLane = null
    if (!t) return
    const n = { title: t, project_id: projectId, status, kind: status === 'idea' ? 'idea' : 'task' }
    if (status === 'idea') n.status = 'backlog'
    if (status === 'up_next') n.position = upNext.length + 1
    await act.addItem(n)
  }
  function drop(status) {
    const id = dragId
    dragId = null
    if (!id) return
    const it = mine.find(i => i.id === id)
    if (!it || it.status === status) return
    if (status === 'done') return act.done(id)
    const patch = { status }
    if (status === 'up_next') patch.position = upNext.length + 1
    act.edit(id, patch)
  }
  // Reorder Up next: drop a card onto another card to take its place.
  function dropOn(target) {
    const id = dragId
    if (!id || target.status !== 'up_next' || id === target.id) return drop(target.status)
    dragId = null
    const order = upNext.filter(i => i.id !== id)
    const at = order.findIndex(i => i.id === target.id)
    const dragged = mine.find(i => i.id === id)
    order.splice(at, 0, dragged)
    order.forEach((it, i) => {
      if (it.position !== i + 1 || it.status !== 'up_next') act.edit(it.id, { status: 'up_next', position: i + 1 })
    })
  }
  const TABS = $derived([
    ['board', 'Board'], ['backlog', `Backlog (${backlog.length})`], ['ideas', `Ideas (${ideas.length})`],
    ['someday', `Someday (${someday.length})`], ['done', `Done (${done.length})`],
  ])
</script>

{#snippet card(it, showPos = 0)}
  <button
    class="t"
    draggable="true"
    ondragstart={() => (dragId = it.id)}
    ondragover={e => e.preventDefault()}
    ondrop={e => { e.stopPropagation(); dropOn(it) }}
    onclick={() => (drawer = it.id)}
  >
    <div>
      {#if showPos}<span class="pos">{showPos}.</span>{/if}
      {#if it.kind !== 'todo' && it.kind !== 'task' || !showPos}<span class="tag {it.kind}">{it.kind}</span>{/if}{it.title}
    </div>
    {#if it.why_first}<div class="meta">Why first: {it.why_first}</div>{/if}
    {#if (it.depends_on || []).length}
      <div class="meta dep">⛓ depends on {it.depends_on.map(d => '#' + d + (itemTitle(d) ? ' ' + itemTitle(d) : '')).join(', ')}</div>
    {/if}
    {#if subCount(it.id)}<div class="meta">{subCount(it.id)} sub-tasks</div>{/if}
    {#if (it.attachments || []).length}<div class="meta">📎 {it.attachments.length}</div>{/if}
    {#if it.branch}<div class="meta">🌿 {it.branch}{it.worktree ? ' · ' + it.worktree : ''}</div>{/if}
    {#if it.session}<div class="meta claude">◆ {it.session}{it.session_model ? ' · ' + it.session_model : ''}</div>{/if}
    {#if evShort(it)}<div class="meta"><code>{evShort(it)}</code></div>{/if}
  </button>
{/snippet}

{#snippet adder(status, label = '+ add')}
  {#if addLane === status}
    <input class="adder" placeholder="Title, Enter to add" bind:value={addDraft} onkeydown={e => { if (e.key === 'Enter') addTo(status); if (e.key === 'Escape') addLane = null }} onblur={() => addTo(status)} use:focusEl />
  {:else}
    <button class="ghost addbtn" onclick={() => { addLane = status; addDraft = '' }}>{label}</button>
  {/if}
{/snippet}

{#snippet lane(title, items, status, hint = '', numbered = false)}
  <div class="lane" role="list" ondragover={e => e.preventDefault()} ondrop={() => drop(status)}>
    <h6>{title} <span>{hint || items.length}</span></h6>
    {#each items as it, i (it.id)}{@render card(it, numbered ? i + 1 : 0)}{/each}
    {@render adder(status)}
  </div>
{/snippet}

<Chime />
<div class="page">
  <header>
    <div class="left">
      <button class="ghost" onclick={() => go(`/board/${boardId}`)}>← Board</button>
      <h4>{p?.name || 'Project'}</h4>
      {#if p}<button class="ghost" title="Pop this project's summary out" onclick={() => shell('open', { kind: 'widget', widget: { type: 'project', project_id: p.id } })}>⤢</button>{/if}
    </div>
    <nav>
      {#each TABS as [k, n] (k)}
        <button class="ghost" class:on={tab === k} onclick={() => (tab = k)}>{n}</button>
      {/each}
    </nav>
  </header>

  {#if !p}
    <div class="faint pad">This project doesn't exist.</div>
  {:else}
    <div class="scroll">
      {#if editDesc}
        <input class="desc" value={p.description} placeholder="Short description" onchange={e => act.editProject(p.id, { description: e.currentTarget.value })} onblur={() => (editDesc = false)} use:focusEl />
      {:else}
        <button class="ghost desc" onclick={() => (editDesc = true)}>{p.description || 'Add a short description'}{p.repo_path ? ' · ' + p.repo_path : ''}</button>
      {/if}

      {#if tab === 'board'}
        <div class="inbox" role="list" ondragover={e => e.preventDefault()} ondrop={() => drop('inbox')}>
          <div class="ibar">
            <span>📥 Inbox — {inbox.length ? `${inbox.length} rough capture${inbox.length === 1 ? '' : 's'} waiting` : 'empty'}</span>
            <input placeholder="Capture something… (Enter)" bind:value={capture} onkeydown={e => e.key === 'Enter' && quickCapture()} />
            {#if inbox.length}<button class="ghost" onclick={() => (triage = !triage)}>{triage ? 'Close' : 'Triage ▸'}</button>{/if}
          </div>
          {#if triage}
            {#each inbox as it (it.id)}
              <div class="tri">
                <button class="ghost ttl" onclick={() => (drawer = it.id)}>{it.title}</button>
                <select value={it.kind} onchange={e => act.edit(it.id, { kind: e.currentTarget.value })}>
                  {#each ['bug', 'task', 'feature', 'idea'] as k (k)}<option>{k}</option>{/each}
                </select>
                <button onclick={() => act.edit(it.id, { status: 'backlog' })}>→ Backlog</button>
                <button onclick={() => act.edit(it.id, { status: 'up_next', position: upNext.length + 1 })}>→ Up next</button>
                <button onclick={() => act.edit(it.id, { status: 'someday' })}>Someday</button>
                <button class="ghost" title="Delete" onclick={() => act.remove(it.id)}>✕</button>
              </div>
            {/each}
          {/if}
        </div>

        <div class="lanes">
          {@render lane('Backlog', backlog.slice(0, 12), 'backlog', 'by priority')}
          {@render lane('Up next', upNext, 'up_next', 'ordered', true)}
          <div class="col">
            {@render lane('In progress', inProgress, 'in_progress')}
            {#if review.length}{@render lane('Review', review, 'review', 'sign off')}{/if}
            <div class="lane" role="list" ondragover={e => e.preventDefault()} ondrop={() => drop('done')}>
              <h6>Recently done <span>with evidence</span></h6>
              {#each done.slice(0, 6) as it (it.id)}
                <button class="ghost done-row" onclick={() => (drawer = it.id)}><span>{it.title}</span><code>{evShort(it)}</code></button>
              {/each}
              {#if !done.length}<div class="faint">Nothing done yet.</div>{/if}
            </div>
          </div>
        </div>
        {#if backlog.length > 12}<div class="faint pad">+{backlog.length - 12} more in the Backlog tab</div>{/if}
      {:else}
        {@const list = tab === 'backlog' ? backlog : tab === 'ideas' ? ideas : tab === 'someday' ? someday : done}
        <div class="list">
          {#each list as it (it.id)}{@render card(it)}{/each}
          {#if !list.length}<div class="faint">Nothing here.</div>{/if}
          {#if tab !== 'done'}{@render adder(tab === 'ideas' ? 'idea' : tab)}{/if}
        </div>
      {/if}
    </div>
  {/if}
</div>

{#if drawer}<ItemDrawer id={drawer} onclose={() => (drawer = null)} />{/if}

<script module>
  const focusEl = el => el.focus()
</script>

<style>
  .page { height: 100vh; display: flex; flex-direction: column; }
  header { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; border-bottom: 1px solid var(--line); background: var(--panel); flex: none; gap: 10px; }
  .left { display: flex; align-items: center; gap: 8px; }
  h4 { margin: 0; font-size: 15px; color: var(--strong); }
  nav button { margin-left: 4px; }
  nav button.on { color: var(--strong); box-shadow: inset 0 -2px 0 var(--accent); border-radius: 0; }
  .scroll { flex: 1; overflow: auto; padding: 8px 12px 16px; }
  .desc { display: block; color: var(--muted); padding: 2px 0; margin-bottom: 8px; width: 100%; text-align: left; }
  .inbox { background: var(--panel-2); border: 1px dashed #3a4150; border-radius: 6px; padding: 8px; margin-bottom: 12px; color: var(--muted); }
  .ibar { display: flex; align-items: center; gap: 10px; }
  .ibar span { white-space: nowrap; }
  .ibar input { flex: 1; min-width: 80px; }
  .tri { display: flex; gap: 4px; align-items: center; margin-top: 5px; }
  .tri .ttl { flex: 1; text-align: left; color: var(--text); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .lanes { display: grid; grid-template-columns: 1.1fr 1fr 1.3fr; gap: 10px; align-items: start; }
  .col { display: flex; flex-direction: column; gap: 10px; }
  .lane { background: var(--panel); border-radius: 6px; padding: 8px; min-height: 80px; }
  .t { display: block; width: 100%; text-align: left; color: var(--text); background: var(--card); border: 1px solid #303644; border-radius: 5px; padding: 6px 7px; margin-bottom: 5px; cursor: pointer; }
  .t:hover { background: #2a3040; }
  .pos { color: var(--muted); margin-right: 3px; }
  .meta { color: var(--muted); font-size: 10.5px; margin-top: 3px; }
  .dep { color: var(--yellow); }
  .claude { color: var(--purple); }
  code { color: var(--muted); font-size: 10.5px; }
  .done-row { display: flex; width: 100%; justify-content: space-between; gap: 8px; border-bottom: 1px solid var(--line); border-radius: 0; padding: 4px 0; text-align: left; color: var(--text); }
  .addbtn { color: var(--faint); padding: 2px 0; }
  .adder { width: 100%; }
  .list { max-width: 720px; }
  .pad { padding: 10px 0; }
</style>
