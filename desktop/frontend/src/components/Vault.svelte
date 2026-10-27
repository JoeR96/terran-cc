<script>
  import { onDestroy } from 'svelte'
  import { api, vaultFile } from '../lib/api.js'
  import { fail } from '../lib/store.svelte.js'
  // The editor (CodeMirror) is large, so it loads when a note first opens.
  const loadEditor = import('./NoteEditor.svelte')

  const saved = () => {
    try {
      return localStorage.getItem('tcc.side.vault.path') || ''
    } catch {
      return ''
    }
  }

  let entries = $state([])
  let open = $state({})
  let path = $state(saved())
  let text = $state('')
  let status = $state('')
  let err = $state('')
  let loaded = $state(false)
  let q = $state('')
  let dirty = false
  let timer

  const selected = $derived(entries.find(e => e.path === path))
  const files = $derived(entries.filter(e => !e.dir).map(e => e.path))
  const isImage = name => /\.(png|jpe?g|gif|webp|svg|bmp|avif)$/i.test(name || '')
  const image = $derived(!!selected && !selected.dir && isImage(selected.name))
  const note = $derived(!!selected && !selected.dir && selected.name.toLowerCase().endsWith('.md'))
  const hits = $derived(
    q.trim()
      ? entries.filter(e => !e.dir && e.name.toLowerCase().includes(q.trim().toLowerCase()))
      : null,
  )

  function parentOf(p) {
    const i = p.lastIndexOf('/')
    return i < 0 ? '' : p.slice(0, i)
  }
  function children(dir) {
    return entries
      .filter(e => parentOf(e.path) === dir)
      .sort((a, b) => (a.dir === b.dir ? a.name.localeCompare(b.name) : a.dir ? -1 : 1))
  }
  function expandTo(p) {
    const bits = p.split('/')
    let cur = ''
    for (let i = 0; i < bits.length - 1; i++) {
      cur = cur ? `${cur}/${bits[i]}` : bits[i]
      open[cur] = true
    }
  }
  function remember(p) {
    path = p
    try {
      localStorage.setItem('tcc.side.vault.path', p)
    } catch {}
  }
  function toggle(p) {
    open[p] = !open[p]
  }

  async function loadTree() {
    entries = await api('GET', '/api/vault/tree')
    err = ''
    loaded = true
  }
  async function loadNote(p) {
    const got = await api('GET', '/api/vault/note?path=' + encodeURIComponent(p) + '&full=1')
    if (path === p) text = got.text || ''
  }
  async function save() {
    clearTimeout(timer)
    if (!dirty || !note) return
    const v = text
    const p = path
    try {
      await api('PUT', '/api/vault/note?path=' + encodeURIComponent(p), { text: v })
      if (path === p) {
        dirty = false
        status = 'saved'
      }
    } catch (e) {
      status = 'not saved'
      fail(e)
    }
  }
  function changed() {
    dirty = true
    status = 'saving…'
    clearTimeout(timer)
    timer = setTimeout(save, 600)
  }
  async function pick(p) {
    if (dirty) await save()
    remember(p)
    text = ''
    status = ''
    const en = entries.find(e => e.path === p)
    if (!en || en.dir || !en.name.toLowerCase().endsWith('.md')) return
    try {
      await loadNote(p)
    } catch (e) {
      status = 'not loaded'
      fail(e)
    }
  }

  onDestroy(() => {
    if (dirty) save()
  })

  loadTree()
    .then(() => {
      const en = entries.find(e => e.path === path)
      if (en) {
        expandTo(path)
        if (!en.dir && en.name.toLowerCase().endsWith('.md')) return loadNote(path)
      }
    })
    .catch(e => {
      err = e.message
      fail(e)
    })
</script>

{#snippet tree(dir, depth)}
  {#each children(dir) as e (e.path)}
    <button
      class="ghost row"
      class:on={path === e.path}
      style="padding-left: {6 + depth * 12}px"
      onclick={() => (e.dir ? toggle(e.path) : pick(e.path))}
    >
      <span class="mark">{e.dir ? (open[e.path] ? '▾' : '▸') : ''}</span>
      <span class="name">{e.name}</span>
    </button>
    {#if e.dir && open[e.path]}
      {@render tree(e.path, depth + 1)}
    {/if}
  {/each}
{/snippet}

<div class="vault">
  <div class="tree">
    <input placeholder="Filter files" bind:value={q} />
    <div class="list">
      {#if hits}
        {#each hits as e (e.path)}
          <button class="ghost row" class:on={path === e.path} onclick={() => pick(e.path)}>
            <span class="name" title={e.path}>{e.name}</span>
          </button>
        {/each}
        {#if !hits.length}<div class="faint pad">No files match.</div>{/if}
      {:else}
        {@render tree('', 0)}
      {/if}
    </div>
  </div>
  <div class="view">
    {#if err}
      <div class="faint pad">{err}</div>
    {:else if !loaded}
      <div class="faint pad">Loading…</div>
    {:else if !entries.length}
      <div class="faint pad">The vault is empty.</div>
    {:else if !path}
      <div class="faint pad">Pick a note.</div>
    {:else if !selected}
      <div class="faint pad">That file is gone.</div>
    {:else if image}
      <div class="read"><img src={vaultFile(path)} alt={selected.name} /></div>
    {:else if note}
      <div class="bar">
        <span class="faint path" title={path}>{path}</span>
      </div>
      {#await loadEditor then { default: NoteEditor }}
        <NoteEditor
          {path}
          {text}
          {files}
          onchange={v => {
            text = v
            changed()
          }}
        />
      {/await}
      <div class="foot faint">{status}</div>
    {:else}
      <div class="faint pad">Can't open this file.</div>
    {/if}
  </div>
</div>

<style>
  .vault { display: flex; height: 100%; min-height: 0; }
  .tree { width: 34%; min-width: 120px; max-width: 280px; display: flex; flex-direction: column; border-right: 1px solid var(--line); padding: 6px; gap: 6px; }
  .list { flex: 1; min-height: 0; overflow: auto; }
  .row { display: flex; width: 100%; text-align: left; gap: 4px; padding: 1px 4px; border-radius: 3px; color: var(--text); }
  .row.on { background: var(--btn); }
  .mark { width: 10px; flex: none; color: var(--faint); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .view { flex: 1; min-width: 0; display: flex; flex-direction: column; padding: 6px 8px; }
  .bar { display: flex; align-items: center; gap: 6px; flex: none; margin-bottom: 4px; }
  .path { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .read { flex: 1; min-height: 0; overflow: auto; user-select: text; }
  img { display: block; max-width: 100%; margin: 6px 0; }
  .foot { font-size: 10px; text-align: right; min-height: 14px; flex: none; }
  .pad { padding: 8px 2px; }
</style>
