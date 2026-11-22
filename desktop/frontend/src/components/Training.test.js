import { describe as suite, it, expect, vi } from 'vitest'
import { render } from 'svelte/server'

// The api is faked: stepping past the cached days reads more.
const calls = []
vi.mock('../lib/api.js', () => ({
  api: async () => {
    calls.push(path)
    return { days: [] }
  },
  listen: () => {},
  conn: () => ({}),
}))

const { default: Training } = await import('./Training.svelte')

const bench = {
  name: 'Bench Press (Barbell)', category: 'MainLift',
  sets: [...Array(4)].map(() => ({ weight_kg: 75, reps: 5, amrap: false })).concat([{ weight_kg: 75, reps: 10, amrap: true }]),
}
const curl = { name: 'Lying Leg Curl (Machine)', category: 'Accessory', sets: [{ weight_kg: 0, reps: 10, amrap: false }, { weight_kg: 0, reps: 10, amrap: false }] }
const press = { name: 'Overhead Press (Barbell)', category: 'Auxiliary', sets: [{ weight_kg: 0, reps: 7, amrap: false }] }
const upperA = {
  kind: 'lift', label: 'Upper A', status: 'planned',
  lift: { week: 3, deload: false, exercises: [bench, curl, press], finisher: { name: 'Stair finisher', prescription: '10 min · steady, level 8' } },
}
const lowerB = { kind: 'lift', label: 'Lower B', status: 'planned', lift: { week: 7, deload: true, exercises: [curl] } }
const walk = { kind: 'cardio', label: 'Zone 2 walk · 45 min', status: 'done' }
const rest = { date: '2026-11-22', rest: true, sessions: [{ kind: 'rest', label: 'Rest', status: 'rest' }] }
const d23 = { date: '2026-11-23', rest: false, sessions: [upperA, walk] }
const d24 = { date: '2026-11-24', rest: false, sessions: [lowerB] }
const plan = { today: '2026-11-22', from: '2026-11-22', no_plan: false, days: [rest, d23, d24], sync: { state: 'ok', stale: false } }

const html = props => render(Training, { props }).body.replace(/<!--[^>]*-->/g, '').replace(/&amp;/g, '&')

suite('Training panel', () => {
  it('shows a whole session: every exercise, every set, AMRAP, finisher, cardio', () => {
    const h = html({ training: { ...plan, today: '2026-11-23' } })
    expect(h).toContain('Today')
    expect(h).toContain('Upper A')
    expect(h).toContain('week 3')
    expect(h.match(/75 kg × 5</g)).toHaveLength(4)
    expect(h).toContain('75 kg × 10+ (AMRAP)')
    expect(h).toContain('Lying Leg Curl (Machine)')
    expect(h).toContain('>× 10<')
    expect(h).toContain('— × 7')
    expect(h).toContain('set weight')
    expect(h).not.toContain('bodyweight')
    expect(h).toContain('Finisher · Stair finisher')
    expect(h).toContain('10 min · steady, level 8')
    expect(h).toContain('Zone 2 walk · 45 min')
    expect(h).toContain('✓ done')
    expect(h).not.toContain('Weights can change')
    expect(h.indexOf('Bench Press')).toBeLessThan(h.indexOf('Lying Leg Curl'))
  })

  it('shows a rest day, then up next with the whole next session', () => {
    const h = html({ training: plan })
    expect(h).toContain('Rest day')
    expect(h).toContain('Up next · Mon 23 Nov')
    expect(h).toContain('75 kg × 10+ (AMRAP)')
    expect(h).toContain('Weights can change if you finish sessions before then.')
    expect(h).not.toContain('Zone 2 walk') // done already: not up next
  })

  it('shows a stepped-to future day with its deload and the weights note', () => {
    const h = html({ training: plan, date: '2026-11-24' })
    expect(h).toContain('Tue 24 Nov')
    expect(h).toContain('>Today<') // the button back
    expect(h).toContain('Deload week')
    expect(h).toContain('Weights can change if you finish sessions before then.')
    expect(h).not.toContain('Up next')
  })

  it('handles no plan, not connected and offline', () => {
    expect(html({ training: { ...plan, no_plan: true } })).toContain('No active plan in 99-Strength.')
    expect(html({ training: null })).toContain('Not connected to 99-Strength')
    const stale = html({ training: { ...plan, sync: { state: 'stale', stale: true } } })
    expect(stale).toContain('⚠ Offline')
    expect(stale).toContain('Rest day')
    expect(html({ training: { ...plan, sync: { state: 'auth', stale: true } } })).toContain('tcc strength connect')
  })

  it('says when a day is outside what was read', () => {
    expect(html({ training: plan, date: '2026-11-21' })).toContain('Only today and later')
    expect(html({ training: plan, date: '2026-12-25' })).toContain('Nothing read for this day.')
  })
})
