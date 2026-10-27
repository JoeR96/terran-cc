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
  import Vault from '../components/Vault.svelte'
  import OsrsView from '../components/OsrsView.svelte'
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
  // Ctrl+T toggles always-on-top while the widget has focus.
  function onKey(e) {
    if (e.ctrlKey && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 't') {
      e.preventDefault()
      setTop()
    }
  }

  // The bottom-right box holds the assistant or the notes scratchpad.
  const read = () => {
    try {
      return localStorage.getItem('tcc.side.tab') || 'chat'
    } catch {
      return 'chat'
    }
  }
  let tab = $state(read())
  // The assistant / notes box folds away so the other panels get its room.
  let boxOpen = $state((() => { try { return localStorage.getItem('tcc.side.box') !== 'closed' } catch { return true } })())
  function toggleBox() {
    boxOpen = !boxOpen
    try {
      localStorage.setItem('tcc.side.box', boxOpen ? 'open' : 'closed')
    } catch {}
  }
  // The middle column swaps between goals + projects and the OSRS view.
  let mid = $state((() => { try { return localStorage.getItem('tcc.side.mid') || 'goals' } catch { return 'goals' } })())
  function setMid(t) {
    mid = t
    try {
      localStorage.setItem('tcc.side.mid', t)
    } catch {}
  }
  function setTab(t) {
    tab = t
    try {
      localStorage.setItem('tcc.side.tab', t)
    } catch {}
  }

  let page = $state((() => { try { return localStorage.getItem('tcc.side.page') || 'hud' } catch { return 'hud' } })())
  function setPage(p) {
    page = p
    try {
      localStorage.setItem('tcc.side.page', p)
    } catch {}
  }
</script>

{#snippet middle()}
  <div class="tabs">
    <button class="ghost" class:on={mid === 'goals'} onclick={() => setMid('goals')}>Goals</button>
    <button class="ghost" class:on={mid === 'osrs'} onclick={() => setMid('osrs')}>OSRS</button>
  </div>
  {#if mid === 'osrs'}
    <OsrsView onopen={openItem} />
  {:else}
    <div class="part"><Goals onopen={openItem} /></div>
    <div class="part"><ProjectTree onopen={openItem} /></div>
  {/if}
{/snippet}

{#snippet box()}
  <div class="tabs">
    <button class="ghost" class:on={boxOpen && tab === 'chat'} onclick={() => (setTab('chat'), boxOpen || toggleBox())}>Assistant</button>
    <button class="ghost" class:on={boxOpen && tab === 'notes'} onclick={() => (setTab('notes'), boxOpen || toggleBox())}>Notes</button>
    <span class="grow"></span>
    {#if boxOpen}<button class="ghost fold" title="Fold away" onclick={toggleBox}>{wider ? '»' : '▾'}</button>{/if}
  </div>
  {#if boxOpen}<div class="tabbody">{#if tab === 'notes'}<Notes />{:else}<Chat />{/if}</div>{/if}
{/snippet}

<svelte:window onkeydown={onKey} />

<div class="sw" bind:clientWidth={width}>
  <div class="hd drag">
    <span class="hdl">
      {dayLabel(new Date((db.date || new Date().toISOString().slice(0, 10)) + 'T12:00'))}{#if wider} · 🔥 {db.streak}-day streak{/if}
      {#if conn().shell}
        <button class="ghost nodrag pin" class:on={onTop} title={onTop ? 'Always on top (Ctrl+T to turn off)' : 'Not on top (Ctrl+T to pin)'} onclick={setTop}>📌</button>
      {/if}
    </span>
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
        <button class="ghost" onclick={setTop}>{onTop ? '✓ ' : ''}Always on top <span class="faint">Ctrl+T</span></button>
      {/if}
    </div>
  {/if}

  <div class="pagetabs nodrag">
    <button class="ghost" class:on={page === 'hud'} onclick={() => setPage('hud')}>HUD</button>
    <button class="ghost" class:on={page === 'notes'} onclick={() => setPage('notes')}>Notes</button>
  </div>

  {#if locking && !dismissed && page === 'hud'}
    <button class="lock" title="Click to show the whole widget" onclick={() => (dismissed = true)}>
      <span class="time" class:paused={t.state === 'paused'}>{mmss(t.remaining_s)}</span>
      <span class="task">{t.label}</span>
      <span class="bar"><i style="width:{t.total_s ? (100 * (t.total_s - t.remaining_s)) / t.total_s : 0}%"></i></span>
    </button>
  {:else if page === 'notes'}
    <div class="vaultpage"><Vault /></div>
  {:else if wider}
    <div class="body wide">
      <div class="sec band">
        <div class="bandtimer"><Timer /></div>
        <div class="bandhabits"><Habits wide title={false} /></div>
      </div>
      <div class="cols three" class:boxed={!boxOpen}>
        <div class="col"><TodoTree onopen={openItem} /></div>
        <div class="col">{@render middle()}</div>
        {#if boxOpen}
          <div class="col boxcol">{@render box()}</div>
        {:else}
          <button class="ghost rail" title="Open the assistant and notes" onclick={toggleBox}><span>« Assistant · Notes</span></button>
        {/if}
      </div>
    </div>
  {:else if wide}
    <div class="body wide">
      <div class="sec"><Timer /></div>
      <div class="sec"><Habits wide /></div>
      <div class="cols">
        <div class="col"><TodoTree onopen={openItem} /></div>
        <div class="col right">
          <div class="scroll">{@render middle()}</div>
          <div class="chatbox" class:shut={!boxOpen}>{@render box()}</div>
        </div>
      </div>
    </div>
  {:else}
    <div class="body">
      <div class="sec"><Timer compact /></div>
      <div class="sec"><Habits /></div>
      <div class="sec"><TodoTree onopen={openItem} /></div>
      <div class="sec">{@render middle()}</div>
      <div class="sec chatnarrow" class:shut={!boxOpen}>{@render box()}</div>
    </div>
  {/if}
</div>

{#if drawer}<ItemDrawer id={drawer} onclose={() => (drawer = null)} />{/if}

<style>
  .sw { height: 100vh; display: flex; flex-direction: column; background: var(--panel); border: 1px solid var(--line-2); }
  .hd { display: flex; justify-content: space-between; align-items: center; padding: 3px 4px 3px 10px; background: var(--panel-2); border-bottom: 1px solid var(--line); color: var(--muted); font-size: 11px; flex: none; }
  .btns button { padding: 0 6px; }
  .hdl { display: flex; align-items: center; gap: 6px; }
  .pin { padding: 0 4px; font-size: 11px; filter: grayscale(1); opacity: .35; }
  .pin.on { filter: none; opacity: 1; }
  .menu { display: flex; flex-direction: column; background: var(--panel-2); border-bottom: 1px solid var(--line); padding: 4px; }
  .menu button { text-align: left; color: var(--text); }
  .body { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; }
  .body.wide { display: flex; flex-direction: column; overflow: hidden; }
  .wide .sec { flex: none; }
  .sec { padding: 8px 10px; border-bottom: 1px solid var(--line); }
  .cols { flex: 1; min-height: 0; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; padding: 8px 10px; }
  .col { min-height: 0; overflow-y: auto; background: var(--bg); border-radius: 7px; padding: 8px 10px; }
  .band { display: flex; gap: 14px; align-items: stretch; }
  .bandtimer { flex: none; width: 250px; }
  .bandhabits { flex: 1; min-width: 0; display: flex; }
  .bandhabits > :global(.habs) { flex: 1; }
  .cols.three { grid-template-columns: 27fr 40fr 33fr; }
  .boxcol { display: flex; flex-direction: column; overflow: hidden; }
  .col.right { display: flex; flex-direction: column; overflow: hidden; }
  .scroll { flex: 1; min-height: 0; overflow-y: auto; }
  .chatbox { flex: none; height: 42%; min-height: 160px; border-top: 1px solid var(--line); padding-top: 8px; }
  .chatnarrow { height: 320px; }
  .chatbox, .chatnarrow { display: flex; flex-direction: column; }
  .chatbox.shut, .chatnarrow.shut { height: auto; min-height: 0; }
  .cols.three.boxed { grid-template-columns: 38fr 62fr 26px; }
  .rail { padding: 0; border-radius: 7px; background: var(--bg); display: flex; justify-content: center; }
  .rail span { writing-mode: vertical-rl; font-size: 10px; letter-spacing: .07em; text-transform: uppercase; padding: 10px 0; }
  .rail:hover { background: var(--panel-2); }
  .grow { flex: 1; }
  .tabs .fold { letter-spacing: 0; }
  .pagetabs { display: flex; gap: 2px; padding: 4px 8px; border-bottom: 1px solid var(--line); flex: none; background: var(--panel-2); }
  .pagetabs button { font-size: 10px; letter-spacing: .07em; text-transform: uppercase; font-weight: 600; padding: 2px 8px; }
  .pagetabs button.on { color: var(--text); background: var(--btn); }
  .vaultpage { flex: 1; min-height: 0; }
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
