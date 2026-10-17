<script>
  import { db } from '../lib/store.svelte.js'
  import { shell, conn } from '../lib/api.js'
  import { fitSections } from '../lib/fit.js'
  import { dayLabel } from '../lib/format.js'
  import { todos, goals, buildTree, ACTIVE } from '../lib/lists.js'
  import Timer from '../components/Timer.svelte'
  import Habits from '../components/Habits.svelte'
  import TodoList from '../components/TodoList.svelte'
  import Goals from '../components/Goals.svelte'
  import ProjectTree from '../components/ProjectTree.svelte'
  import ItemDrawer from '../components/ItemDrawer.svelte'
  import { route } from '../lib/route.svelte.js'

  // Wider than this, the widget shows habits across the top and splits the
  // rest into to-dos (left) and goals + projects (right).
  const WIDE = 560
  const FOLD_H = 28
  const open = list => list.filter(it => it.status !== 'done').length
  // Every section the narrow widget can show, in order. todo sections carry
  // when + context; the rest are goals and the project tree.
  const secs = [
    { key: 'today', label: 'Today', title: 'Today · me', when: 'today', context: 'irl' },
    { key: 'todayw', label: 'Work', title: 'Today · work', when: 'today', context: 'work' },
    { key: 'goals', label: 'Goals' },
    { key: 'weekw', label: 'Week · work', title: 'This week · work', when: 'week', context: 'work' },
    { key: 'week', label: 'Week · me', title: 'This week · me', when: 'week', context: 'irl' },
    { key: 'month', label: 'Month · me', title: 'This month · me', when: 'month', context: 'irl' },
    { key: 'monthw', label: 'Month · work', title: 'This month · work', when: 'month', context: 'work' },
    { key: 'tree', label: 'Projects' },
  ]
  const byKey = k => secs.find(s => s.key === k)
  const count = s =>
    s.when ? open(todos(db.items, s.when, s.context)) : s.key === 'goals' ? open(goals(db.items)) : buildTree(db.projects, db.items, ACTIVE).reduce((n, p) => n + p.count, 0)
  // In the wide layout these fold into one line under the left column.
  const leftMain = ['today', 'todayw', 'weekw']
  const leftFold = ['week', 'month', 'monthw']

  let width = $state(0)
  let bodyH = $state(0)
  let heights = $state({})
  let expanded = $state([])
  let menu = $state(false)
  let drawer = $state(null)
  let onTop = $state(route.query.get('top') !== '0')
  const wide = $derived(width >= WIDE)

  const fit = $derived(
    fitSections(
      [
        { key: 'timer', h: heights.timer || 0, fixed: true },
        { key: 'habits', h: heights.habits || 0, fixed: true },
        ...secs.map(s => ({ key: s.key, h: heights[s.key] || 0 })),
      ],
      bodyH,
      FOLD_H,
      expanded,
    ),
  )
  const isShown = k => fit.shown.includes(k)

  function toggleExpand(k) {
    expanded = expanded.includes(k) ? expanded.filter(x => x !== k) : [...expanded, k]
  }
  function setTop() {
    onTop = !onTop
    shell('top', { on: onTop })
    menu = false
  }
  const openItem = it => (drawer = it.id)
</script>

{#snippet section(s)}
  {#if s.when}
    <TodoList when={s.when} context={s.context} title={s.title} limit={s.key === 'today' || s.key === 'todayw' ? 0 : 8} onopen={openItem} />
  {:else if s.key === 'goals'}
    <Goals onopen={openItem} />
  {:else}
    <ProjectTree onopen={openItem} />
  {/if}
{/snippet}

{#snippet foldLine(keys)}
  <div class="collapsed">
    ▸
    {#each keys as k, i (k)}
      {#if i} · {/if}
      <button class="ghost" onclick={() => toggleExpand(k)}>{byKey(k).label} ({count(byKey(k))})</button>
    {/each}
  </div>
{/snippet}

<div class="sw" bind:clientWidth={width}>
  <div class="hd drag">
    <span>{dayLabel(new Date((db.date || new Date().toISOString().slice(0, 10)) + 'T12:00'))}</span>
    <span class="nodrag btns">
      <button class="ghost" title="Menu" onclick={() => (menu = !menu)}>⋯</button>
      {#if conn().shell}
        <button class="ghost" title="Minimise" onclick={() => shell('minimise')}>–</button>
        <button class="ghost" title="Close" onclick={() => shell('close')}>✕</button>
      {/if}
    </span>
  </div>
  {#if menu}
    <div class="menu">
      <button class="ghost" onclick={() => { shell('open', { kind: 'dashboard' }); menu = false }}>Open dashboard</button>
      {#if conn().shell}
        <button class="ghost" onclick={setTop}>{onTop ? '✓ ' : ''}Always on top</button>
      {/if}
    </div>
  {/if}

  {#if wide}
    <div class="body wide">
      <div class="sec"><Timer /></div>
      <div class="sec"><Habits wide /></div>
      <div class="cols">
        <div class="col">
          {#each leftMain as k (k)}<div class="part">{@render section(byKey(k))}</div>{/each}
          {#each leftFold.filter(k => expanded.includes(k)) as k (k)}
            <div class="part exp">
              <button class="ghost fold" title="Fold" onclick={() => toggleExpand(k)}>▾</button>
              {@render section(byKey(k))}
            </div>
          {/each}
          {#if leftFold.some(k => !expanded.includes(k))}{@render foldLine(leftFold.filter(k => !expanded.includes(k)))}{/if}
        </div>
        <div class="col">
          <div class="part">{@render section(byKey('goals'))}</div>
          <div class="part">{@render section(byKey('tree'))}</div>
        </div>
      </div>
    </div>
  {:else}
    <div class="body" bind:clientHeight={bodyH}>
      <div class="sec" bind:offsetHeight={heights.timer}><Timer compact /></div>
      <div class="sec" bind:offsetHeight={heights.habits}><Habits /></div>
      {#each secs as s (s.key)}
        <div class="sec" class:measure={!isShown(s.key)} bind:offsetHeight={heights[s.key]}>
          <div class="exp">
            {#if expanded.includes(s.key)}
              <button class="ghost fold" title="Fold" onclick={() => toggleExpand(s.key)}>▾</button>
            {/if}
            {@render section(s)}
          </div>
        </div>
      {/each}
      {#if fit.folded.length}{@render foldLine(fit.folded)}{/if}
    </div>
  {/if}
</div>

{#if drawer}<ItemDrawer id={drawer} onclose={() => (drawer = null)} />{/if}

<style>
  .sw { height: 100vh; display: flex; flex-direction: column; background: var(--panel); border: 1px solid var(--line-2); }
  .hd { display: flex; justify-content: space-between; align-items: center; padding: 3px 4px 3px 10px; background: var(--panel-2); border-bottom: 1px solid var(--line); color: var(--muted); font-size: 11px; flex: none; }
  .btns button { padding: 0 6px; }
  .menu { display: flex; flex-direction: column; background: var(--panel-2); border-bottom: 1px solid var(--line); padding: 4px; }
  .menu button { text-align: left; color: var(--text); }
  .body { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; position: relative; }
  .body.wide { display: flex; flex-direction: column; overflow: hidden; }
  .wide .sec { flex: none; }
  .sec { padding: 8px 10px; border-bottom: 1px solid var(--line); }
  .sec.measure { position: absolute; visibility: hidden; left: 0; right: 0; pointer-events: none; }
  .cols { flex: 1; min-height: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 8px 10px; }
  .col { min-height: 0; overflow-y: auto; background: var(--bg); border-radius: 7px; padding: 6px 10px; }
  .part { padding: 4px 0 8px; }
  .part + .part { border-top: 1px solid var(--line); padding-top: 8px; }
  .exp { position: relative; }
  .fold { position: absolute; right: -6px; top: -4px; font-size: 10px; }
  .part.exp .fold { top: 2px; }
  .collapsed { color: var(--faint); padding: 5px 10px; font-size: 11px; }
  .col .collapsed { padding: 6px 0 0; border-top: 1px solid var(--line); }
  .collapsed button { color: var(--muted); padding: 0 2px; }
</style>
