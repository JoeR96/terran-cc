<script>
  import { db, bucket } from '../lib/store.svelte.js'
  import { shell, conn } from '../lib/api.js'
  import { fitSections } from '../lib/fit.js'
  import { dayLabel } from '../lib/format.js'
  import Timer from '../components/Timer.svelte'
  import Habits from '../components/Habits.svelte'
  import TodoList from '../components/TodoList.svelte'
  import { route } from '../lib/route.svelte.js'

  const FOLD_H = 28
  const flexSecs = [
    { key: 'today', label: 'Today' },
    { key: 'week', label: 'Week', title: 'This week' },
    { key: 'month', label: 'Month', title: 'This month' },
  ]

  let bodyH = $state(0)
  let heights = $state({})
  let expanded = $state([])
  let menu = $state(false)
  let onTop = $state(route.query.get('top') !== '0')

  const fit = $derived(
    fitSections(
      [
        { key: 'timer', h: heights.timer || 0, fixed: true },
        { key: 'habits', h: heights.habits || 0, fixed: true },
        ...flexSecs.map(s => ({ key: s.key, h: heights[s.key] || 0 })),
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
</script>

<div class="sw">
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

  <div class="body" bind:clientHeight={bodyH}>
    <div class="sec" bind:offsetHeight={heights.timer}><Timer compact /></div>
    <div class="sec" bind:offsetHeight={heights.habits}><Habits /></div>
    {#each flexSecs as s (s.key)}
      <div class="sec" class:measure={!isShown(s.key)} bind:offsetHeight={heights[s.key]}>
        <div class="exp">
          {#if expanded.includes(s.key)}
            <button class="ghost fold" title="Fold" onclick={() => toggleExpand(s.key)}>▾</button>
          {/if}
          <TodoList when={s.key} title={s.title || s.label} limit={s.key === 'today' ? 0 : 6} />
        </div>
      </div>
    {/each}
    {#if fit.folded.length}
      <div class="collapsed">
        ▸
        {#each fit.folded as k, i (k)}
          {#if i} · {/if}
          <button class="ghost" onclick={() => toggleExpand(k)}>
            {flexSecs.find(s => s.key === k).label} ({bucket(k).filter(it => it.status !== 'done').length})
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .sw { height: 100vh; display: flex; flex-direction: column; background: var(--panel); border: 1px solid var(--line-2); }
  .hd { display: flex; justify-content: space-between; align-items: center; padding: 3px 4px 3px 10px; background: var(--panel-2); border-bottom: 1px solid var(--line); color: var(--muted); font-size: 11px; flex: none; }
  .btns button { padding: 0 6px; }
  .menu { display: flex; flex-direction: column; background: var(--panel-2); border-bottom: 1px solid var(--line); padding: 4px; }
  .menu button { text-align: left; color: var(--text); }
  .body { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; position: relative; }
  .sec { padding: 8px 10px; border-bottom: 1px solid var(--line); }
  .sec.measure { position: absolute; visibility: hidden; left: 0; right: 0; pointer-events: none; }
  .exp { position: relative; }
  .fold { position: absolute; right: -6px; top: -4px; font-size: 10px; }
  .collapsed { color: var(--faint); padding: 5px 10px; font-size: 11px; }
  .collapsed button { color: var(--muted); padding: 0 2px; }
</style>
