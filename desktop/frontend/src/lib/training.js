// Helpers for the Training card: the plan tccd reads from 99-Strength
// (GET /api/training). Days come today first; a session's kind is rest,
// lift, cardio or mobility and its status planned, done, missed, paused or
// rest. A lift still to do carries its sets; an AMRAP set's reps are its
// rep-out target. These match `tcc status` and `tcc workout`.

import { syncTime } from './strength.js'

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const num = n => String(+(+n).toFixed(2))

// dayName renders "2026-11-24" as "Tue 24 Nov".
export function dayName(date) {
  const [y, m, d] = date.split('-').map(Number)
  const t = new Date(Date.UTC(y, m - 1, d))
  return `${DAYS[t.getUTCDay()]} ${d} ${MONTHS[m - 1]}`
}

export const kg = v => `${num(v)} kg`

// needsWeight reports a main or auxiliary lift, always loaded, whose
// weight is not set yet (99-Strength sends 0).
export const needsWeight = e =>
  (e?.category === 'MainLift' || e?.category === 'Auxiliary') && !!e.sets?.length && e.sets.every(s => !s.weight_kg)

// weightText is how a set's weight reads. 99-Strength sends 0 both for
// bodyweight work and for a weight not set yet, so 0 is "bodyweight" only
// when the name says so, "—" for a lift that needs a weight, else nothing.
export function weightText(e, v) {
  if (v > 0) return kg(v)
  if (/bodyweight/i.test(e?.name || '')) return 'bodyweight'
  if (needsWeight(e)) return '—'
  return ''
}

// setText is "75 kg × 5", "75 kg × 10+ (AMRAP)", or "× 10" with no weight to show.
export function setText(s, e) {
  const w = weightText(e, s.weight_kg)
  return `${w ? w + ' ' : ''}× ${s.reps}${s.amrap ? '+ (AMRAP)' : ''}`
}

// addDays moves "2026-11-24" by n days.
export function addDays(date, n) {
  const [y, m, d] = date.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10)
}

// dayView is what the Training panel shows for date: that day from the
// plan (or from more, days read for further out), whether it is a rest
// day, and on a restful today the next planned day.
export function dayView(training, date, more = []) {
  if (!training || training.no_plan) return { day: null, rest: false, next: null, future: false }
  const all = [...(training.days || []), ...more]
  const day = all.find(d => d.date === date) || null
  const rest = !day || day.rest || !work(day).length
  const next = date === training.today && !open(day) ? all.find(d => d.date > date && open(d)) || null : null
  return { day, rest, next, future: date > training.today }
}

// hasDay reports whether date is in the days already read.
export const hasDay = (training, date, more = []) => [...(training?.days || []), ...more].some(d => d.date === date)

// shortExercise drops the equipment: "Bench Press (Barbell)" is "Bench Press".
export const shortExercise = name => name.replace(/\s*\([^)]*\)\s*$/, '') || name

// exerciseSummary is "5×5+ @ 75 kg": sets × first reps ("+" when the last
// set is AMRAP) at the heaviest weight.
export function exerciseSummary(e) {
  if (!e.sets?.length) return ''
  const last = e.sets[e.sets.length - 1]
  const top = Math.max(...e.sets.map(s => s.weight_kg))
  const w = weightText(e, top)
  return `${e.sets.length}×${e.sets[0].reps}${last.amrap ? '+' : ''}${w ? ' @ ' + w : ''}`
}

// liftSummary is the headline of a lift: its first main lift, or first exercise.
export function liftSummary(lift) {
  if (!lift?.exercises?.length) return ''
  const e = lift.exercises.find(x => x.category === 'MainLift') || lift.exercises[0]
  const sum = exerciseSummary(e)
  return sum ? `${shortExercise(e.name)} ${sum}` : shortExercise(e.name)
}

// finisherShort is "Stair 10 min" from "Stair finisher" / "10 min · steady, level 8".
export function finisherShort(f) {
  const name = f.name.trim().replace(/\s*finisher$/i, '') || f.name
  return `${name} ${f.prescription.split('·')[0].trim()}`.trim()
}

export const ICONS = { lift: '🏋', cardio: '🏃', mobility: '🧘', rest: '💤' }

// work is a day's sessions other than rest.
export const work = day => (day?.sessions || []).filter(s => s.kind !== 'rest')

// open reports whether a day still has something to do.
export const open = day => work(day).some(s => s.status === 'planned')

// pick finds today and, when nothing is left to do today, the next planned day.
export function pick(training) {
  if (!training) return { today: null, next: null }
  const today = training.days.find(d => d.date === training.today) || null
  const next = open(today) ? null : training.days.find(d => d.date > training.today && open(d)) || null
  return { today, next }
}

// liftTitle is the tooltip of a lift: every exercise with all its sets.
export function liftTitle(s) {
  if (!s.lift) return s.label
  const head = `${s.label} · week ${s.lift.week}${s.lift.deload ? ' (deload)' : ''}`
  const lines = s.lift.exercises.map(e => `${e.name}: ${e.sets.map(s => setText(s, e)).join(', ')}`)
  if (s.lift.finisher) lines.push(`Finisher: ${s.lift.finisher.name} — ${s.lift.finisher.prescription}`)
  return [head, ...lines].join('\n')
}

// trainingNote is the warning when the plan shown is an old read, or null.
export function trainingNote(sync, now = new Date()) {
  if (!sync?.stale) return null
  if (sync.state === 'auth') return { kind: 'auth', text: 'Token needs renewing: run tcc strength connect' }
  return { kind: 'stale', text: sync.last_sync ? `Offline · plan from ${syncTime(sync.last_sync, now)}` : 'Offline' }
}
