<script>
  import { db, projectOf, openItems } from '../lib/store.svelte.js'
  import { timeOf } from '../lib/format.js'
  import Timer from './Timer.svelte'
  import Habits from './Habits.svelte'
  import TodoList from './TodoList.svelte'
  import OsrsView from './OsrsView.svelte'

  // One widget's contents. onproject opens a project page; onitem opens a ticket.
  let { widget, onproject = () => {}, onitem = null } = $props()

  const inbox = $derived(db.items.filter(i => i.status === 'inbox'))
  const working = $derived(db.items.filter(i => i.status === 'in_progress' || i.status === 'review'))
  const live = $derived(working.filter(i => i.status === 'in_progress' && i.session).length)
  const proj = $derived(widget.project_id ? projectOf(widget.project_id) : null)
  const count = (pid, st) => db.items.filter(i => i.project_id === pid && i.status === st).length
  const upNext = $derived(
    proj ? db.items.filter(i => i.project_id === proj.id && i.status === 'up_next').sort((a, b) => a.position - b.position) : [],
  )
  const byProject = $derived(
    Object.entries(Object.groupBy(inbox, i => i.project_id ?? 0)).map(([pid, list]) => ({ pid: +pid, list })),
  )
</script>

{#if widget.type === 'today'}
  <TodoList when="today" onopen={onitem} />
{:else if widget.type === 'week'}
  <TodoList when="week" onopen={onitem} />
{:else if widget.type === 'month'}
  <TodoList when="month" onopen={onitem} />
{:else if widget.type === 'anytime'}
  <TodoList when="anytime" onopen={onitem} />
{:else if widget.type === 'habits'}
  <Habits title={false} />
{:else if widget.type === 'osrs'}
  <OsrsView onopen={onitem} />
{:else if widget.type === 'timer'}
  <Timer big />
{:else if widget.type === 'inbox'}
  {#if !inbox.length}
    <div class="faint">Inbox is empty.</div>
  {/if}
  {#each byProject as g (g.pid)}
    <button class="ghost line" onclick={() => g.pid && onproject(g.pid)}>
      <span>{projectOf(g.pid)?.name || 'Personal'}</span>
      <span class="muted">{g.list.length} waiting{g.pid ? ' · Triage ▸' : ''}</span>
    </button>
  {/each}
{:else if widget.type === 'inprogress'}
  <div class="muted small">{live} Claude session{live === 1 ? '' : 's'} live</div>
  {#each working as it (it.id)}
    <button class="ghost card" onclick={() => onitem?.(it)}>
      <div>
        {#if it.status === 'review'}<span class="tag idea">review</span>{/if}
        <span class="tag {it.kind}">{it.kind}</span>{it.title}
        <span class="faint">· {projectOf(it.project_id)?.slug || 'personal'}</span>
      </div>
      {#if it.branch}<div class="meta">🌿 {it.branch}{it.worktree ? ' · ' + it.worktree : ''}</div>{/if}
      {#if it.session}
        <div class="meta claude">◆ {it.session}{it.session_model ? ' · ' + it.session_model : ''}{it.session_started_at ? ' · started ' + timeOf(it.session_started_at) : ''}</div>
      {/if}
    </button>
  {/each}
  {#if !working.length}<div class="faint">Nothing in progress.</div>{/if}
{:else if widget.type === 'projects'}
  {#each db.projects.filter(p => !p.archived) as p (p.id)}
    <button class="ghost line" onclick={() => onproject(p.id)}>
      <span>{p.name}</span>
      <span class="muted">{openItems(p.id).length} open · {count(p.id, 'in_progress')} in progress{count(p.id, 'review') ? ` · ${count(p.id, 'review')} review` : ''}</span>
    </button>
  {/each}
  {#if !db.projects.length}<div class="faint">No projects yet. Use “+ Project” in the top bar.</div>{/if}
{:else if widget.type === 'project'}
  {#if proj}
    <button class="ghost line head" onclick={() => onproject(proj.id)}><b>{proj.name}</b><span class="muted">Open ▸</span></button>
    <div class="counts muted">
      {count(proj.id, 'inbox')} inbox · {count(proj.id, 'backlog')} backlog · {count(proj.id, 'up_next')} up next · {count(proj.id, 'in_progress')} in progress · {count(proj.id, 'review')} review
    </div>
    {#each upNext.slice(0, 4) as it, i (it.id)}
      <button class="ghost card" onclick={() => onitem?.(it)}>{i + 1}. {it.title}</button>
    {/each}
  {:else}
    <div class="faint">This project no longer exists.</div>
  {/if}
{/if}

<style>
  .line { display: flex; width: 100%; justify-content: space-between; gap: 8px; text-align: left; color: var(--text); padding: 4px 2px; border-bottom: 1px solid var(--line); border-radius: 0; }
  .line span:last-child { white-space: nowrap; }
  .head { border: 0; }
  .card { display: block; width: 100%; text-align: left; color: var(--text); background: var(--card); border: 1px solid var(--btn-hover); border-radius: 5px; padding: 5px 7px; margin-bottom: 5px; }
  .card:hover { background: var(--card); }
  .meta { color: var(--muted); font-size: 10.5px; margin-top: 2px; }
  .claude { color: var(--purple); }
  .small { font-size: 11px; margin-bottom: 4px; }
  .counts { font-size: 11px; margin-bottom: 6px; }
</style>
