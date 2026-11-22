import { describe as suite, it, expect } from 'vitest'
import { dayName, setText, shortExercise, exerciseSummary, liftSummary, finisherShort, pick, open, liftTitle, trainingNote } from './training.js'

// The plan as tccd serves it (GET /api/training), from plans_upcoming.json.
const bench = {
  name: 'Bench Press (Barbell)', category: 'MainLift',
  sets: [...Array(4)].map(() => ({ weight_kg: 75, reps: 5, amrap: false })).concat([{ weight_kg: 75, reps: 10, amrap: true }]),
}
const pulldown = { name: 'Lat Pulldown (Cable)', category: 'Accessory', sets: [{ weight_kg: 52.5, reps: 12, amrap: false }] }
const upperA = {
  kind: 'lift', label: 'Upper A', status: 'planned', lift_day: 'Day1',
  lift: { week: 3, deload: false, exercises: [pulldown, bench], finisher: { name: 'Stair finisher', prescription: '10 min · steady, level 8' } },
}
const rest = { date: '2026-11-22', phase: 'cut', rest: true, sessions: [{ kind: 'rest', label: 'Rest', status: 'rest' }] }
const lift = { date: '2026-11-23', phase: 'cut', rest: false, sessions: [upperA] }
const walk = { date: '2026-11-24', rest: false, sessions: [{ kind: 'cardio', label: 'Zone 2 walk', status: 'planned' }] }
const plan = { today: '2026-11-22', from: '2026-11-22', no_plan: false, days: [rest, lift, walk], sync: { state: 'ok', stale: false } }

suite('training labels', () => {
  it('names days and sets', () => {
    expect(dayName('2026-11-24')).toBe('Tue 24 Nov')
    expect(setText({ weight_kg: 75, reps: 5, amrap: false })).toBe('75 kg × 5')
    expect(setText({ weight_kg: 75, reps: 10, amrap: true })).toBe('75 kg × 10+ (AMRAP)')
    expect(setText({ weight_kg: 0, reps: 8, amrap: false })).toBe('bodyweight × 8')
    expect(shortExercise('Bench Press (Barbell)')).toBe('Bench Press')
  })
  it('sums a lift up by its main lift', () => {
    expect(exerciseSummary(bench)).toBe('5×5+ @ 75 kg')
    expect(liftSummary(upperA.lift)).toBe('Bench Press 5×5+ @ 75 kg')
    expect(liftSummary({ exercises: [pulldown] })).toBe('Lat Pulldown 1×12 @ 52.5 kg')
    expect(liftSummary(null)).toBe('')
  })
  it('shortens the finisher', () => {
    expect(finisherShort(upperA.lift.finisher)).toBe('Stair 10 min')
  })
  it('lists every set in the tooltip', () => {
    expect(liftTitle(upperA)).toBe(
      'Upper A · week 3\nLat Pulldown (Cable): 52.5 kg × 12\nBench Press (Barbell): 75 kg × 5, 75 kg × 5, 75 kg × 5, 75 kg × 5, 75 kg × 10+ (AMRAP)\nFinisher: Stair finisher — 10 min · steady, level 8',
    )
  })
})

suite('pick', () => {
  it('shows up next on a rest day', () => {
    const { today, next } = pick(plan)
    expect(today).toBe(rest)
    expect(next).toBe(lift)
  })
  it('shows no up next while today has work left', () => {
    expect(pick({ ...plan, today: '2026-11-23' })).toEqual({ today: lift, next: null })
  })
  it('moves on once today is done', () => {
    const done = { ...lift, sessions: [{ ...upperA, status: 'done', lift: null }] }
    expect(open(done)).toBe(false)
    expect(pick({ ...plan, today: '2026-11-23', days: [done, walk] }).next).toBe(walk)
  })
  it('copes with nothing', () => {
    expect(pick(null)).toEqual({ today: null, next: null })
    expect(pick({ today: '2026-11-22', days: [] })).toEqual({ today: null, next: null })
  })
})

suite('trainingNote', () => {
  const now = new Date(2026, 10, 22, 12, 0)
  it('is quiet when fresh', () => expect(trainingNote(plan.sync, now)).toBeNull())
  it('says when the plan is an old read', () => {
    const at = new Date(2026, 10, 22, 8, 5).toISOString()
    expect(trainingNote({ state: 'stale', stale: true, last_sync: at }, now)).toEqual({ kind: 'stale', text: 'Offline · plan from 08:05' })
    expect(trainingNote({ state: 'auth', stale: true }, now).kind).toBe('auth')
  })
})
