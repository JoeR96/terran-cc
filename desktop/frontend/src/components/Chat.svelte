<script>
  import { api } from '../lib/api.js'

  // Chat with the assistant (OpenCode Go, model in the loop). It can add,
  // edit and complete to-dos and tickets, log habits and run the timer.
  // The conversation is kept per window until "new".
  const KEY = 'tcc.chat'
  const load = () => {
    try {
      return JSON.parse(localStorage.getItem(KEY)) || null
    } catch {
      return null
    }
  }
  const fresh = () => ({ session: 'tcc-' + Date.now().toString(36), msgs: [] })
  let chat = $state(load() || fresh())
  let draft = $state('')
  let busy = $state(false)
  let log

  function save() {
    try {
      localStorage.setItem(KEY, JSON.stringify({ session: chat.session, msgs: chat.msgs.slice(-40) }))
    } catch {}
  }
  function scroll() {
    queueMicrotask(() => log && (log.scrollTop = log.scrollHeight))
  }
  $effect(() => {
    chat.msgs.length
    scroll()
  })

  async function send() {
    const text = draft.trim()
    if (!text || busy) return
    draft = ''
    chat.msgs.push({ role: 'user', content: text })
    busy = true
    save()
    // Only real turns go back to the model; failed replies stay local.
    const history = chat.msgs.filter(m => !m.error).map(m => ({ role: m.role, content: m.content })).slice(-20)
    try {
      const r = await api('POST', '/api/assist', { session: chat.session, messages: history })
      chat.msgs.push({ role: 'assistant', content: r.reply || '(no reply)', actions: r.actions || [] })
    } catch (e) {
      chat.msgs.push({ role: 'assistant', content: e.message, error: true })
    }
    busy = false
    save()
  }
  function reset() {
    chat = fresh()
    save()
  }
</script>

<div class="chat">
  {#if chat.msgs.length}<button class="ghost new" title="Start a new conversation" onclick={reset}>new chat</button>{/if}
  <div class="log" bind:this={log}>
    {#each chat.msgs as m, i (i)}
      <div class="m {m.role}" class:err={m.error}>
        <div class="text">{m.content}</div>
        {#each m.actions || [] as a, j (j)}
          <div class="act" class:bad={!a.ok}>{a.ok ? '✓' : '✗'} {a.tool.replace('_', ' ')} · {a.summary}</div>
        {/each}
      </div>
    {:else}
      <div class="faint hint">Tell me what to change: "idea for bonanza: exploding barrels", "osrs short term: prep for raids s4", "log 500ml water", "lock in 25 min on the axe PR".</div>
    {/each}
    {#if busy}<div class="m assistant faint">thinking…</div>{/if}
  </div>
  <textarea
    rows="2"
    placeholder="Ask or tell… (Enter to send, Shift+Enter for a new line)"
    bind:value={draft}
    disabled={busy}
    onkeydown={e => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        send()
      }
    }}
  ></textarea>
</div>

<style>
  .chat { display: flex; flex-direction: column; min-height: 0; height: 100%; position: relative; }
  .new { position: absolute; top: -24px; right: 0; font-size: 10px; padding: 0 5px; }
  .log { flex: 1; min-height: 40px; overflow-y: auto; display: flex; flex-direction: column; gap: 6px; padding: 2px 0 6px; user-select: text; }
  .m { max-width: 92%; padding: 4px 8px; border-radius: 6px; white-space: pre-wrap; word-break: break-word; }
  .m.user { align-self: flex-end; background: var(--btn); }
  .m.assistant { align-self: flex-start; background: var(--panel-2); }
  .m.err { color: var(--red); background: var(--red-bg); }
  .act { font-size: 10.5px; color: var(--green); margin-top: 2px; }
  .act.bad { color: var(--red); }
  .hint { font-size: 11px; }
  textarea { resize: none; flex: none; }
</style>
