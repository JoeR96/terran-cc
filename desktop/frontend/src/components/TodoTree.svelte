<script>
  import { db, act } from '../lib/store.svelte.js'
  import { todoTree, moveBefore, descendants } from '../lib/lists.js'
  import { focus } from '../lib/actions.js'

  // Personal to-dos as a collapsible tree: Me / Work / OSRS › when › to-do ›
  // sub to-do. Styled like ProjectTree.
  let { onopen = null } = $props()

  const GROUPS = [
    { ctx: 'irl', name: 'Me', hz: [['today', 'Today'], ['week', 'This week'], ['month', 'This month']] },
    { ctx: 'work', name: 'Work', hz: [['today', 'Today'], ['week', 'This week'], ['month', 'This month']] },
    // OSRS keeps only Today here; its short, medium and long term goals live
    // in the OSRS tab, with their pieces and progress.
    // OSRS's This week is its short-term goals; medium and long term live
    // in the OSRS tab with their pieces and progress.
    { ctx: 'osrs', name: 'OSRS', hz: [['today', 'Today'], ['week', 'This week']], note: 'Longer goals live in the OSRS tab' },
  ]
  const OPEN_KEY = 'tcc.todo.open'
  const SHUT_KEY = 'tcc.todo.shut'
  // All sits on top and gathers each group's to-dos by date. OSRS terms are
  // goals rather than dates, so only OSRS's Today joins in.
  const ALL = [['today', 'Today', ['irl', 'work', 'osrs']], ['week', 'This week', ['irl', 'work', 'osrs']], ['month', 'This month', ['irl', 'work']]]
  const TAG = { irl: 'me', work: 'work', osrs: 'osrs' }
  const groupOf = ctx => GROUPS.find(g => g.ctx === ctx)
  const DEFAULT_OPEN = ['g:all', 'h:all:today', ...GROUPS.flatMap(g => [`g:${g.ctx}`, `h:${g.ctx}:today`])]

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
  // OSRS goals carry long piece lists, so they start folded and remember
  // being opened instead.
  let unfolded = $state(load('tcc.todo.osrsopen', []))
  const isOsrs = id => db.items.find(i => i.id === id)?.context === 'osrs'
  const isShut = id => (isOsrs(id) ? !unfolded.includes(id) : shut.includes(id))
  const toggleShut = id =>
    isOsrs(id) ? save('tcc.todo.osrsopen', (unfolded = flip(unfolded, id))) : save(SHUT_KEY, (shut = flip(shut, id)))

  // Drag a group row to reorder Me / Work / OSRS (kept per window). Drag a
  // to-do onto another to-do to put it in front of it (moving it to that
  // list and parent if they differ), or onto a section row to move it to the
  // top level there. Sub to-dos travel with their parent.
  let order = $state(load('tcc.todo.order', GROUPS.map(g => g.ctx)))
  const groups = $derived([...GROUPS].sort((a, b) => rank(a.ctx) - rank(b.ctx)))
  const rank = k => (order.includes(k) ? order.indexOf(k) : 99)
  let drag = $state(null) // {group} or {id, list}
  let over = $state(null)

  function dragStart(e, d) {
    drag = d
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(d.group ?? d.id))
  }
  function dragOver(e, key, ok) {
    if (!drag || !ok) return
    e.preventDefault()
    over = key
  }
  function dropGroup(ctx) {
    if (drag?.group) save('tcc.todo.order', (order = moveBefore(groups.map(g => g.ctx), drag.group, ctx)))
    drag = over = null
  }
  // move gives a to-do (and everything under it) a new section.
  async function move(id, ctx, when, parent) {
    await act.edit(id, { context: ctx, when, parent_id: parent })
    for (const d of descendants(db.items, id)) act.edit(d, { context: ctx, when })
  }
  async function dropTodo(target, siblings) {
    const d = drag
    drag = over = null
    if (!d?.id || d.id === target.id || descendants(db.items, d.id).includes(target.id)) return
    if (d.list !== listKey(siblings)) await move(d.id, ctx(target), target.when, target.parent_id ?? null)
    const ids = moveBefore([...siblings.map(n => n.item.id).filter(id => id !== d.id), d.id], d.id, target.id)
    ids.forEach((id, i) => {
      if (db.items.find(it => it.id === id)?.position !== i + 1) act.edit(id, { position: i + 1 })
    })
  }
  function dropSection(c, when) {
    const d = drag
    drag = over = null
    if (d?.id) move(d.id, c, when, null)
  }
  const ctx = it => it.context || 'irl'
  // Siblings are identified by their parent and list, which survive the
  // reloads that live updates cause mid-drag.
  const listKey = siblings => siblings.length && `${siblings[0].item.context}:${siblings[0].item.when}:${siblings[0].item.parent_id ?? ''}`

  const trees = $derived(Object.fromEntries(GROUPS.flatMap(g => g.hz.map(([w]) => [`${g.ctx}:${w}`, todoTree(db.items, w, g.ctx)]))))
  const allNodes = $derived(Object.fromEntries(ALL.map(([w, , ctxs]) => [w, ctxs.flatMap(c => trees[`${c}:${w}`])])))
  const openIn = nodes => nodes.reduce((n, c) => n + c.open + (c.item.status !== 'done'), 0)

  let adding = $state(null) // {ctx, when, parent}
  let draft = $state('')
  let editing = $state(null)
  let editDraft = $state('')

  function startAdd(ctx, when, parent = null) {
    adding = { ctx, when, parent }
    draft = ''
    if (!open.includes(`h:${ctx}:${when}`)) toggleOpen(`h:${ctx}:${when}`)
    if (parent && isShut(parent)) toggleShut(parent)
  }
  async function add() {
    const t = draft.trim()
    if (!t) return (adding = null)
    const a = adding
    draft = ''
    await act.addItem({ title: t, when: a.when, context: a.ctx, kind: 'todo', ...(a.parent ? { parent_id: a.parent } : {}) })
  }
  // Adding from All: the chip picks which group the to-do belongs to.
  let allAdding = $state(null) // {when, ctx}
  let allDraft = $state('')
  function startAllAdd(w) {
    allAdding = { when: w, ctx: 'irl' }
    allDraft = ''
    if (!open.includes(`h:all:${w}`)) toggleOpen(`h:all:${w}`)
  }
  function cycleCtx(ctxs) {
    allAdding.ctx = ctxs[(ctxs.indexOf(allAdding.ctx) + 1) % ctxs.length]
  }
  async function addAll() {
    const t = allDraft.trim()
    if (!t) return (allAdding = null)
    allDraft = ''
    await act.addItem({ title: t, when: allAdding.when, context: allAdding.ctx, kind: 'todo' })
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

{#snippet todo(n, g, when, depth, siblings, tag = false)}
  {@const it = n.item}
  <div
    class="row t"
    class:done={it.status === 'done'}
    class:over={over === 't' + it.id}
    style="padding-left:{depth * 14}px"
    draggable={editing !== it.id}
    role="listitem"
    ondragstart={e => (e.stopPropagation(), dragStart(e, { id: it.id, list: listKey(siblings) }))}
    ondragover={e => dragOver(e, 't' + it.id, !!drag?.id && drag.id !== it.id)}
    ondragleave={() => over === 't' + it.id && (over = null)}
    ondrop={e => (e.preventDefault(), e.stopPropagation(), dropTodo(it, siblings))}
    ondragend={() => (drag = over = null)}
  >
    {#if n.children.length}
      <button class="ghost caret" onclick={() => toggleShut(it.id)}>{isShut(it.id) ? '▸' : '▾'}</button>
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
      {#if tag}<span class="ctag {g.ctx}">{TAG[g.ctx]}</span>{/if}
      <span class="title" role="button" tabindex="0" title="Click to rename" onclick={() => ((editing = it.id), (editDraft = it.title))} onkeydown={() => {}}>{it.title}</span>
      {#if n.children.length && isShut(it.id)}<span class="faint n">{n.open}</span>{/if}
      <span class="acts">
        <button class="ghost" title="Add a sub to-do" onclick={() => startAdd(g.ctx, when, it.id)}>+</button>
        {#if onopen}<button class="ghost" title="Open" onclick={() => onopen(it)}>↗</button>{/if}
        <button class="ghost" title="Delete" onclick={() => act.remove(it.id)}>✕</button>
      </span>
    {/if}
  </div>
  {#if !isShut(it.id)}
    {#each n.children as c (c.item.id)}{@render todo(c, g, when, depth + 1, n.children)}{/each}
    {#if isAdding(g.ctx, when, it.id)}{@render addRow(depth + 1)}{/if}
  {/if}
{/snippet}

<h6>To-dos</h6>
<button class="ghost row node" onclick={() => toggleOpen('g:all')}>
  <span class="caret">{open.includes('g:all') ? '▾' : '▸'}</span><b>All</b><span class="faint n">{ALL.reduce((n, [w]) => n + openIn(allNodes[w]), 0)}</span>
</button>
{#if open.includes('g:all')}
  {#each ALL as [w, label] (w)}
    {@const hk = `h:all:${w}`}
    <div class="row hz" style="padding-left:14px">
      <button class="ghost node" onclick={() => toggleOpen(hk)}>
        <span class="caret">{open.includes(hk) ? '▾' : '▸'}</span>{label}<span class="faint n">{openIn(allNodes[w])}</span>
      </button>
      <button class="ghost plus" title="Add a to-do" onclick={() => startAllAdd(w)}>+</button>
    </div>
    {#if open.includes(hk)}
      {#each allNodes[w] as n (n.item.id)}{@render todo(n, groupOf(n.item.context || 'irl'), w, 2, trees[`${n.item.context || 'irl'}:${w}`], true)}{/each}
      {#if allAdding?.when === w}
        <div class="row" style="padding-left:44px">
          <button class="ghost ctag {allAdding.ctx}" title="Click to switch between me, work and osrs" onmousedown={e => e.preventDefault()} onclick={() => cycleCtx(ALL.find(a => a[0] === w)[2])}>{TAG[allAdding.ctx]}</button>
          <input
            placeholder="New to-do, Enter to add"
            bind:value={allDraft}
            onkeydown={e => {
              if (e.key === 'Enter') addAll()
              if (e.key === 'Escape') allAdding = null
            }}
            onblur={() => {
              if (!allDraft.trim()) allAdding = null
            }}
            use:focus
          />
        </div>
      {/if}
    {/if}
  {/each}
{/if}
<div class="sep"></div>
{#each groups as g (g.ctx)}
  {@const gk = `g:${g.ctx}`}
  <button
    class="ghost row node grp"
    class:over={over === gk}
    draggable="true"
    title="Click to fold, drag to reorder"
    onclick={() => toggleOpen(gk)}
    ondragstart={e => dragStart(e, { group: g.ctx })}
    ondragover={e => dragOver(e, gk, !!drag?.group && drag.group !== g.ctx)}
    ondragleave={() => over === gk && (over = null)}
    ondrop={e => (e.preventDefault(), dropGroup(g.ctx))}
    ondragend={() => (drag = over = null)}
  >
    <span class="caret">{open.includes(gk) ? '▾' : '▸'}</span><b>{g.name}</b><span class="faint n">{g.hz.reduce((n, [w]) => n + openIn(trees[`${g.ctx}:${w}`]), 0)}</span>
  </button>
  {#if open.includes(gk)}
    {#if g.note}<div class="row gnote faint" style="padding-left:14px">{g.note}</div>{/if}
    {#each g.hz as [w, label] (w)}
      {@const hk = `h:${g.ctx}:${w}`}
      {@const nodes = trees[`${g.ctx}:${w}`]}
      <div
        class="row hz"
        class:over={over === hk}
        style="padding-left:14px"
        role="group"
        ondragover={e => dragOver(e, hk, !!drag?.id)}
        ondragleave={() => over === hk && (over = null)}
        ondrop={e => (e.preventDefault(), dropSection(g.ctx, w))}
      >
        <button class="ghost node" onclick={() => toggleOpen(hk)}>
          <span class="caret">{open.includes(hk) ? '▾' : '▸'}</span>{label}<span class="faint n">{openIn(nodes)}</span>
        </button>
        <button class="ghost plus" title="Add a to-do" onclick={() => startAdd(g.ctx, w)}>+</button>
      </div>
      {#if open.includes(hk)}
        {#each nodes as n (n.item.id)}{@render todo(n, g, w, 2, nodes)}{/each}
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
  .ctag { flex: none; font-size: 9.5px; line-height: 14px; padding: 0 4px; border-radius: 3px; background: var(--btn); color: var(--muted); }
  .ctag.work { background: var(--blue-bg); color: var(--blue); }
  .ctag.osrs { background: var(--yellow-bg); color: var(--yellow); }
  .gnote { font-size: 10.5px; }
  .sep { border-top: 1px solid var(--line); margin: 5px 0; }
  .over { box-shadow: inset 0 2px 0 var(--accent); }
  .grp, .t { cursor: grab; }
  .edit, .row input { flex: 1; min-width: 0; padding: 0 4px; }
</style>
