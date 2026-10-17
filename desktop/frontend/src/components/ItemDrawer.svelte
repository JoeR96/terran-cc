<script>
  import { db, act, fail, projectOf } from '../lib/store.svelte.js'
  import { upload, fileURL } from '../lib/api.js'
  import { timeOf } from '../lib/format.js'

  let { id, onclose } = $props()

  const it = $derived(db.items.find(i => i.id === id))
  const STATUSES = [
    ['inbox', 'Inbox'], ['backlog', 'Backlog'], ['up_next', 'Up next'], ['in_progress', 'In progress'],
    ['review', 'Review'], ['done', 'Done'], ['someday', 'Someday'],
  ]
  const KINDS = ['todo', 'bug', 'task', 'feature', 'idea']
  const WHENS = [['', '—'], ['today', 'Today'], ['week', 'This week'], ['month', 'This month'], ['anytime', 'Anytime']]

  let more = $state(false)
  let noteDraft = $state('')
  let notesEdit = $state(null)

  const set = (field, value) => act.edit(id, { [field]: value })
  const setText = (field, e) => {
    const v = e.currentTarget.value
    if (v !== (it[field] ?? '')) set(field, v)
  }
  const setInt = (field, e) => {
    const v = parseInt(e.currentTarget.value, 10)
    if (Number.isFinite(v) && v !== it[field]) set(field, v)
  }
  function setDeps(e) {
    const deps = e.currentTarget.value.split(/[ ,]+/).map(s => parseInt(s.replace('#', ''), 10)).filter(Number.isFinite)
    act.edit(id, { depends_on: deps })
  }
  async function addNote() {
    const t = noteDraft.trim()
    if (!t) return
    noteDraft = ''
    await act.note(id, t)
  }
  async function saveNotes() {
    if (notesEdit !== null && notesEdit !== it.notes) await set('notes', notesEdit)
    notesEdit = null
  }
  async function onPaste(e) {
    const files = [...(e.clipboardData?.files || [])].filter(f => f.type.startsWith('image/'))
    if (!files.length) return
    e.preventDefault()
    for (const f of files) await upload(id, f).catch(fail)
  }
  async function onPick(e) {
    for (const f of e.currentTarget.files) await upload(id, f).catch(fail)
    e.currentTarget.value = ''
  }
  function del() {
    if (confirm(`Delete #${id} “${it.title}”?`)) {
      act.remove(id)
      onclose()
    }
  }
  const isImg = n => /\.(png|jpe?g|gif|webp|bmp)$/i.test(n)
</script>

<svelte:window onkeydown={e => e.key === 'Escape' && notesEdit === null && onclose()} />

<div class="scrim" role="presentation" onclick={onclose}></div>
<aside onpaste={onPaste}>
  {#if !it}
    <div class="faint">This item was deleted.</div>
    <button onclick={onclose}>Close</button>
  {:else}
    <div class="top">
      <span class="faint">#{it.id}{it.project_id ? ' · ' + (projectOf(it.project_id)?.name || '') : ' · personal'}</span>
      <button class="ghost" onclick={onclose}>✕</button>
    </div>
    <input class="title" value={it.title} onchange={e => setText('title', e)} />

    <div class="fields">
      <label>Status
        <select value={it.status} onchange={e => set('status', e.currentTarget.value)}>
          {#each STATUSES as [v, n] (v)}<option value={v}>{n}</option>{/each}
        </select>
      </label>
      <label>Kind
        <select value={it.kind} onchange={e => set('kind', e.currentTarget.value)}>
          {#each KINDS as k (k)}<option value={k}>{k}</option>{/each}
        </select>
      </label>
      <label>When
        <select value={it.when || ''} onchange={e => set('when', e.currentTarget.value || 'anytime')}>
          {#each WHENS as [v, n] (v)}<option value={v}>{n}</option>{/each}
        </select>
      </label>
      <label>Context
        <select value={it.context} onchange={e => set('context', e.currentTarget.value)}>
          <option value="irl">irl</option><option value="work">work</option>
        </select>
      </label>
      <label>Project
        <select value={it.project_id ?? ''} onchange={e => act.edit(id, { project_id: e.currentTarget.value ? +e.currentTarget.value : null })}>
          <option value="">Personal</option>
          {#each db.projects as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
        </select>
      </label>
      {#if it.project_id}
        <label>Priority <input type="number" value={it.priority} onchange={e => setInt('priority', e)} /></label>
        {#if it.status === 'up_next'}
          <label>Position <input type="number" value={it.position} onchange={e => setInt('position', e)} /></label>
        {/if}
      {/if}
    </div>

    {#if it.project_id}
      <label class="wide">Why first <input value={it.why_first} onchange={e => setText('why_first', e)} /></label>
      <label class="wide">Depends on <input placeholder="#3, #7" value={(it.depends_on || []).map(d => '#' + d).join(', ')} onchange={setDeps} /></label>
    {/if}

    <div class="sect">
      <h6>Notes {#if notesEdit === null}<button class="ghost" onclick={() => (notesEdit = it.notes || '')}>edit</button>{/if}</h6>
      {#if notesEdit !== null}
        <textarea rows="8" bind:value={notesEdit} onblur={saveNotes}></textarea>
      {:else if it.notes}
        <pre class="notes">{it.notes}</pre>
      {:else}
        <div class="faint">No notes.</div>
      {/if}
      <div class="row">
        <input placeholder="Add a timestamped note" bind:value={noteDraft} onkeydown={e => e.key === 'Enter' && addNote()} />
      </div>
    </div>

    <div class="sect">
      <h6>Attachments <label class="pick">+ file<input type="file" accept="image/*" multiple onchange={onPick} hidden /></label></h6>
      <div class="faint small">Paste a screenshot anywhere in this panel to attach it.</div>
      <div class="att">
        {#each it.attachments || [] as a (a)}
          <a href={fileURL(it.id, a.split('/').pop())} target="_blank" title={a}>
            {#if isImg(a)}<img src={fileURL(it.id, a.split('/').pop())} alt={a} />{:else}{a.split('/').pop()}{/if}
          </a>
        {/each}
      </div>
    </div>

    {#if (it.evidence || []).length}
      <div class="sect">
        <h6>Evidence</h6>
        {#each it.evidence as ev, i (i)}
          <div class="ev"><span class="tag task">{ev.type}</span><code>{ev.value}</code></div>
        {/each}
      </div>
    {/if}

    {#if it.branch || it.worktree || it.session}
      <div class="sect">
        <h6>Claude</h6>
        {#if it.branch}<div>🌿 {it.branch}</div>{/if}
        {#if it.worktree}<div class="muted">{it.worktree}</div>{/if}
        {#if it.session}<div class="claude">◆ {it.session}{it.session_model ? ' · ' + it.session_model : ''}{it.session_started_at ? ' · ' + timeOf(it.session_started_at) : ''}</div>{/if}
      </div>
    {/if}

    <button class="ghost" onclick={() => (more = !more)}>{more ? '▾' : '▸'} more</button>
    {#if more}
      <div class="fields">
        <label>Branch <input value={it.branch} onchange={e => setText('branch', e)} /></label>
        <label>Worktree <input value={it.worktree} onchange={e => setText('worktree', e)} /></label>
        <label>Model <input value={it.model} onchange={e => setText('model', e)} /></label>
        <label>Effort <input value={it.effort} onchange={e => setText('effort', e)} /></label>
      </div>
    {/if}

    <div class="actions">
      {#if it.status === 'done'}
        <button onclick={() => act.reopen(id)}>Reopen</button>
      {:else}
        <button class="primary" onclick={() => act.done(id)}>{it.status === 'review' ? 'Sign off · Done' : 'Done'}</button>
      {/if}
      <span style="flex:1"></span>
      <button class="ghost danger" onclick={del}>Delete</button>
    </div>
  {/if}
</aside>

<style>
  .scrim { position: fixed; inset: 0; background: rgba(0, 0, 0, .35); z-index: 40; }
  aside { position: fixed; top: 0; right: 0; bottom: 0; width: min(440px, 100vw); z-index: 41; background: var(--panel); border-left: 1px solid var(--line-2); padding: 10px 14px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; box-shadow: -10px 0 30px rgba(0, 0, 0, .4); user-select: text; }
  .top { display: flex; justify-content: space-between; align-items: center; }
  .title { font-size: 15px; font-weight: 600; color: var(--strong); background: none; border-color: transparent; padding: 2px 4px; }
  .title:hover { border-color: var(--line-2); }
  .fields { display: grid; grid-template-columns: 1fr 1fr; gap: 6px 10px; }
  label { display: flex; flex-direction: column; gap: 2px; color: var(--muted); font-size: 10.5px; }
  label.wide input { width: 100%; }
  .sect { border-top: 1px solid var(--line); padding-top: 8px; }
  .notes { white-space: pre-wrap; font: inherit; margin: 0 0 6px; max-height: 260px; overflow: auto; }
  .row input { width: 100%; }
  .pick { cursor: pointer; color: var(--muted); text-transform: none; letter-spacing: 0; font-size: 11px; display: inline; }
  .small { font-size: 10.5px; }
  .att { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 6px; }
  .att img { max-width: 120px; max-height: 90px; border-radius: 4px; border: 1px solid var(--line-2); }
  .att a { color: var(--text); }
  .ev { display: flex; gap: 4px; align-items: center; margin-bottom: 2px; }
  code { color: var(--muted); font-size: 11px; }
  .claude { color: var(--purple); }
  .actions { display: flex; gap: 6px; border-top: 1px solid var(--line); padding-top: 8px; margin-top: auto; }
  .danger { color: var(--red); }
</style>
