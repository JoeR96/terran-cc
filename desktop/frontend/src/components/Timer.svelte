<script>
  import { db, act } from '../lib/store.svelte.js'
  import { mmss } from '../lib/format.js'

  // A plain countdown: pick a length, it counts down, chimes and forgets.
  // compact: smaller digits (side widget). big: large digits (dashboard).
  let { compact = false, big = false } = $props()

  const MINUTES = [5, 10, 15, 20, 25, 30, 45, 60, 90]

  const t = $derived(db.timer)
  const idle = $derived(!t || t.state === 'idle')
  const pct = $derived(t?.total_s ? Math.min(100, (100 * (t.total_s - t.remaining_s)) / t.total_s) : 0)
</script>

{#if idle}
  <div class="grid" class:compact>
    {#each MINUTES as m (m)}
      <button title="Start a {m} minute timer" onclick={() => act.timer('start', { minutes: m })}>{m}</button>
    {/each}
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
{/if}

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(44px, 1fr)); gap: 5px; }
  .grid button { padding: 6px 0; font-variant-numeric: tabular-nums; font-weight: 600; }
  .grid.compact { grid-template-columns: repeat(auto-fill, minmax(36px, 1fr)); gap: 4px; }
  .grid.compact button { padding: 3px 0; font-size: 12px; }
  .pomo { display: flex; align-items: center; justify-content: space-between; }
  .t { font-size: 26px; font-weight: 600; color: var(--accent); padding: 0; font-variant-numeric: tabular-nums; }
  .t:hover { background: none; color: #ffa47c; }
  .t.paused { color: var(--muted); }
  .big .t { font-size: 48px; }
  .compact .t { font-size: 22px; }
  .ctl button { font-size: 13px; padding: 2px 7px; margin-left: 3px; }
  .bar { height: 3px; background: var(--line); border-radius: 2px; margin-top: 4px; overflow: hidden; }
  .bar div { height: 100%; background: var(--accent); transition: width 1s linear; }
</style>
