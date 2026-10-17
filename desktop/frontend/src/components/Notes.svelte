<script>
  import { db, fail } from '../lib/store.svelte.js'
  import { api } from '../lib/api.js'

  // A scratchpad for pasting things to keep for a while. It saves as you
  // type and is shared by every window through tccd.
  let text = $state(db.notes)
  let dirty = false
  let focused = false
  let status = $state('')
  let timer

  // Take edits from other windows unless this one is mid-edit.
  $effect(() => {
    const remote = db.notes
    if (!dirty && !focused) text = remote
  })

  function changed() {
    dirty = true
    status = 'saving…'
    clearTimeout(timer)
    timer = setTimeout(save, 600)
  }
  async function save() {
    clearTimeout(timer)
    if (!dirty) return
    const v = text
    try {
      await api('PUT', '/api/ui/notes', v)
      db.notes = v
      dirty = false
      status = 'saved'
    } catch (e) {
      status = 'not saved'
      fail(e)
    }
  }
</script>

<div class="notes">
  <textarea
    placeholder="Paste anything here to keep it for a while. It saves as you type."
    bind:value={text}
    oninput={changed}
    onfocus={() => (focused = true)}
    onblur={() => ((focused = false), save())}
    spellcheck="false"
  ></textarea>
  <div class="foot faint">{status}{status && text ? ' · ' : ''}{text ? `${text.length} chars` : ''}</div>
</div>

<style>
  .notes { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  textarea { flex: 1; min-height: 60px; resize: none; font: 12px/1.45 var(--font); }
  .foot { font-size: 10px; text-align: right; padding-top: 2px; min-height: 14px; }
</style>
