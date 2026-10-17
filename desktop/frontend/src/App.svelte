<script>
  import { db } from './lib/store.svelte.js'
  import { route } from './lib/route.svelte.js'
  import { shell, conn } from './lib/api.js'
  import Side from './views/Side.svelte'
  import Board from './views/Board.svelte'
  import Project from './views/Project.svelte'
  import Popout from './views/Popout.svelte'
  import Chime from './components/Chime.svelte'

  let { startError = '' } = $props()
  const [view, arg] = $derived([route.parts[0] || 'board', route.parts[1]])
  let lastBoard = $state(route.query.get('board') || 'main')
  $effect(() => {
    if (view === 'board' && arg) lastBoard = arg
  })
  // Tell the shell where a dashboard window is, so it reopens there.
  $effect(() => {
    if (!conn()?.shell || (view !== 'board' && view !== 'project')) return
    shell('state', view === 'board' ? { board_id: arg || 'main', project_id: 0 } : { board_id: lastBoard, project_id: +arg })
  })
</script>

{#if startError}
  <div class="pad">
    <h3>Terran CC can't reach its service</h3>
    <p class="muted">{startError}</p>
  </div>
{:else if !db.ready}
  <div class="pad faint">{db.online ? 'Loading…' : 'Connecting to tccd…'}</div>
{:else if view === 'side'}
  <Side />
{:else if view === 'project'}
  <Project projectId={+arg} boardId={route.query.get('board') || lastBoard} />
{:else if view === 'widget'}
  <Popout type={arg} projectId={route.query.get('project') ? +route.query.get('project') : null} />
{:else}
  <Board boardId={arg || 'main'} />
{/if}

<!-- In the desktop app the shell plays the chime once for all windows. -->
{#if !conn()?.shell}<Chime />{/if}

{#if db.ready && !db.online}
  <div class="offline">Lost the connection to tccd. Reconnecting…</div>
{/if}
{#if db.error}
  <div class="toast">{db.error}</div>
{/if}

<style>
  .pad { padding: 24px; }
</style>
