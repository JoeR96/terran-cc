<script>
  import { db, act, itemTitle, bucket } from '../lib/store.svelte.js'
  import { mmss } from '../lib/format.js'

  // compact: one line when idle (side widget). big: large digits (dashboard).
  let { compact = false, big = false } = $props()

  const t = $derived(db.timer)
  const idle = $derived(!t || t.state === 'idle')
  const phaseName = $derived(
    !t ? '' : t.phase === 'focus' ? 'Focus' : t.phase === 'short_break' ? 'Short break' : 'Long break',
  )
  const cycleLabel = $derived(t ? `${(t.cycle % (t.config?.long_every || 4)) + 1} of ${t.config?.long_every || 4}` : '')
  const target = $derived(t?.item_id ? itemTitle(t.item_id) : '')

  let picking = $state(false)
  const choices = $derived(
    [...bucket('today'), ...db.items.filter(i => i.status === 'in_progress' && i.when !== 'today')].filter(
      i => i.status !== 'done',
    ),
  )

  function startWith(id) {
    picking = false
    act.timer('start', id ? { item_id: id } : {})
  }
</script>

{#if idle && compact}
  <div class="line">
    <button class="play" title="Start focus" onclick={() => (picking = !picking)}>▶</button>
    <span class="muted">Focus {mmss((t?.config?.focus_min || 25) * 60)}</span>
    <span class="faint">· idle</span>
  </div>
{:else}
  <div class="pomo" class:big>
    <button class="ghost t" class:brk={t?.phase !== 'focus'} title="Start / pause" onclick={() => (idle ? (picking = !picking) : act.timer('toggle'))}>
      {mmss(t?.remaining_s ?? 1500)}
    </button>
    <span class="ctl">
      {#if idle}
        <button title="Start focus" onclick={() => (picking = !picking)}>▶</button>
      {:else}
        <button title={t.state === 'running' ? 'Pause' : 'Resume'} onclick={() => act.timer('toggle')}>{t.state === 'running' ? '⏸' : '▶'}</button>
        <button title="Skip to next phase" onclick={() => act.timer('skip')}>⏭</button>
        <button title="Stop" onclick={() => act.timer('stop')}>⏹</button>
      {/if}
    </span>
  </div>
  <div class="sub muted">
    {#if idle}Idle{:else}{phaseName} · {cycleLabel}{#if t.state === 'paused'} · paused{/if}{/if}{#if target}{' · '}<span class="target">{target}</span>{/if}
  </div>
{/if}

{#if picking}
  <div class="pick">
    <div class="faint">Focus on…</div>
    <button class="ghost row" onclick={() => startWith(null)}>Nothing in particular</button>
    {#each choices as it (it.id)}
      <button class="ghost row" onclick={() => startWith(it.id)}>{it.title}</button>
    {/each}
  </div>
{/if}

<style>
  .line { display: flex; align-items: center; gap: 6px; }
  .play { padding: 0 7px; }
  .pomo { display: flex; align-items: center; justify-content: space-between; }
  .t { font-size: 26px; font-weight: 600; color: var(--accent); padding: 0; font-variant-numeric: tabular-nums; }
  .t:hover { background: none; color: #ffa47c; }
  .t.brk { color: var(--green); }
  .big .t { font-size: 48px; }
  .ctl button { font-size: 13px; padding: 2px 7px; margin-left: 3px; }
  .sub { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .target { color: var(--text); }
  .pick { margin-top: 6px; display: flex; flex-direction: column; background: var(--bg); border: 1px solid var(--line-2); border-radius: 6px; padding: 4px; max-height: 220px; overflow: auto; }
  .pick .faint { padding: 2px 4px; font-size: 10.5px; }
  .row { text-align: left; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
