<script>
  import Split from './Split.svelte'
  import DockColumn from './DockColumn.svelte'
  import { dnd } from '../lib/dockdrag.svelte.js'
  import { move, activate, resize } from '../lib/dock.js'

  // Columns of tab groups. Every change goes back through onchange, which
  // saves the layout.
  let { layout, panel, onchange } = $props()

  let edgeOver = $state(false)
  const doMove = (p, to) => onchange(move(layout, p, to))
  const doActivate = (g, p) => onchange(activate(layout, g, p))
</script>

{#snippet column(ci)}
  {@const col = layout.cols[ci]}
  {#if col}
    <DockColumn {col} {panel} onmove={doMove} onactivate={doActivate} onresize={s => onchange(resize(layout, ci, s))} />
  {/if}
{/snippet}

<div class="dock">
  <Split count={layout.cols.length} value={layout.cols.map(c => c.size)} defaults={layout.cols.map(() => 100 / layout.cols.length)} onchange={s => onchange(resize(layout, null, s))} render={column} />
  {#if dnd.panel}
    <div
      class="edge"
      class:over={edgeOver}
      role="region"
      aria-label="New column"
      ondragover={e => ((edgeOver = true), e.preventDefault())}
      ondragleave={() => (edgeOver = false)}
      ondrop={e => {
        e.preventDefault()
        const p = dnd.panel
        edgeOver = false
        dnd.panel = null
        if (p) doMove(p, { newCol: layout.cols.length })
      }}
    >
      <span>New column</span>
    </div>
  {/if}
</div>

<style>
  .dock { position: relative; flex: 1; min-height: 0; display: flex; padding: 6px 6px 8px; }
  .edge { position: absolute; top: 6px; right: 6px; bottom: 8px; width: 70px; display: flex; align-items: center; justify-content: center; border: 2px dashed var(--line-2); border-radius: 7px; background: rgba(12, 12, 12, .7); z-index: 6; }
  .edge span { writing-mode: vertical-rl; font-size: 11px; color: var(--muted); }
  .edge.over { border-color: var(--accent); background: var(--accent-fill); }
  .edge.over span { color: var(--accent); }
</style>
