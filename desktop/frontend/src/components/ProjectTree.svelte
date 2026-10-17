<script>
  import { db, act } from '../lib/store.svelte.js'
  import { buildTree, ACTIVE, moveBefore } from '../lib/lists.js'

  // Collapsible project › feature › ticket tree. T / W / M on a ticket makes
  // it a goal for today, this week or this month.
  let { onopen = null } = $props()

  const OPEN_KEY = 'tcc.tree.open'
  const ALL_OPEN = ['inbox', 'backlog', 'up_next', 'in_progress', 'review']
  const STATUS = { inbox: 'inbox', backlog: 'backlog', up_next: 'up next', in_progress: 'in progress', review: 'review' }

  const load = (k, d) => {
    try {
      return JSON.parse(localStorage.getItem(k)) ?? d
    } catch {
      return d
    }
  }
  const store = (k, v) => {
    try {
      localStorage.setItem(k, JSON.stringify(v))
    } catch {}
  }

  let open = $state(load(OPEN_KEY, []))
  let all = $state(load('tcc.tree.all', false))
  // Projects show in the order you drag them into (kept per window).
  let order = $state(load('tcc.tree.order', []))
  const rank = id => (order.includes(id) ? order.indexOf(id) : 1e6 + id)
  const tree = $derived(buildTree(db.projects, db.items, all ? ALL_OPEN : ACTIVE).sort((a, b) => rank(a.id) - rank(b.id)))

  // Drag a project onto another to reorder. Drag a ticket onto a feature to
  // file it there, or onto a project to put it straight under that project.
  let drag = $state(null) // {project} or {ticket}
  let over = $state(null)
  function dragStart(e, d) {
    drag = d
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(d.project ?? d.ticket))
  }
  function dragOver(e, key, ok) {
    if (!ok) return
    e.preventDefault()
    over = key
  }
  function dropProject(p) {
    const d = drag
    drag = over = null
    if (d?.project) store('tcc.tree.order', (order = moveBefore(tree.map(t => t.id), d.project, p.id)))
    else if (d?.ticket) act.edit(d.ticket, { project_id: p.id, parent_id: null })
  }
  function dropFeature(f) {
    const d = drag
    drag = over = null
    if (d?.ticket && d.ticket !== f.id) act.edit(d.ticket, { project_id: f.project_id, parent_id: f.id })
  }
  const end = () => (drag = over = null)

  function toggle(key) {
    open = open.includes(key) ? open.filter(k => k !== key) : [...open, key]
    store(OPEN_KEY, open)
  }
  function setAll() {
    all = !all
    store('tcc.tree.all', all)
  }
  const goal = (it, when) => act.edit(it.id, { when: it.when === when ? 'anytime' : when })
</script>

{#snippet ticket(it, depth)}
  <div class="row t" style="padding-left:{depth * 14}px" draggable="true" role="listitem" ondragstart={e => dragStart(e, { ticket: it.id })} ondragend={end}>
    <button class="box" class:on={it.status === 'done'} aria-label="Done" title="Done" onclick={() => act.done(it.id)}></button>
    {#if it.kind !== 'feature' && it.kind !== 'todo'}<span class="tag {it.kind}">{it.kind}</span>{/if}
    <span class="title" role="button" tabindex="0" title={it.title} onclick={() => onopen?.(it)} onkeydown={() => {}}>{it.title}</span>
    <span class="st {it.status}">{STATUS[it.status]}</span>
    <span class="hz" class:set={it.when in { today: 1, week: 1, month: 1 }}>
      {#each [['today', 'T'], ['week', 'W'], ['month', 'M']] as [w, l] (w)}
        <button class="ghost" class:on={it.when === w} title={it.when === w ? 'Remove from goals' : `Goal for ${w === 'today' ? 'today' : 'this ' + w}`} onclick={() => goal(it, w)}>{l}</button>
      {/each}
    </span>
  </div>
{/snippet}

<h6>All projects <button class="ghost filter" title="Switch between active work and everything open" onclick={setAll}>{all ? 'all open' : 'active'}</button></h6>
{#each tree as p (p.id)}
  {@const pk = 'p' + p.id}
  <button
    class="ghost row node proj"
    class:over={over === pk}
    draggable="true"
    title="Click to fold, drag to reorder"
    onclick={() => toggle(pk)}
    ondragstart={e => dragStart(e, { project: p.id })}
    ondragover={e => dragOver(e, pk, (!!drag?.project && drag.project !== p.id) || !!drag?.ticket)}
    ondragleave={() => over === pk && (over = null)}
    ondrop={e => (e.preventDefault(), dropProject(p))}
    ondragend={end}
  >
    <span class="caret">{open.includes(pk) ? '▾' : '▸'}</span><b>{p.project.name}</b><span class="faint n">{p.count}</span>
  </button>
  {#if open.includes(pk)}
    {#each p.children as c (c.item.id)}
      {#if c.children.length}
        {@const fk = 'f' + c.item.id}
        <div
          class="row feat"
          class:over={over === fk}
          style="padding-left:14px"
          role="group"
          ondragover={e => dragOver(e, fk, !!drag?.ticket && drag.ticket !== c.item.id)}
          ondragleave={() => over === fk && (over = null)}
          ondrop={e => (e.preventDefault(), dropFeature(c.item))}
        >
          <button class="ghost node" onclick={() => toggle(fk)}>
            <span class="caret">{open.includes(fk) ? '▾' : '▸'}</span><span class="tag feature">feature</span>{c.item.title}<span class="faint n">{c.count}</span>
          </button>
          <button class="ghost open" title="Open feature" onclick={() => onopen?.(c.item)}>↗</button>
        </div>
        {#if open.includes(fk)}
          {#each c.children as k (k.item.id)}{@render ticket(k.item, 2)}{/each}
        {/if}
      {:else}
        {@render ticket(c.item, 1)}
      {/if}
    {/each}
  {/if}
{:else}
  <div class="faint">Nothing {all ? 'open' : 'active'} in any project.</div>
{/each}

<style>
  .over { box-shadow: inset 0 2px 0 var(--accent); }
  .proj, .t { cursor: grab; }
  .filter { font-size: 10px; letter-spacing: 0; text-transform: none; padding: 0 5px; }
  .row { display: flex; align-items: center; gap: 6px; min-height: 21px; width: 100%; }
  .node { text-align: left; color: var(--text); padding: 1px 2px; display: flex; align-items: center; gap: 4px; min-width: 0; flex: 1; white-space: nowrap; overflow: hidden; }
  .node:hover { background: none; color: var(--strong); }
  .caret { width: 10px; color: var(--muted); flex: none; }
  .n { margin-left: 6px; font-size: 10.5px; }
  .feat .open { display: none; padding: 0 3px; font-size: 11px; }
  .feat:hover .open { display: inline; }
  .t .box { margin-left: 14px; }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: pointer; }
  .title:hover { color: var(--strong); }
  .st { flex: none; font-size: 9.5px; color: var(--muted); border: 1px solid var(--line-2); border-radius: 3px; padding: 0 4px; line-height: 14px; }
  .st.review { color: var(--yellow); border-color: #5a4d2a; }
  .st.in_progress { color: #9cc4ff; border-color: #2f4560; }
  .hz { display: none; flex: none; }
  .hz.set, .t:hover .hz { display: flex; }
  .hz button { padding: 0 3px; font-size: 10px; line-height: 14px; }
  .hz.set button:not(.on) { visibility: hidden; }
  .t:hover .hz button { visibility: visible; }
  .hz button.on { color: var(--accent); }
</style>
