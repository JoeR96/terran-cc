<script>
  import { db, act } from '../lib/store.svelte.js'
  import { api } from '../lib/api.js'
  import { ICONS, dayName, addDays, dayView, hasDay, work, setText, needsWeight, trainingNote } from '../lib/training.js'

  // The Training panel: one day of the 99-Strength plan in full, every
  // session with each exercise's sets and the finisher. Arrows step through
  // the days; days past the two weeks tccd keeps are read on demand.
  // training and date are for tests; the panel reads db.training.
  let { training = undefined, date = undefined } = $props()

  const t = $derived(training === undefined ? db.training : training)
  let picked = $state(date ?? null)
  let more = $state([]) // days read beyond the cached range
  let loading = $state(false)
  const shown = $derived(picked || t?.today || '')
  const view = $derived(dayView(t, shown, more))
  const note = $derived(trainingNote(t?.sync, new Date(db.now * 1000)))
  const mark = s => (s.status === 'done' ? '✓ done' : s.status === 'planned' ? '' : s.status)

  async function go(n) {
    const d = n === 0 ? t.today : addDays(shown, n)
    picked = d === t.today ? null : d
    if (hasDay(t, d, more) || d < t.from) return
    loading = true
    try {
      const r = await api('GET', `/api/training?from=${d}&days=14`)
      more = [...more.filter(x => !r.days.some(y => y.date === x.date)), ...r.days]
    } catch {
      // Leave the day empty; the header says it could not be read.
    } finally {
      loading = false
    }
  }
</script>

{#snippet sessions(day)}
  {#each work(day) as s}
    <section class="s {s.status}">
      <div class="sh">
        <span class="ic">{ICONS[s.kind] || '•'}</span>
        <span class="lbl">{s.label}</span>
        {#if s.lift}<span class="wk">week {s.lift.week}</span>{/if}
        {#if mark(s)}<span class="mk">{mark(s)}</span>{/if}
      </div>
      {#if s.lift}
        {#if s.lift.deload}<div class="deload">Deload week: lighter weights and fewer sets, no AMRAP</div>{/if}
        <ol class="ex">
          {#each s.lift.exercises as e}
            <li>
              <div class="en">{e.name}{#if needsWeight(e)}<span class="hint" title="No starting weight yet: set it in 99-Strength">set weight</span>{/if}</div>
              <div class="sets">
                {#each e.sets as x}<span class="set" class:amrap={x.amrap}>{setText(x, e)}</span>{/each}
              </div>
            </li>
          {/each}
        </ol>
        {#if s.lift.finisher}
          <div class="fin"><span class="fn">Finisher · {s.lift.finisher.name}</span><span class="fp">{s.lift.finisher.prescription}</span></div>
        {/if}
      {/if}

    </section>
  {/each}
{/snippet}

<div class="tp">
  {#if !t}
    <div class="faint">Not connected to 99-Strength. Run <code>tcc strength connect</code>.</div>
  {:else}
    <div class="top">
      <button class="ghost" title="Previous day" aria-label="Previous day" onclick={() => go(-1)}>‹</button>
      <span class="day">{shown === t.today ? 'Today' : dayName(shown)}{#if shown === t.today} <span class="faint">· {dayName(shown)}</span>{/if}</span>
      <button class="ghost" title="Next day" aria-label="Next day" onclick={() => go(1)}>›</button>
      <span class="grow"></span>
      {#if shown !== t.today}<button class="ghost" onclick={() => go(0)}>Today</button>{/if}
      <button class="ghost src" title="Plan from 99-Strength. Click to refresh." onclick={() => act.loadTraining()}>↻</button>
    </div>
    {#if note}<div class="note {note.kind}">⚠ {note.text}</div>{/if}
    {#if t.no_plan}
      <div class="faint">No active plan in 99-Strength.</div>
    {:else if loading}
      <div class="faint">Loading…</div>
    {:else if !view.day && shown < t.from}
      <div class="faint">Only today and later are planned here.</div>
    {:else if !view.day}
      <div class="faint">Nothing read for this day.</div>
    {:else}
      {#if view.future}<div class="faint fut">Weights can change if you finish sessions before then.</div>{/if}
      {#if view.rest}
        <div class="rest">💤 Rest day</div>
      {:else}
        {@render sessions(view.day)}
      {/if}
      {#if view.next}
        <div class="next">Up next · {dayName(view.next.date)}</div>
        <div class="faint fut">Weights can change if you finish sessions before then.</div>
        {@render sessions({ sessions: work(view.next).filter(s => s.status === 'planned') })}
      {/if}
    {/if}
  {/if}
</div>

<style>
  .tp { font-size: 12px; }
  .top { display: flex; align-items: center; gap: 4px; margin-bottom: 6px; position: sticky; top: -8px; background: var(--bg); padding: 2px 0; z-index: 1; }
  .top button { padding: 0 7px; font-size: 13px; }
  .day { font-weight: 600; }
  .grow { flex: 1; }
  .src { color: var(--faint); }
  .note { font-size: 11px; padding: 2px 6px; margin: 0 0 6px; border-radius: 4px; }
  .note.stale { background: var(--yellow-bg); color: var(--yellow); }
  .note.auth { background: var(--red-bg); color: var(--red); }
  .fut { font-size: 11px; margin-bottom: 4px; }
  .rest { color: var(--muted); padding: 4px 0; }
  .next { color: var(--muted); font-size: 10.5px; text-transform: uppercase; letter-spacing: .04em; margin: 10px 0 2px; }
  .s { margin-bottom: 10px; }
  .sh { display: flex; align-items: center; gap: 6px; min-height: 22px; }
  .ic { flex: none; width: 18px; text-align: center; }
  .lbl { font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .wk { color: var(--muted); font-size: 11px; }
  .mk { margin-left: auto; color: var(--green); font-size: 11px; }
  .s.done .lbl { color: var(--green); }
  .s.missed .mk, .s.paused .mk { color: var(--yellow); }
  .deload { margin: 0 0 4px 24px; font-size: 11px; color: var(--yellow); }
  .ex { margin: 0; padding: 0 0 0 24px; list-style: none; }
  .ex li { padding: 3px 0; border-top: 1px solid var(--line); }
  .ex li:first-child { border-top: none; }
  .en { font-weight: 500; }
  .hint { margin-left: 6px; font-size: 10px; color: var(--yellow); border: 1px solid var(--yellow-line); border-radius: 3px; padding: 0 4px; }
  .sets { display: flex; flex-wrap: wrap; gap: 2px 10px; color: var(--muted); font-variant-numeric: tabular-nums; font-size: 11.5px; }
  .set.amrap { color: var(--accent); }
  .fin, .sum { margin: 4px 0 0 24px; font-size: 11.5px; color: var(--muted); display: flex; flex-wrap: wrap; gap: 0 8px; }
  .fn { color: var(--text); }
</style>
