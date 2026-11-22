<script>
  import { db, act } from '../lib/store.svelte.js'
  import { ICONS, dayName, pick, work, liftSummary, finisherShort, liftTitle, exerciseSummary, shortExercise, trainingNote } from '../lib/training.js'

  // Training from 99-Strength, under the habits: today's sessions and, once
  // nothing is left today, the next planned day. Hover a lift for every set.
  let { wide = false } = $props()

  const t = $derived(db.training)
  const days = $derived(pick(t))
  const note = $derived(trainingNote(t?.sync, new Date(db.now * 1000)))
  const mark = s => (s.status === 'done' ? '✓' : s.status === 'planned' ? '' : s.status)
</script>

{#snippet session(s, showSets)}
  <div class="s {s.status}" title={liftTitle(s)}>
    <span class="ic">{ICONS[s.kind] || '•'}</span>
    <span class="lbl">{s.label}</span>
    {#if s.lift?.deload}<span class="tag">deload</span>{/if}
    {#if mark(s)}<span class="mk">{mark(s)}</span>{/if}
  </div>
  {#if s.lift && showSets}
    <ul class="ex">
      {#each s.lift.exercises as e}
        <li><span class="en">{shortExercise(e.name)}</span><span class="es">{exerciseSummary(e)}</span></li>
      {/each}
      {#if s.lift.finisher}<li class="fin"><span class="en">Finisher</span><span class="es">{finisherShort(s.lift.finisher)}</span></li>{/if}
    </ul>
  {:else if s.lift}
    <div class="sum">{liftSummary(s.lift)}{#if s.lift.finisher} · {finisherShort(s.lift.finisher)}{/if}</div>
  {/if}
{/snippet}

{#if t}
  <div class="tr" class:wide>
    <h6><span>Training</span><button class="ghost src" title="Plan from 99-Strength. Click to refresh." onclick={() => act.loadTraining()}>99-Strength</button></h6>
    {#if note}<div class="note {note.kind}">⚠ {note.text}</div>{/if}
    {#if t.no_plan}
      <div class="faint">No active plan in 99-Strength.</div>
    {:else}
      <div class="row">
        <span class="when">Today</span>
        <div class="what">
          {#if !days.today || days.today.rest || !work(days.today).length}
            <div class="s rest"><span class="ic">💤</span><span class="lbl">Rest</span></div>
          {:else}
            {#each work(days.today) as s}{@render session(s, true)}{/each}
          {/if}
        </div>
      </div>
      {#if days.next}
        <div class="row next">
          <span class="when">Up next · {dayName(days.next.date)}</span>
          <div class="what">
            {#each work(days.next).filter(s => s.status === 'planned') as s}{@render session(s, false)}{/each}
          </div>
        </div>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .tr { margin-top: 6px; }
  h6 { display: flex; justify-content: space-between; align-items: center; }
  .src { color: var(--faint); letter-spacing: 0; text-transform: none; font-size: 10px; font-weight: 400; padding: 0 3px; }
  .note { font-size: 11px; padding: 2px 6px; margin: 0 0 4px; border-radius: 4px; }
  .note.stale { background: var(--yellow-bg); color: var(--yellow); }
  .note.auth { background: var(--red-bg); color: var(--red); }
  .row { display: flex; flex-direction: column; gap: 1px; margin-bottom: 4px; }
  .when { color: var(--muted); font-size: 10.5px; text-transform: uppercase; letter-spacing: .04em; }
  .s { display: flex; align-items: center; gap: 6px; min-height: 22px; }
  .ic { flex: none; width: 18px; text-align: center; }
  .lbl { flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
  .s.done .lbl { color: var(--green); }
  .s.rest .lbl { color: var(--muted); font-weight: 400; }
  .mk { flex: none; color: var(--green); font-size: 11px; }
  .s.missed .mk, .s.paused .mk { color: var(--yellow); }
  .tag { flex: none; font-size: 10px; color: var(--yellow); border: 1px solid var(--yellow-line); border-radius: 3px; padding: 0 4px; }
  .sum { color: var(--muted); font-size: 11.5px; padding-left: 24px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ex { list-style: none; margin: 0 0 2px; padding: 0 0 0 24px; font-size: 11.5px; }
  .ex li { display: flex; justify-content: space-between; gap: 8px; line-height: 18px; }
  .en { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .es { flex: none; color: var(--muted); font-variant-numeric: tabular-nums; }
  .fin .en { color: var(--muted); }
  .wide .row { flex-direction: row; gap: 10px; }
  .wide .when { flex: 0 0 140px; padding-top: 4px; }
  .wide .what { flex: 1; min-width: 0; }
</style>
