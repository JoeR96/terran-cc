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

export const kg = v => (v ? `${num(v)} kg` : 'bodyweight')

// setText is "75 kg × 5", or "75 kg × 10+ (AMRAP)".
export const setText = s => `${kg(s.weight_kg)} × ${s.reps}${s.amrap ? '+ (AMRAP)' : ''}`

// shortExercise drops the equipment: "Bench Press (Barbell)" is "Bench Press".
export const shortExercise = name => name.replace(/\s*\([^)]*\)\s*$/, '') || name

// exerciseSummary is "5×5+ @ 75 kg": sets × first reps ("+" when the last
// set is AMRAP) at the heaviest weight.
export function exerciseSummary(e) {
  if (!e.sets?.length) return ''
  const last = e.sets[e.sets.length - 1]
  const top = Math.max(...e.sets.map(s => s.weight_kg))
  return `${e.sets.length}×${e.sets[0].reps}${last.amrap ? '+' : ''} @ ${kg(top)}`
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
  const lines = s.lift.exercises.map(e => `${e.name}: ${e.sets.map(setText).join(', ')}`)
  if (s.lift.finisher) lines.push(`Finisher: ${s.lift.finisher.name} — ${s.lift.finisher.prescription}`)
  return [head, ...lines].join('\n')
}

// trainingNote is the warning when the plan shown is an old read, or null.
export function trainingNote(sync, now = new Date()) {
  if (!sync?.stale) return null
  if (sync.state === 'auth') return { kind: 'auth', text: 'Token needs renewing: run tcc strength connect' }
  return { kind: 'stale', text: sync.last_sync ? `Offline · plan from ${syncTime(sync.last_sync, now)}` : 'Offline' }
}
