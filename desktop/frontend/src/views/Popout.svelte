<script>
  import { projectOf } from '../lib/store.svelte.js'
  import { shell, conn } from '../lib/api.js'
  import { route } from '../lib/route.svelte.js'
  import WidgetBody from '../components/WidgetBody.svelte'
  import ItemDrawer from '../components/ItemDrawer.svelte'

  // One widget filling a small window of its own.
  let { type, projectId = null } = $props()

  const NAMES = {
    today: 'Today', week: 'This week', month: 'This month', anytime: 'Anytime', habits: 'Habits',
    timer: 'Timer', inprogress: 'In progress', inbox: 'Inbox', projects: 'Projects',
  }
  const widget = $derived({ id: 'pop', type, project_id: projectId })
  const name = $derived(type === 'project' ? projectOf(projectId)?.name || 'Project' : NAMES[type] || type)
  let drawer = $state(null)
  let onTop = $state(route.query.get('top') !== '0')
</script>

<div class="pop">
  <div class="hd drag">
    <span>{name}</span>
    <span class="nodrag">
      {#if conn().shell}
        <button class="ghost" class:on={onTop} title="Always on top" onclick={() => { onTop = !onTop; shell('top', { on: onTop }) }}>📌</button>
        <button class="ghost" title="Close" onclick={() => shell('close')}>✕</button>
      {/if}
    </span>
  </div>
  <div class="body">
    <WidgetBody
      {widget}
      onproject={id => shell('open', { kind: 'dashboard', project_id: id })}
      onitem={it => (drawer = it.id)}
    />
  </div>
</div>
{#if drawer}<ItemDrawer id={drawer} onclose={() => (drawer = null)} />{/if}

<style>
  .pop { height: 100vh; display: flex; flex-direction: column; background: var(--panel-2); border: 1px solid var(--line-2); }
  .hd { display: flex; justify-content: space-between; align-items: center; padding: 3px 4px 3px 10px; color: var(--muted); font-size: 10.5px; text-transform: uppercase; letter-spacing: .07em; border-bottom: 1px solid var(--line); }
  .hd button { padding: 0 5px; opacity: .5; }
  .hd button.on, .hd button:hover { opacity: 1; }
  .body { flex: 1; overflow: auto; padding: 8px 10px; }
</style>
