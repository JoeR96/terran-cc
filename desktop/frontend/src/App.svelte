<script>
  import { db } from './lib/store.svelte.js'
  import { route } from './lib/route.svelte.js'
  import Side from './views/Side.svelte'
  import Board from './views/Board.svelte'
  import Project from './views/Project.svelte'
  import Popout from './views/Popout.svelte'

  let { startError = '' } = $props()
  const [view, arg] = $derived([route.parts[0] || 'board', route.parts[1]])
  let lastBoard = $state('main')
  $effect(() => {
    if (view === 'board' && arg) lastBoard = arg
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

{#if db.ready && !db.online}
  <div class="offline">Lost the connection to tccd. Reconnecting…</div>
{/if}
{#if db.error}
  <div class="toast">{db.error}</div>
{/if}

<style>
  .pad { padding: 24px; }
</style>
