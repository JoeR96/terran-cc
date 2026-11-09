<script>
  import { db, act } from '../lib/store.svelte.js'
  import { select } from '../lib/actions.js'
  import { shortName, counterValue, pct, minPct, level, describe, parseAmount, syncNote, sourceTitle, streakTitle } from '../lib/strength.js'

  // The habits panel when 99-Strength owns the habits. Same three shapes as
  // the local panel: a list (narrow side widget, dashboard widget) or cards
  // (wide), with or without its own title.
  let { bars = true, title = true, wide = false } = $props()

  let editing = $state(null)
  let draft = $state('')
  let bad = $state(false)

  const sync = $derived(db.habitSync)
  const note = $derived(syncNote(sync, new Date(db.now * 1000)))
  const num = n => String(+(+n).toFixed(2))

  function startEdit(h) {
    editing = h.id
    draft = h.value > 0 ? String(h.value) : ''
    bad = false
  }
  // commit sends what was typed as the day's value. An empty box is "never
  // mind"; something that is not a value stays open, marked, to be fixed.
  function commit(h) {
    if (draft.trim() === '') {
      editing = null
      bad = false
      return
    }
    const v = parseAmount(draft, h)
    if (v === null) {
      bad = true
      return
    }
    editing = null
    bad = false
    if (v !== h.value) act.logHabit(h, { set: v })
  }
  function keys(e) {
    if (e.key === 'Enter') e.currentTarget.blur()
    if (e.key === 'Escape') {
      draft = ''
      editing = null
      bad = false
      e.currentTarget.blur()
    }
  }
</script>

{#snippet source()}
  <button class="ghost src" title={sourceTitle(sync, new Date(db.now * 1000))} onclick={() => act.syncStrength()}>
    99-Strength{#if sync?.pending && !note}<span class="wait"> ⟳{sync.pending}</span>{/if}
  </button>
{/snippet}

{#snippet streak()}
  <span class="streak" class:grace={db.streakInfo?.grace_used} title={streakTitle(db.streakInfo)}>🔥{db.streak}</span>
{/snippet}

{#if title}
  <h6><span class="ttl">Habits {@render source()}</span>{@render streak()}</h6>
{/if}
<!-- "habs" is the class the side widget's band lays out. -->
<div class="habs" class:wide>
  {#if !title}
    <div class="meta">{@render source()}{@render streak()}</div>
  {/if}
  {#if note}
    <button class="note {note.kind}" title={note.title} onclick={() => act.syncStrength()}>⚠ {note.text}</button>
  {/if}
  <div class="cells">
  {#each db.habits as h (h.id)}
    <div class="cell {level(h)}">
      {#if h.kind === 'checkbox'}
        <button class="h tick" role="checkbox" aria-checked={h.value >= 1} title={describe(h)} onclick={() => act.logHabit(h, { toggle: true })}>
          <span class="name">{h.icon} {shortName(h.name)}</span>
          {#if h.pending}<span class="wait" title="Not sent to 99-Strength yet">⟳</span>{/if}
          <span class="box big" class:on={h.value >= 1}></span>
        </button>
      {:else}
        <div class="h">
          <span class="name" title={describe(h)}>{h.icon} {shortName(h.name)}</span>
          {#if h.pending}<span class="wait" title="Not sent to 99-Strength yet">⟳</span>{/if}
          <span class="p">
            {#if h.kind === 'numeric'}
              {#if editing === h.id}
                <input
                  class="num"
                  class:bad
                  inputmode="decimal"
                  placeholder={h.unit === 'kg' ? '94.6' : '0'}
                  aria-label="{h.name}{h.unit ? ' in ' + h.unit : ''}"
                  bind:value={draft}
                  onblur={() => commit(h)}
                  onkeydown={keys}
                  use:select
                />
                <span class="faint">{h.unit}</span>
              {:else if !(h.value > 0)}
                <!-- Not logged yet: an empty box that becomes the editor when you click into it. -->
                <input
                  class="num"
                  inputmode="decimal"
                  placeholder={h.unit === 'kg' ? '94.6' : '0'}
                  aria-label="{h.name}{h.unit ? ' in ' + h.unit : ''}"
                  value=""
                  onfocus={() => startEdit(h)}
                />
                <span class="faint">{h.unit}</span>
              {:else}
                <button class="ghost val hit" title="Click to change" onclick={() => startEdit(h)}>{num(h.value)}<span class="unit">{h.unit}</span></button>
              {/if}
            {:else}
              {#if editing === h.id}
                <input class="num" class:bad bind:value={draft} onblur={() => commit(h)} onkeydown={keys} use:select />
              {:else}
                <button class="ghost val {level(h)}" title="Click to type an amount" onclick={() => startEdit(h)}>{counterValue(h)}</button>
              {/if}
              <button class="tap" title="One less" aria-label="One less {h.name}" disabled={!(h.value > 0)} onclick={() => act.logHabit(h, { delta: -1 })}>−</button>
              <button class="tap more" title="Add one{h.unit ? ' (' + h.unit + ')' : ''}" aria-label="Add one {h.name}" onclick={() => act.logHabit(h, { delta: 1 })}>+</button>
            {/if}
          </span>
        </div>
      {/if}
      {#if h.kind === 'counter' && (bars || wide)}
        <div class="bar">
          <i style="width:{pct(h)}%"></i>
          {#if minPct(h) !== null}<b class="mark" style="left:{minPct(h)}%" title="Minimum {num(h.minimum)}"></b>{/if}
        </div>
      {:else if wide}
        <div class="bar"><i style="width:{h.hit ? 100 : 0}%"></i></div>
      {/if}
    </div>
  {/each}
  </div>
  {#if db.ready && !db.habits.length}
    <div class="faint">{sync?.last_sync ? 'No habits in 99-Strength yet.' : 'Waiting for 99-Strength…'}</div>
  {/if}
</div>

<style>
  .ttl { display: flex; align-items: center; gap: 6px; min-width: 0; }
  /* The small source label; clicking it refreshes. */
  .src { color: var(--faint); letter-spacing: 0; text-transform: none; font-size: 10px; font-weight: 400; padding: 0 3px; white-space: nowrap; }
  .streak { color: var(--accent); letter-spacing: 0; text-transform: none; font-size: 11px; }
  .streak.grace { color: var(--yellow); }
  .wait { color: var(--yellow); flex: none; }
  .meta { display: flex; justify-content: space-between; align-items: center; margin: -2px 0 2px; }
  .note { display: block; width: 100%; text-align: left; font-size: 11px; padding: 2px 6px; margin: 0 0 4px; border-radius: 4px; white-space: normal; }
  .note.stale { background: var(--yellow-bg); color: var(--yellow); }
  .note.auth, .note.error { background: var(--red-bg); color: var(--red); }

  .h { display: flex; justify-content: space-between; align-items: center; min-height: 26px; gap: 6px; }
  .name { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .p { display: flex; gap: 3px; align-items: center; flex: none; }
  .val { color: var(--text); padding: 0 3px; font-variant-numeric: tabular-nums; }
  .val.min { color: var(--yellow); }
  .val.hit { color: var(--green); }
  .val .unit { color: var(--faint); margin-left: 4px; }
  /* Big enough to hit without aiming. */
  .tap { min-width: 26px; height: 22px; padding: 0; font-size: 14px; line-height: 20px; color: var(--muted); }
  .tap.more { color: var(--strong); background: var(--btn-hover); }
  .tap.more:hover { background: var(--accent); color: var(--bg); }
  .tap:disabled { opacity: .35; cursor: default; }
  .tap:disabled:hover { background: var(--btn); }
  .num { width: 58px; padding: 0 4px; text-align: right; }
  .num.bad { border-color: var(--red); }
  .tick { width: 100%; background: none; padding: 0; border-radius: 4px; color: inherit; text-align: left; }
  .tick:hover { background: none; }
  .tick:hover .box { border-color: var(--green); }
  .box.big { width: 18px; height: 18px; border-radius: 4px; margin: 0 4px; }

  .bar { position: relative; height: 3px; background: var(--line); border-radius: 2px; margin: 0 0 4px; }
  .bar i { display: block; height: 100%; background: var(--accent); border-radius: 2px; transition: width .3s; }
  .min .bar i { background: var(--yellow); }
  .hit .bar i { background: var(--green); }
  /* The minimum: a notch on the bar that the fill has to pass. */
  .bar .mark { position: absolute; top: -2px; width: 2px; height: 7px; margin-left: -1px; background: var(--strong); border-radius: 1px; }

  .habs.wide { display: flex; flex-direction: column; min-width: 0; }
  .wide .cells { flex: 1; display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 8px; }
  .wide .meta { margin: -3px 0 3px; }
  /* Wide: each habit is a solid block whose background fills with progress. */
  .wide .cell { position: relative; overflow: hidden; background: var(--panel-2); border: 1px solid var(--line-2); border-radius: 6px; padding: 0 8px; display: flex; align-items: center; min-height: 48px; }
  .wide .h { position: relative; z-index: 1; flex: 1; min-width: 0; font-size: 12.5px; font-weight: 600; flex-wrap: wrap; row-gap: 2px; padding: 4px 0; }
  .wide .name { flex: 1 0 100%; }
  .wide .wait { margin-right: auto; }
  .wide .p, .wide .box { margin-left: auto; }
  .wide .tick { align-self: stretch; }
  .wide .val { font-size: 12.5px; padding: 0 2px; }
  .wide .tap { min-width: 32px; height: 26px; font-size: 15px; line-height: 24px; }
  .wide .bar { position: absolute; inset: 0; height: auto; margin: 0; border-radius: 0; background: none; }
  .wide .bar i { border-radius: 0; background: var(--accent-fill); }
  .wide .min .bar i { background: var(--yellow-line); }
  .wide .hit .bar i { background: var(--green-fill); }
  .wide .bar .mark { top: 0; bottom: 0; height: auto; width: 0; margin-left: 0; background: none; border-left: 2px dotted var(--muted); border-radius: 0; }
</style>
