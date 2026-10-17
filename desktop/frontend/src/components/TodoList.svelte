<script>
  import { db, act, bucket, projectOf } from '../lib/store.svelte.js'
  import { focus } from '../lib/actions.js'

  // when: today | week | month | anytime. limit caps rows (side widget).
  let { when, title = '', limit = 0, onopen = null } = $props()

  const all = $derived(bucket(when))
  const doneCount = $derived(all.filter(i => i.status === 'done').length)
  const rows = $derived(limit && all.length > limit ? all.slice(0, limit - 1) : all)
  const more = $derived(all.length - rows.length)

  let adding = $state(false)
  let draft = $state('')
  let work = $state(false)
  let editing = $state(null)
  let editDraft = $state('')

  async function add() {
    const t = draft.trim()
    if (!t) {
      adding = false
      return
    }
    draft = ''
    await act.addItem({ title: t, when, context: work ? 'work' : 'irl', kind: 'todo' })
  }
  function toggle(it) {
    it.status === 'done' ? act.reopen(it.id) : act.done(it.id)
  }
  function saveTitle(it) {
    const t = editDraft.trim()
    editing = null
    if (t && t !== it.title) act.edit(it.id, { title: t })
  }
</script>

{#if title}
  <h6>{title} <span>{doneCount} / {all.length}</span></h6>
{/if}
{#each rows as it (it.id)}
  <div class="td" class:done={it.status === 'done'}>
    <button class="box" class:on={it.status === 'done'} aria-label="Done" title={it.status === 'done' ? 'Reopen' : 'Done'} onclick={() => toggle(it)}></button>
    {#if editing === it.id}
      <input
        class="edit"
        bind:value={editDraft}
        onblur={() => saveTitle(it)}
        onkeydown={e => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') editing = null
        }}
        use:focus
      />
    {:else}
      <span
        class="title"
        role="button"
        tabindex="0"
        title="Click to rename"
        onclick={() => {
          editing = it.id
          editDraft = it.title
        }}
        onkeydown={() => {}}
      >
        {#if it.project_id}<span class="tag proj">{projectOf(it.project_id)?.slug}</span>{/if}{it.title}
      </span>
      <span class="acts">
        {#if it.context === 'work'}<span class="ctx" title="Work">W</span>{/if}
        {#if it.status !== 'done'}
          <button class="ghost" title="Focus the timer on this" onclick={() => act.timer('start', { item_id: it.id })}>◷</button>
        {/if}
        {#if onopen}<button class="ghost" title="Open" onclick={() => onopen(it)}>↗</button>{/if}
        <button class="ghost" title="Delete" onclick={() => act.remove(it.id)}>✕</button>
      </span>
    {/if}
  </div>
{/each}
{#if more > 0}
  <div class="faint more">+{more} more</div>
{/if}
{#if adding}
  <div class="addrow">
    <input
      placeholder="New to-do, Enter to add"
      bind:value={draft}
      onkeydown={e => {
        if (e.key === 'Enter') add()
        if (e.key === 'Escape') {
          adding = false
          draft = ''
        }
      }}
      onblur={() => {
        if (!draft.trim()) adding = false
      }}
      use:focus
    />
    <button class="ghost ctxbtn" class:on={work} title="Work or IRL" onmousedown={e => e.preventDefault()} onclick={() => (work = !work)}>{work ? 'work' : 'irl'}</button>
  </div>
{:else}
  <button class="ghost add" onclick={() => (adding = true)}>+ add</button>
{/if}

<style>
  .td { display: flex; gap: 7px; align-items: center; min-height: 21px; }
  .title { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; cursor: text; }
  .done .title { text-decoration: line-through; color: var(--faint); }
  .acts { display: none; gap: 1px; align-items: center; }
  .td:hover .acts { display: flex; }
  .acts button { padding: 0 3px; font-size: 11px; }
  .ctx { font-size: 9px; color: #9cc4ff; border: 1px solid #23334a; border-radius: 3px; padding: 0 3px; }
  .edit { flex: 1; min-width: 0; padding: 0 4px; }
  .add { padding: 2px 0; color: var(--faint); }
  .add:hover { background: none; color: var(--text); }
  .more { padding: 1px 0 1px 19px; }
  .addrow { display: flex; gap: 4px; margin-top: 3px; }
  .addrow input { flex: 1; min-width: 0; }
  .ctxbtn { font-size: 10px; }
  .ctxbtn.on { color: #9cc4ff; }
</style>
