<script>
  import Split from './Split.svelte'
  import DockGroup from './DockGroup.svelte'

  // One column: its tab groups stacked, with dividers between them.
  let { col, panel, onmove, onactivate, onresize } = $props()
</script>

{#snippet group(gi)}
  {@const g = col.groups[gi]}
  {#if g}{#key g.id}<DockGroup group={g} {panel} {onmove} {onactivate} />{/key}{/if}
{/snippet}

<Split dir="col" count={col.groups.length} value={col.groups.map(g => g.size)} defaults={col.groups.map(() => 100 / col.groups.length)} onchange={onresize} render={group} />
