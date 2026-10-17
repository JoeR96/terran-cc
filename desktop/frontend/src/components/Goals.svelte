<script>
  import { db, act, projectOf } from '../lib/store.svelte.js'
  import { goals } from '../lib/lists.js'

  // Project tickets set for today, this week or this month.
  let { onopen = null } = $props()

  const rows = $derived(goals(db.items))
  const LABEL = { today: 'today', week: 'week', month: 'month' }
</script>

<h6>Goals <span>{rows.filter(i => i.status !== 'done').length}</span></h6>
{#each rows as it (it.id)}
  <div class="g" class:done={it.status === 'done'}>
    <span class="chip {it.when}">{LABEL[it.when]}</span>
    <button class="box" class:on={it.status === 'done'} aria-label="Done" title={it.status === 'done' ? 'Reopen' : 'Done'} onclick={() => (it.status === 'done' ? act.reopen(it.id) : act.done(it.id))}></button>
    <span class="title" role="button" tabindex="0" title={it.title} onclick={() => onopen?.(it)} onkeydown={() => {}}>{it.title}</span>
    <span class="faint slug">{projectOf(it.project_id)?.slug}</span>
    <button class="ghost x" title="Remove from goals" onclick={() => act.edit(it.id, { when: 'anytime' })}>✕</button>
  </div>
{:else}
  <div class="faint empty">Set a ticket's T / W / M in the tree below to make it a goal.</div>
{/each}

<style>
  .g { display: flex; gap: 6px; align-items: center; min-height: 21px; }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: pointer; }
  .title:hover { color: var(--strong); }
  .done .title { text-decoration: line-through; color: var(--faint); }
  .slug { font-size: 10.5px; }
  .x { display: none; padding: 0 3px; font-size: 11px; }
  .g:hover .x { display: inline; }
  .chip { flex: none; width: 40px; text-align: center; font-size: 9.5px; border: 1px solid var(--line-2); color: var(--muted); border-radius: 8px; line-height: 14px; }
  .chip.today { border-color: var(--accent); color: var(--accent); }
  .chip.week { border-color: var(--blue); color: var(--blue); }
  .empty { font-size: 11px; padding: 2px 0; }
</style>
