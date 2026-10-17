<script>
  import { db } from '../lib/store.svelte.js'
  import { shell, conn } from '../lib/api.js'
  import { dayLabel, mmss } from '../lib/format.js'
  import Timer from '../components/Timer.svelte'
  import Habits from '../components/Habits.svelte'
  import TodoTree from '../components/TodoTree.svelte'
  import Goals from '../components/Goals.svelte'
  import ProjectTree from '../components/ProjectTree.svelte'
  import ItemDrawer from '../components/ItemDrawer.svelte'
  import Chat from '../components/Chat.svelte'
  import Notes from '../components/Notes.svelte'
  import { route } from '../lib/route.svelte.js'

  // Wider than this, the widget shows habits across the top and splits the
  // rest into to-dos (left) and goals + projects (right). Narrower, it all
  // stacks in one scrolling column; the trees fold to save space.
  const WIDE = 560
  // Wider still, the timer and habits share one band and the rest splits
  // into three: to-dos, goals + projects, assistant / notes.
  const WIDER = 1100

  let width = $state(0)
  let menu = $state(false)
  let drawer = $state(null)
  let onTop = $state(route.query.get('top') !== '0')
  const wide = $derived(width >= WIDE)
  const wider = $derived(width >= WIDER)

  // A lock-in countdown takes over the widget until it ends or is clicked.
  let dismissed = $state(false)
  const t = $derived(db.timer)
  const locking = $derived(!!t?.label && t.state !== 'idle')
  $effect(() => {
    if (!locking) dismissed = false
  })

  function setTop() {
    onTop = !onTop
    shell('top', { on: onTop })
    menu = false
  }
  const openItem = it => (drawer = it.id)

  // The bottom-right box holds the assistant or the notes scratchpad.
  const read = () => {
    try {
      return localStorage.getItem('tcc.side.tab') || 'chat'
    } catch {
      return 'chat'
    }
  }
  let tab = $state(read())
  function setTab(t) {
    tab = t
    try {
      localStorage.setItem('tcc.side.tab', t)
    } catch {}
  }
</script>

{#snippet box()}
  <div class="tabs">
    <button class="ghost" class:on={tab === 'chat'} onclick={() => setTab('chat')}>Assistant</button>
    <button class="ghost" class:on={tab === 'notes'} onclick={() => setTab('notes')}>Notes</button>
  </div>
  <div class="tabbody">{#if tab === 'notes'}<Notes />{:else}<Chat />{/if}</div>
{/snippet}

<div class="sw" bind:clientWidth={width}>
  <div class="hd drag">
    <span>{dayLabel(new Date((db.date || new Date().toISOString().slice(0, 10)) + 'T12:00'))}{#if wider} · 🔥 {db.streak}-day streak{/if}</span>
    <span class="nodrag btns">
      <button class="ghost" title="Menu" onclick={() => (menu = !menu)}>⋯</button>
      {#if conn().shell}
        <button class="ghost" title="Minimise" onclick={() => shell('minimise')}>–</button>
        <button class="ghost" title="Close" onclick={() => shell('close')}>✕</button>
      {/if}
    </span>
  </div>
  {#if menu}
    <div class="menu">
      <button class="ghost" onclick={() => { shell('open', { kind: 'dashboard' }); menu = false }}>Open dashboard</button>
      {#if conn().shell}
        <button class="ghost" onclick={setTop}>{onTop ? '✓ ' : ''}Always on top</button>
      {/if}
    </div>
  {/if}

  {#if locking && !dismissed}
    <button class="lock" title="Click to show the whole widget" onclick={() => (dismissed = true)}>
      <span class="time" class:paused={t.state === 'paused'}>{mmss(t.remaining_s)}</span>
      <span class="task">{t.label}</span>
      <span class="bar"><i style="width:{t.total_s ? (100 * (t.total_s - t.remaining_s)) / t.total_s : 0}%"></i></span>
    </button>
  {:else if wider}
    <div class="body wide">
      <div class="sec band">
        <div class="bandtimer"><Timer /></div>
        <div class="bandhabits"><Habits wide title={false} /></div>
      </div>
      <div class="cols three">
        <div class="col"><TodoTree onopen={openItem} /></div>
        <div class="col">
          <div class="part"><Goals onopen={openItem} /></div>
          <div class="part"><ProjectTree onopen={openItem} /></div>
        </div>
        <div class="col boxcol">{@render box()}</div>
      </div>
    </div>
  {:else if wide}
    <div class="body wide">
      <div class="sec"><Timer /></div>
      <div class="sec"><Habits wide /></div>
      <div class="cols">
        <div class="col"><TodoTree onopen={openItem} /></div>
        <div class="col right">
          <div class="scroll">
            <div class="part"><Goals onopen={openItem} /></div>
            <div class="part"><ProjectTree onopen={openItem} /></div>
          </div>
          <div class="chatbox">{@render box()}</div>
        </div>
      </div>
    </div>
  {:else}
    <div class="body">
      <div class="sec"><Timer compact /></div>
      <div class="sec"><Habits /></div>
      <div class="sec"><TodoTree onopen={openItem} /></div>
      <div class="sec"><Goals onopen={openItem} /></div>
      <div class="sec"><ProjectTree onopen={openItem} /></div>
      <div class="sec chatnarrow">{@render box()}</div>
    </div>
  {/if}
</div>

{#if drawer}<ItemDrawer id={drawer} onclose={() => (drawer = null)} />{/if}

<style>
  .sw { height: 100vh; display: flex; flex-direction: column; background: var(--panel); border: 1px solid var(--line-2); }
  .hd { display: flex; justify-content: space-between; align-items: center; padding: 3px 4px 3px 10px; background: var(--panel-2); border-bottom: 1px solid var(--line); color: var(--muted); font-size: 11px; flex: none; }
  .btns button { padding: 0 6px; }
  .menu { display: flex; flex-direction: column; background: var(--panel-2); border-bottom: 1px solid var(--line); padding: 4px; }
  .menu button { text-align: left; color: var(--text); }
  .body { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; }
  .body.wide { display: flex; flex-direction: column; overflow: hidden; }
  .wide .sec { flex: none; }
  .sec { padding: 8px 10px; border-bottom: 1px solid var(--line); }
  .cols { flex: 1; min-height: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 8px 10px; }
  .col { min-height: 0; overflow-y: auto; background: var(--bg); border-radius: 7px; padding: 8px 10px; }
  .band { display: flex; gap: 14px; align-items: center; }
  .bandtimer { flex: none; width: 250px; }
  .bandhabits { flex: 1; min-width: 0; }
  .cols.three { grid-template-columns: 27fr 40fr 33fr; }
  .boxcol { display: flex; flex-direction: column; overflow: hidden; }
  .col.right { display: flex; flex-direction: column; overflow: hidden; }
  .scroll { flex: 1; min-height: 0; overflow-y: auto; }
  .chatbox { flex: none; height: 42%; min-height: 160px; border-top: 1px solid var(--line); padding-top: 8px; }
  .chatnarrow { height: 320px; }
  .chatbox, .chatnarrow { display: flex; flex-direction: column; }
  .tabs { display: flex; gap: 2px; margin-bottom: 6px; flex: none; }
  .tabs button { font-size: 10px; letter-spacing: .07em; text-transform: uppercase; font-weight: 600; padding: 1px 6px; }
  .tabs button.on { color: var(--text); background: var(--btn); }
  .tabbody { flex: 1; min-height: 0; }
  .part { padding: 0 0 8px; }
  .part + .part { border-top: 1px solid var(--line); padding-top: 8px; }
  .lock { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; background: var(--panel); border-radius: 0; padding: 16px; }
  .lock:hover { background: var(--panel); }
  .lock .time { font-size: clamp(40px, 16vw, 140px); font-weight: 600; color: var(--accent); font-variant-numeric: tabular-nums; line-height: 1; }
  .lock .time.paused { color: var(--muted); }
  .lock .task { font-size: clamp(14px, 3.5vw, 28px); color: var(--text); text-align: center; max-width: 90%; }
  .lock .bar { width: min(80%, 600px); height: 4px; background: var(--line); border-radius: 2px; overflow: hidden; }
  .lock .bar i { display: block; height: 100%; background: var(--accent); transition: width 1s linear; }
</style>
