<script>
  import { drag, fit } from '../lib/split.js'

  // Panes side by side (dir="row") or stacked (dir="col") with dividers to
  // drag. Sizes are kept per window under key, or, given value, belong to
  // the caller, who hears about changes through onchange. Double-click a
  // divider to go back to the defaults.
  // Panes come either as a list of snippets (panes) or as a count with one
  // snippet that takes the pane's index (render).
  let { key = '', dir = 'row', panes = null, count = 0, render = null, defaults = null, min = 12, value = null, onchange = null } = $props()
  const n = () => (panes ? panes.length : count)

  const storeKey = () => `tcc.split.${key}`
  const initial = () => {
    try {
      return fit(JSON.parse(localStorage.getItem(storeKey())) ?? defaults, n())
    } catch {
      return fit(defaults, n())
    }
  }
  let own = $state(value ? fit(value, n()) : initial())
  // Controlled: follow value except mid-drag.
  $effect(() => {
    if (value && dragging < 0) own = fit(value, n())
  })
  const sizes = $derived(own)
  let box
  let dragging = $state(-1)

  // The pane count can change (the Claude box folding away): refit.
  $effect(() => {
    if (own.length !== n()) own = fit(value || defaults, n())
  })

  function save() {
    if (onchange) return onchange([...own])
    try {
      localStorage.setItem(storeKey(), JSON.stringify(own))
    } catch {}
  }

  function down(e, i) {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    const rect = box.getBoundingClientRect()
    const total = dir === 'row' ? rect.width : rect.height
    const start = dir === 'row' ? e.clientX : e.clientY
    const from = [...own]
    dragging = i
    const move = ev => {
      const now = dir === 'row' ? ev.clientX : ev.clientY
      own = drag(from, i, ((now - start) / total) * 100, min)
    }
    const up = () => {
      dragging = -1
      e.target.removeEventListener('pointermove', move)
      e.target.removeEventListener('pointerup', up)
      save()
    }
    e.target.addEventListener('pointermove', move)
    e.target.addEventListener('pointerup', up)
  }
  function reset() {
    own = fit(defaults, n())
    save()
  }
</script>

<div class="split {dir}" class:busy={dragging >= 0} bind:this={box}>
  {#each Array(n()) as _, i (i)}
    <div class="pane" style="flex: {sizes[i] ?? 0} 1 0">{#if panes}{@render panes[i]()}{:else}{@render render(i)}{/if}</div>
    {#if i < n() - 1}
      <div
        class="gutter"
        class:on={dragging === i}
        role="separator"
        aria-orientation={dir === 'row' ? 'vertical' : 'horizontal'}
        title="Drag to resize · double-click to reset"
        onpointerdown={e => down(e, i)}
        ondblclick={reset}
      ></div>
    {/if}
  {/each}
</div>

<style>
  .split { display: flex; min-width: 0; min-height: 0; flex: 1; }
  .split.row { flex-direction: row; }
  .split.col { flex-direction: column; }
  .pane { min-width: 0; min-height: 0; display: flex; flex-direction: column; overflow: hidden; }
  .gutter { flex: none; position: relative; z-index: 2; }
  .row > .gutter { width: 8px; cursor: col-resize; }
  .col > .gutter { height: 8px; cursor: row-resize; }
  .gutter::after { content: ''; position: absolute; background: var(--line); border-radius: 2px; transition: background .15s; }
  .row > .gutter::after { left: 3px; width: 2px; top: 30%; bottom: 30%; }
  .col > .gutter::after { top: 3px; height: 2px; left: 30%; right: 30%; }
  .gutter:hover::after, .gutter.on::after { background: var(--accent); }
  .row > .gutter:hover::after, .row > .gutter.on::after { top: 0; bottom: 0; }
  .col > .gutter:hover::after, .col > .gutter.on::after { left: 0; right: 0; }
  /* Iframes and terminals swallow pointer events mid-drag; this stops it. */
  .busy :global(*) { user-select: none; }
  .busy .pane { pointer-events: none; }
</style>
