<script>
  import { db, act } from '../lib/store.svelte.js'
  import { mmss } from '../lib/format.js'
  import { focus } from '../lib/actions.js'

  // A plain countdown: pick a length, it counts down, chimes and forgets.
  // compact: smaller digits (side widget). big: large digits (dashboard).
  let { compact = false, big = false } = $props()

  const MINUTES = [5, 10, 15, 20, 25, 30, 45, 60, 90]

  const t = $derived(db.timer)
  const idle = $derived(!t || t.state === 'idle')
  // Lock in: 🔒 asks for a task, then a time; the side widget then shows only
  // the countdown and the task.
  let asking = $state(false)
  let task = $state('')
  let picked = $state('')
  function start(m) {
    act.timer('start', picked ? { minutes: m, label: picked } : { minutes: m })
    asking = false
    picked = task = ''
  }
  const pct = $derived(t?.total_s ? Math.min(100, (100 * (t.total_s - t.remaining_s)) / t.total_s) : 0)
</script>

{#if idle && asking && !picked}
  <div class="ask">
    <input
      placeholder="Lock in on… (Enter, then pick a time)"
      bind:value={task}
      onkeydown={e => {
        if (e.key === 'Enter' && task.trim()) picked = task.trim()
        if (e.key === 'Escape') asking = false
      }}
      use:focus
    />
    <button class="ghost" title="Cancel" onclick={() => (asking = false)}>✕</button>
  </div>
{:else if idle}
  {#if picked}
    <div class="for">🔒 <b>{picked}</b> for… <button class="ghost" title="Cancel" onclick={() => ((picked = task = ''), (asking = false))}>✕</button></div>
  {/if}
  <div class="grid" class:compact>
    {#each MINUTES as m (m)}
      <button title="Start a {m} minute timer" onclick={() => start(m)}>{m}</button>
    {/each}
    {#if !picked}<button class="lock" title="Lock in on a task: name it, then pick a time" onclick={() => (asking = true)}>🔒</button>{/if}
  </div>
{:else}
  <div class="pomo" class:big class:compact>
    <button class="ghost t" class:paused={t.state === 'paused'} title={t.state === 'running' ? 'Pause' : 'Resume'} onclick={() => act.timer('toggle')}>
      {mmss(t.remaining_s)}
    </button>
    <span class="ctl">
      <button title={t.state === 'running' ? 'Pause' : 'Resume'} onclick={() => act.timer('toggle')}>{t.state === 'running' ? '⏸' : '▶'}</button>
      <button title="Stop" onclick={() => act.timer('stop')}>⏹</button>
    </span>
  </div>
  <div class="bar"><div style="width: {pct}%"></div></div>
  {#if t.label}<div class="label" title={t.label}>🔒 {t.label}</div>{/if}
{/if}

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(44px, 1fr)); gap: 5px; }
  .grid button { padding: 6px 0; font-variant-numeric: tabular-nums; font-weight: 600; }
  .grid.compact { grid-template-columns: repeat(auto-fill, minmax(36px, 1fr)); gap: 4px; }
  .grid.compact button { padding: 3px 0; font-size: 12px; }
  .pomo { display: flex; align-items: center; justify-content: space-between; }
  .t { font-size: 26px; font-weight: 600; color: var(--accent); padding: 0; font-variant-numeric: tabular-nums; }
  .t:hover { background: none; color: var(--accent-hi); }
  .t.paused { color: var(--muted); }
  .big .t { font-size: 48px; }
  .compact .t { font-size: 22px; }
  .ctl button { font-size: 13px; padding: 2px 7px; margin-left: 3px; }
  .lock { background: var(--yellow-bg); }
  .ask { display: flex; gap: 4px; }
  .ask input { flex: 1; min-width: 0; }
  .for { margin-bottom: 5px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .for b { color: var(--text); font-weight: 600; }
  .for button { padding: 0 4px; }
  .label { margin-top: 4px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .bar { height: 3px; background: var(--line); border-radius: 2px; margin-top: 4px; overflow: hidden; }
  .bar div { height: 100%; background: var(--accent); transition: width 1s linear; }
</style>
