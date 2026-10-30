<script>
  import { onDestroy } from 'svelte'
  import { api, vaultFile } from '../lib/api.js'
  import { fail } from '../lib/store.svelte.js'
  import { newPath, renamed, folderOf, moved, untitled } from '../lib/vault.js'
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
  // here is the folder new notes go in: the last folder clicked, or the
  // open note's folder.
  let here = $state('')
  // edit is a name being typed: a new note or folder, or a rename.
  let edit = $state(null)
  let menu = $state(null)
  // fresh is a note just made, so the editor takes the cursor.
  let fresh = $state('')
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
  const under = (p, top) => p === top || p.startsWith(top + '/')

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
    here = p
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
    here = folderOf(en)
    if (!en || en.dir || !en.name.toLowerCase().endsWith('.md')) return
    try {
      await loadNote(p)
    } catch (e) {
      status = 'not loaded'
      fail(e)
    }
  }

  function startNew(kind, dir = here) {
    menu = null
    q = ''
    if (dir) {
      expandTo(dir + '/x')
      open[dir] = true
    }
    edit = { kind, dir, value: kind === 'note' ? untitled(dir, files) : 'New folder' }
  }
  function startRename(e) {
    menu = null
    if (!e) return
    q = ''
    expandTo(e.path)
    edit = { kind: 'rename', entry: e, value: e.dir ? e.name : e.name.replace(/\.md$/i, '') }
  }
  async function commit() {
    const ed = edit
    if (!ed) return
    try {
      if (ed.kind === 'rename') {
        const from = ed.entry.path
        const to = renamed(from, ed.value, ed.entry.dir)
        if (!to) throw new Error("That name won't work")
        if (to !== from) {
          if (dirty && under(path, from)) await save()
          await api('POST', '/api/vault/move', { from, to })
          for (const k of Object.keys(open)) {
            if (open[k] && under(k, from)) open[moved(k, from, to)] = true
          }
          if (under(path, from)) remember(moved(path, from, to))
          here = moved(here, from, to)
        }
        edit = null
        await loadTree()
        return
      }
      const to = newPath(ed.dir, ed.value, ed.kind)
      if (!to) throw new Error("That name won't work")
      await api('POST', `/api/vault/${ed.kind}?path=` + encodeURIComponent(to))
      edit = null
      await loadTree()
      if (ed.kind === 'note') {
        fresh = to
        await pick(to)
      } else {
        open[to] = true
        here = to
      }
    } catch (e) {
      fail(e)
    }
  }
  async function remove(e) {
    menu = null
    if (!e || !confirm(`Move “${e.name}” to the vault's trash?`)) return
    const gone = under(path, e.path)
    if (gone) {
      clearTimeout(timer)
      dirty = false
    }
    try {
      await api('DELETE', '/api/vault/entry?path=' + encodeURIComponent(e.path))
      if (gone) {
        remember('')
        text = ''
        status = ''
      }
      if (under(here, e.path)) here = parentOf(e.path)
      await loadTree()
    } catch (err) {
      fail(err)
    }
  }
  // F2 and Del act on the selected file while the tree has focus.
  function treeKeys(ev) {
    if (edit || ev.target.tagName === 'INPUT') return
    if (ev.key === 'F2') {
      ev.preventDefault()
      startRename(selected)
    } else if (ev.key === 'Delete') {
      ev.preventDefault()
      remove(selected)
    }
  }
  function shortcuts(ev) {
    if ((ev.ctrlKey || ev.metaKey) && !ev.shiftKey && !ev.altKey && ev.key.toLowerCase() === 'n') {
      ev.preventDefault()
      startNew('note')
    }
  }
  function nameKeys(ev) {
    if (ev.key === 'Enter') {
      ev.preventDefault()
      commit()
    } else if (ev.key === 'Escape') {
      edit = null
    }
  }
  function focusName(node) {
    node.focus()
    node.select()
  }
  function openMenu(ev, e) {
    ev.preventDefault()
    ev.stopPropagation()
    menu = { x: ev.clientX, y: ev.clientY, entry: e }
  }

  onDestroy(() => {
    if (dirty) save()
  })

  loadTree()
    .then(() => {
      const en = entries.find(e => e.path === path)
      if (en) {
        expandTo(path)
        here = folderOf(en)
        if (!en.dir && en.name.toLowerCase().endsWith('.md')) return loadNote(path)
      }
    })
    .catch(e => {
      err = e.message
      fail(e)
    })
</script>

{#snippet nameInput(depth, mark)}
  <div class="row edit" style="padding-left: {6 + depth * 12}px">
    <span class="mark">{mark}</span>
    <input bind:value={edit.value} use:focusName onkeydown={nameKeys} onblur={() => (edit = null)} />
  </div>
{/snippet}

{#snippet tree(dir, depth)}
  {#if edit && edit.kind !== 'rename' && edit.dir === dir}
    {@render nameInput(depth, edit.kind === 'dir' ? '▸' : '')}
  {/if}
  {#each children(dir) as e (e.path)}
    {#if edit?.kind === 'rename' && edit.entry.path === e.path}
      {@render nameInput(depth, e.dir ? (open[e.path] ? '▾' : '▸') : '')}
    {:else}
      <button
        class="ghost row"
        class:on={path === e.path}
        class:here={e.dir && here === e.path}
        style="padding-left: {6 + depth * 12}px"
        onclick={() => (e.dir ? toggle(e.path) : pick(e.path))}
        ondblclick={() => startRename(e)}
        oncontextmenu={ev => openMenu(ev, e)}
      >
        <span class="mark">{e.dir ? (open[e.path] ? '▾' : '▸') : ''}</span>
        <span class="name">{e.name}</span>
      </button>
    {/if}
    {#if e.dir && open[e.path]}
      {@render tree(e.path, depth + 1)}
    {/if}
  {/each}
{/snippet}

{#snippet actions()}
  <span class="acts">
    <button class="ghost" title="Rename (F2)" onclick={() => startRename(selected)}>rename</button>
    <button class="ghost danger" title="Move to the vault's trash (Del)" onclick={() => remove(selected)}>delete</button>
  </span>
{/snippet}

<svelte:window onclick={() => (menu = null)} onkeydown={ev => ev.key === 'Escape' && (menu = null)} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="vault" onkeydown={shortcuts}>
  <div class="tree">
    <div class="tools">
      <input placeholder="Filter files" bind:value={q} />
      <button class="ghost tool" title="New note in {here || 'the vault'} (Ctrl+N)" onclick={() => startNew('note')}>+ note</button>
      <button class="ghost tool" title="New folder in {here || 'the vault'}" onclick={() => startNew('dir')}>+ folder</button>
    </div>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="list" onkeydown={treeKeys} oncontextmenu={ev => openMenu(ev, null)}>
      {#if hits}
        {#each hits as e (e.path)}
          <button class="ghost row" class:on={path === e.path} onclick={() => pick(e.path)} oncontextmenu={ev => openMenu(ev, e)}>
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
      <div class="faint pad">The vault is empty. <button class="ghost link" onclick={() => startNew('note')}>Make a note</button></div>
    {:else if !path}
      <div class="faint pad">Pick a note, or <button class="ghost link" onclick={() => startNew('note')}>make a new one</button>.</div>
    {:else if !selected}
      <div class="faint pad">That file is gone.</div>
    {:else if image}
      <div class="bar">
        <span class="faint path" title={path}>{path}</span>
        {@render actions()}
      </div>
      <div class="read"><img src={vaultFile(path)} alt={selected.name} /></div>
    {:else if note}
      <div class="bar">
        <span class="faint path" title={path}>{path}</span>
        {@render actions()}
      </div>
      {#await loadEditor then { default: NoteEditor }}
        <NoteEditor
          {path}
          {text}
          {files}
          focus={fresh === path}
          onchange={v => {
            fresh = ''
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

{#if menu}
  <div class="menu" style="left: {menu.x}px; top: {menu.y}px" role="menu">
    <button class="ghost" onclick={() => startNew('note', folderOf(menu.entry))}>New note</button>
    <button class="ghost" onclick={() => startNew('dir', folderOf(menu.entry))}>New folder</button>
    {#if menu.entry}
      <button class="ghost" onclick={() => startRename(menu.entry)}>Rename</button>
      <button class="ghost danger" onclick={() => remove(menu.entry)}>Delete</button>
    {/if}
  </div>
{/if}

<style>
  .vault { display: flex; height: 100%; min-height: 0; }
  .tree { width: 34%; min-width: 120px; max-width: 280px; display: flex; flex-direction: column; border-right: 1px solid var(--line); padding: 6px; gap: 6px; }
  .tools { display: flex; gap: 2px; align-items: center; }
  .tools input { flex: 1; min-width: 0; }
  .tool { flex: none; padding: 0 4px; color: var(--muted); white-space: nowrap; }
  .list { flex: 1; min-height: 0; overflow: auto; }
  .row { display: flex; width: 100%; text-align: left; gap: 4px; padding: 1px 4px; border-radius: 3px; color: var(--text); }
  .row.on { background: var(--btn); }
  .row.here .name { color: var(--strong); }
  .row.edit input { flex: 1; min-width: 0; padding: 0 3px; }
  .mark { width: 10px; flex: none; color: var(--faint); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .view { flex: 1; min-width: 0; display: flex; flex-direction: column; padding: 6px 8px; }
  .bar { display: flex; align-items: center; gap: 6px; flex: none; margin-bottom: 4px; }
  .path { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .acts { margin-left: auto; display: flex; gap: 4px; flex: none; }
  .link { color: var(--blue); padding: 0; }
  .danger { color: var(--red); }
  .read { flex: 1; min-height: 0; overflow: auto; user-select: text; }
  img { display: block; max-width: 100%; margin: 6px 0; }
  .foot { font-size: 10px; text-align: right; min-height: 14px; flex: none; }
  .pad { padding: 8px 2px; }
  .menu { position: fixed; z-index: 50; display: flex; flex-direction: column; min-width: 120px; padding: 3px; background: var(--bg); border: 1px solid var(--line-2); border-radius: 4px; box-shadow: 0 4px 14px rgba(0, 0, 0, 0.5); }
  .menu button { text-align: left; padding: 3px 8px; border-radius: 3px; }
</style>
