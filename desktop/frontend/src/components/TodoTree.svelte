<script>
  import { db, act } from '../lib/store.svelte.js'
  import { todoTree } from '../lib/lists.js'
  import { focus } from '../lib/actions.js'

  // Personal to-dos as a collapsible tree: Me / Work / OSRS › when › to-do ›
  // sub to-do. Styled like ProjectTree.
  let { onopen = null } = $props()

  const GROUPS = [
    { ctx: 'irl', name: 'Me', hz: [['today', 'Today'], ['week', 'This week'], ['month', 'This month']] },
    { ctx: 'work', name: 'Work', hz: [['today', 'Today'], ['week', 'This week'], ['month', 'This month']] },
    // OSRS goals reuse the "when" buckets: short = week, medium = month, long = anytime.
    { ctx: 'osrs', name: 'OSRS', hz: [['today', 'Today'], ['week', 'Short term'], ['month', 'Medium term'], ['anytime', 'Long term']] },
  ]
  const OPEN_KEY = 'tcc.todo.open'
  const SHUT_KEY = 'tcc.todo.shut'
  const DEFAULT_OPEN = GROUPS.flatMap(g => [`g:${g.ctx}`, `h:${g.ctx}:today`])

  const load = (k, d) => {
    try {
      return JSON.parse(localStorage.getItem(k)) ?? d
    } catch {
      return d
    }
  }
  const save = (k, v) => {
    try {
      localStorage.setItem(k, JSON.stringify(v))
    } catch {}
  }

  // Groups and sections start closed unless opened; to-dos with sub to-dos
  // start open unless shut.
  let open = $state(load(OPEN_KEY, DEFAULT_OPEN))
  let shut = $state(load(SHUT_KEY, []))
  const flip = (list, k) => (list.includes(k) ? list.filter(x => x !== k) : [...list, k])
  const toggleOpen = k => save(OPEN_KEY, (open = flip(open, k)))
  const toggleShut = id => save(SHUT_KEY, (shut = flip(shut, id)))

  const trees = $derived(Object.fromEntries(GROUPS.flatMap(g => g.hz.map(([w]) => [`${g.ctx}:${w}`, todoTree(db.items, w, g.ctx)]))))
  const openIn = nodes => nodes.reduce((n, c) => n + c.open + (c.item.status !== 'done'), 0)

  let adding = $state(null) // {ctx, when, parent}
  let draft = $state('')
  let editing = $state(null)
  let editDraft = $state('')

  function startAdd(ctx, when, parent = null) {
    adding = { ctx, when, parent }
    draft = ''
    if (!open.includes(`h:${ctx}:${when}`)) toggleOpen(`h:${ctx}:${when}`)
    if (parent && shut.includes(parent)) toggleShut(parent)
  }
  async function add() {
    const t = draft.trim()
    if (!t) return (adding = null)
    const a = adding
    draft = ''
    await act.addItem({ title: t, when: a.when, context: a.ctx, kind: 'todo', ...(a.parent ? { parent_id: a.parent } : {}) })
  }
  function rename(it) {
    const t = editDraft.trim()
    editing = null
    if (t && t !== it.title) act.edit(it.id, { title: t })
  }
  const isAdding = (ctx, when, parent) => adding && adding.ctx === ctx && adding.when === when && adding.parent === parent
</script>

{#snippet addRow(depth)}
  <div class="row" style="padding-left:{depth * 14 + 16}px">
    <input
      placeholder={adding.parent ? 'Sub to-do, Enter to add' : 'New to-do, Enter to add'}
      bind:value={draft}
      onkeydown={e => {
        if (e.key === 'Enter') add()
        if (e.key === 'Escape') adding = null
      }}
      onblur={() => {
        if (!draft.trim()) adding = null
      }}
      use:focus
    />
  </div>
{/snippet}

{#snippet todo(n, g, when, depth)}
  {@const it = n.item}
  <div class="row t" class:done={it.status === 'done'} style="padding-left:{depth * 14}px">
    {#if n.children.length}
      <button class="ghost caret" onclick={() => toggleShut(it.id)}>{shut.includes(it.id) ? '▸' : '▾'}</button>
    {:else}
      <span class="caret"></span>
    {/if}
    <button class="box" class:on={it.status === 'done'} aria-label="Done" title={it.status === 'done' ? 'Reopen' : 'Done'} onclick={() => (it.status === 'done' ? act.reopen(it.id) : act.done(it.id))}></button>
    {#if editing === it.id}
      <input
        class="edit"
        bind:value={editDraft}
        onblur={() => rename(it)}
        onkeydown={e => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') editing = null
        }}
        use:focus
      />
    {:else}
      <span class="title" role="button" tabindex="0" title="Click to rename" onclick={() => ((editing = it.id), (editDraft = it.title))} onkeydown={() => {}}>{it.title}</span>
      {#if n.children.length && shut.includes(it.id)}<span class="faint n">{n.open}</span>{/if}
      <span class="acts">
        <button class="ghost" title="Add a sub to-do" onclick={() => startAdd(g.ctx, when, it.id)}>+</button>
        {#if onopen}<button class="ghost" title="Open" onclick={() => onopen(it)}>↗</button>{/if}
        <button class="ghost" title="Delete" onclick={() => act.remove(it.id)}>✕</button>
      </span>
    {/if}
  </div>
  {#if !shut.includes(it.id)}
    {#each n.children as c (c.item.id)}{@render todo(c, g, when, depth + 1)}{/each}
    {#if isAdding(g.ctx, when, it.id)}{@render addRow(depth + 1)}{/if}
  {/if}
{/snippet}

<h6>To-dos</h6>
{#each GROUPS as g (g.ctx)}
  {@const gk = `g:${g.ctx}`}
  <button class="ghost row node" onclick={() => toggleOpen(gk)}>
    <span class="caret">{open.includes(gk) ? '▾' : '▸'}</span><b>{g.name}</b><span class="faint n">{g.hz.reduce((n, [w]) => n + openIn(trees[`${g.ctx}:${w}`]), 0)}</span>
  </button>
  {#if open.includes(gk)}
    {#each g.hz as [w, label] (w)}
      {@const hk = `h:${g.ctx}:${w}`}
      {@const nodes = trees[`${g.ctx}:${w}`]}
      <div class="row hz" style="padding-left:14px">
        <button class="ghost node" onclick={() => toggleOpen(hk)}>
          <span class="caret">{open.includes(hk) ? '▾' : '▸'}</span>{label}<span class="faint n">{openIn(nodes)}</span>
        </button>
        <button class="ghost plus" title="Add a to-do" onclick={() => startAdd(g.ctx, w)}>+</button>
      </div>
      {#if open.includes(hk)}
        {#each nodes as n (n.item.id)}{@render todo(n, g, w, 2)}{/each}
        {#if isAdding(g.ctx, w, null)}{@render addRow(2)}{/if}
      {/if}
    {/each}
  {/if}
{/each}

<style>
  .row { display: flex; align-items: center; gap: 6px; min-height: 21px; width: 100%; }
  .node { text-align: left; color: var(--text); padding: 1px 2px; display: flex; align-items: center; gap: 4px; min-width: 0; flex: 1; white-space: nowrap; overflow: hidden; }
  .node:hover { background: none; color: var(--strong); }
  .caret { width: 10px; color: var(--muted); flex: none; padding: 0; font-size: inherit; }
  button.caret:hover { background: none; color: var(--text); }
  .n { margin-left: 6px; font-size: 10.5px; }
  .hz .plus { display: none; padding: 0 5px; }
  .hz:hover .plus { display: inline; }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: text; }
  .done .title { text-decoration: line-through; color: var(--faint); }
  .acts { display: none; gap: 1px; align-items: center; flex: none; }
  .t:hover .acts { display: flex; }
  .acts button { padding: 0 4px; font-size: 11px; }
  .edit, .row input { flex: 1; min-width: 0; padding: 0 4px; }
</style>
