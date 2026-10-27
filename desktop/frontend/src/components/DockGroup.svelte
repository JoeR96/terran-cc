<script>
  import { dnd } from '../lib/dockdrag.svelte.js'
  import { LABELS } from '../lib/dock.js'

  // One tab group: draggable tabs on top, the active panel below. Drop a
  // tab on the tab bar to move it here, or on the lower part of the panel
  // to stack it underneath.
  let { group, panel, onmove, onactivate } = $props()

  let barOver = $state(-1) // insert index while a tab hovers over the bar
  let belowOver = $state(false)

  function overBar(e) {
    if (!dnd.panel) return
    e.preventDefault()
    const tabs = [...e.currentTarget.querySelectorAll('.tab')]
    const i = tabs.findIndex(t => {
      const r = t.getBoundingClientRect()
      return e.clientX < r.left + r.width / 2
    })
    barOver = i < 0 ? tabs.length : i
  }
  function dropBar(e) {
    e.preventDefault()
    const p = dnd.panel
    const at = barOver
    barOver = -1
    dnd.panel = null
    if (p) onmove(p, { group: group.id, at })
  }
  function dropBelow(e) {
    e.preventDefault()
    const p = dnd.panel
    belowOver = false
    dnd.panel = null
    // Stacking a group's only tab under itself would change nothing.
    if (p && !(group.panels.length === 1 && group.panels[0] === p)) onmove(p, { below: group.id })
  }
</script>

<div class="grp">
  <div class="bar" role="tablist" tabindex="-1" ondragover={overBar} ondragleave={() => (barOver = -1)} ondrop={dropBar}>
    {#each group.panels as p, i (p)}
      {#if barOver === i}<span class="caret"></span>{/if}
      <button
        class="tab"
        class:on={group.active === p}
        class:moving={dnd.panel === p}
        role="tab"
        aria-selected={group.active === p}
        draggable="true"
        title="Drag to move this panel"
        onclick={() => onactivate(group.id, p)}
        ondragstart={e => {
          dnd.panel = p
          e.dataTransfer.effectAllowed = 'move'
          e.dataTransfer.setData('text/plain', p)
        }}
        ondragend={() => ((dnd.panel = null), (barOver = -1), (belowOver = false))}>{LABELS[p]}</button>
    {/each}
    {#if barOver === group.panels.length}<span class="caret"></span>{/if}
    <span class="fill"></span>
  </div>
  <div class="body">
    {@render panel(group.active)}
    {#if dnd.panel}
      <div
        class="below"
        class:over={belowOver}
        role="region"
        aria-label="Stack here"
        ondragover={e => ((belowOver = true), e.preventDefault())}
        ondragleave={() => (belowOver = false)}
        ondrop={dropBelow}
      >
        <span>Stack here</span>
      </div>
    {/if}
  </div>
</div>

<style>
  .grp { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); border-radius: 7px; overflow: hidden; }
  .bar { display: flex; align-items: stretch; gap: 1px; flex: none; background: var(--panel-2); border-bottom: 1px solid var(--line); min-height: 24px; }
  .tab { background: none; border-radius: 0; padding: 3px 12px; font-size: 10.5px; letter-spacing: .06em; text-transform: uppercase; font-weight: 600; color: var(--muted); cursor: grab; border-bottom: 2px solid transparent; }
  .tab:hover { color: var(--text); background: var(--btn); }
  .tab.on { color: var(--text); border-bottom-color: var(--accent); background: var(--bg); }
  .tab.moving { opacity: .4; }
  .caret { width: 2px; background: var(--accent); margin: 3px 0; }
  .fill { flex: 1; }
  .body { position: relative; flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .below { position: absolute; left: 0; right: 0; bottom: 0; height: 40%; display: flex; align-items: center; justify-content: center; border: 2px dashed var(--line-2); border-radius: 6px; background: rgba(12, 12, 12, .55); color: var(--muted); font-size: 11px; z-index: 5; }
  .below.over { border-color: var(--accent); color: var(--accent); background: var(--accent-fill); }
</style>
