<script>
  import { db, act } from '../lib/store.svelte.js'
  import { habitValue } from '../lib/format.js'
  import { select } from '../lib/actions.js'

  // wide: habits sit side by side as cards instead of one per line.
  let { bars = true, title = true, wide = false } = $props()

  let editing = $state(null)
  let draft = $state('')

  function startEdit(h) {
    editing = h.id
    draft = String(h.value)
  }
  function commit(h) {
    const v = Number(draft)
    editing = null
    if (draft.trim() !== '' && Number.isFinite(v) && v !== h.value) act.logHabit(h, { set: v })
  }
  const short = name => (name.startsWith('Creatine') ? 'Supps' : name === 'Standing desk' ? 'Standing' : name)
  const pct = h => Math.min(100, (h.value / (h.target || 1)) * 100)
</script>

{#if title}
  <h6>Habits <span class="streak" title="Days in a row with every habit hit">🔥{db.streak}</span></h6>
{/if}
<div class="habs" class:wide>
{#each db.habits as h (h.id)}
  <div class="cell">
  <div class="h">
    <span class="name" title={h.name}>{h.icon} {short(h.name)}</span>
    {#if h.type === 'check'}
      <button class="box" class:on={h.value >= 1} title="Toggle" aria-label="Toggle {h.name}" onclick={() => act.logHabit(h, { toggle: true })}></button>
    {:else}
      <span class="p">
        {#if editing === h.id}
          <input
            class="num"
            bind:value={draft}
            onblur={() => commit(h)}
            onkeydown={e => {
              if (e.key === 'Enter') e.currentTarget.blur()
              if (e.key === 'Escape') editing = null
            }}
            use:select
          />
          <span class="faint">{h.unit}</span>
        {:else}
          <button class="ghost val" class:hit={h.hit} title="Click to type an amount" onclick={() => startEdit(h)}>{habitValue(h)}</button>
        {/if}
        <button class="mini" title="Add {h.step}{h.unit}" onclick={() => act.logHabit(h, { delta: h.step })}>+</button>
      </span>
    {/if}
  </div>
  {#if bars && h.type !== 'check'}
    <div class="bar"><i class:hit={h.hit} style="width:{pct(h)}%"></i></div>
  {:else if wide}
    <div class="bar"><i class:hit={h.hit} style="width:{h.value >= 1 ? 100 : 0}%"></i></div>
  {/if}
  </div>
{/each}
</div>

<style>
  .streak { color: var(--accent); letter-spacing: 0; text-transform: none; font-size: 11px; }
  .habs.wide { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; height: 100%; }
  /* Wide: each habit is a solid block whose background fills with progress. */
  .wide .cell { position: relative; overflow: hidden; background: var(--panel-2); border: 1px solid var(--line-2); border-radius: 6px; padding: 0 8px; display: flex; align-items: center; min-height: 44px; }
  .wide .h { position: relative; z-index: 1; flex: 1; min-width: 0; font-size: 12.5px; font-weight: 600; }
  .wide .name { flex: 1; min-width: 0; }
  .wide .p { flex: none; }
  .wide .val { font-size: 12.5px; padding: 0 2px; }
  .wide .mini { font-size: 13px; line-height: 20px; padding: 0 7px; }
  .wide .bar { position: absolute; inset: 0; height: auto; margin: 0; border-radius: 0; background: none; }
  .wide .bar i { border-radius: 0; background: var(--accent-fill); }
  .wide .bar i.hit { background: var(--green-fill); }
  .h { display: flex; justify-content: space-between; align-items: center; min-height: 22px; gap: 6px; }
  .name { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .p { display: flex; gap: 3px; align-items: center; }
  .val { color: var(--text); padding: 0 3px; font-variant-numeric: tabular-nums; }
  .val.hit { color: var(--green); }
  .mini { padding: 0 6px; font-size: 12px; line-height: 17px; }
  .num { width: 58px; padding: 0 4px; text-align: right; }
  .bar { height: 3px; background: var(--line); border-radius: 2px; margin: 0 0 3px; }
  .bar i { display: block; height: 100%; background: var(--accent); border-radius: 2px; transition: width .3s; }
  .bar i.hit { background: var(--green); }
</style>
