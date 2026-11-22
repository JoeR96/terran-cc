import { describe as suite, it, expect } from 'vitest'
import { shortName, counterValue, pct, minPct, level, describe, parseAmount, optimistic, syncTime, syncNote, sourceTitle, streakTitle } from './strength.js'

// Habits as tccd serves them when 99-Strength is the source.
const water = { id: 1, name: 'Water (532ml bottle)', kind: 'counter', type: 'count', unit: 'bottles', target: 6, minimum: 4, step: 1, value: 3, hit: false, source: '99-strength' }
const weigh = { id: 3, name: 'Morning weigh-in', kind: 'numeric', type: 'numeric', unit: 'kg', target: 1, minimum: 1, value: 0, hit: false, source: '99-strength' }
const crea = { id: 5, name: 'Creatine (3–5g)', kind: 'checkbox', type: 'check', unit: '', target: 1, minimum: 1, step: 1, value: 0, hit: false, source: '99-strength' }

suite('labels', () => {
  it('drops a trailing bracket from long names', () => {
    expect(shortName('Water (532ml bottle)')).toBe('Water')
    expect(shortName('Creatine (3–5g)')).toBe('Creatine')
    expect(shortName('Morning weigh-in')).toBe('Morning weigh-in')
    expect(shortName('(odd)')).toBe('(odd)')
  })
  it('shows a counter as value/target', () => {
    expect(counterValue(water)).toBe('3/6')
    expect(counterValue({ ...water, value: 7 })).toBe('7/6')
  })
  it('describes each kind for the tooltip', () => {
    expect(describe(water)).toBe('Water (532ml bottle) · 3 of 6 bottles · minimum 4')
    expect(describe(weigh)).toBe('Morning weigh-in · not logged')
    expect(describe({ ...weigh, value: 94.6 })).toBe('Morning weigh-in · 94.6 kg')
    expect(describe({ ...crea, value: 1, hit: true })).toBe('Creatine (3–5g) · done')
  })
})

suite('the bar and its minimum marker', () => {
  it('fills by value over target, never past the end', () => {
    expect(pct(water)).toBe(50)
    expect(pct({ ...water, value: 9 })).toBe(100)
    expect(pct({ ...water, value: 0 })).toBe(0)
  })
  it('puts the marker at minimum over target', () => {
    expect(minPct(water)).toBeCloseTo(66.67, 1)
    expect(minPct({ ...water, target: 2, minimum: 1 })).toBe(50)
  })
  it('has no marker when the minimum is the target', () => {
    expect(minPct(crea)).toBeNull()
    expect(minPct(weigh)).toBeNull()
    expect(minPct({ ...water, minimum: 0 })).toBeNull()
  })
  it('colours by what has been reached', () => {
    expect(level(water)).toBe('')
    expect(level({ ...water, value: 4, min_hit: true })).toBe('min')
    expect(level({ ...water, value: 6, min_hit: true, hit: true })).toBe('hit')
  })
})

suite('parseAmount', () => {
  it('reads a weigh-in like the phone app does', () => {
    expect(parseAmount('94.6', weigh)).toBe(94.6)
    expect(parseAmount(' 94,64 ', weigh)).toBe(94.6)
    expect(parseAmount('95', weigh)).toBe(95)
  })
  it('refuses a weight no person has, and anything that is not a number', () => {
    for (const bad of ['12', '500', '', 'abc', '-80', '9e2', '94.6kg']) expect(parseAmount(bad, weigh)).toBeNull()
  })
  it('reads a typed counter value', () => {
    expect(parseAmount('5', water)).toBe(5)
    expect(parseAmount('0', water)).toBe(0)
    expect(parseAmount('-1', water)).toBeNull()
  })
})

suite('optimistic', () => {
  it('adds and takes away on a counter, never below zero', () => {
    expect(optimistic(water, { delta: 1 })).toMatchObject({ value: 4, min_hit: true, hit: false })
    expect(optimistic({ ...water, value: 5 }, { delta: 1 })).toMatchObject({ value: 6, min_hit: true, hit: true })
    expect(optimistic({ ...water, value: 0 }, { delta: -1 }).value).toBe(0)
    expect(optimistic(water, { set: 2 })).toMatchObject({ value: 2, min_hit: false })
  })
  it('toggles a checkbox', () => {
    expect(optimistic(crea, { toggle: true })).toMatchObject({ value: 1, hit: true, min_hit: true })
    expect(optimistic({ ...crea, value: 1, hit: true }, { toggle: true })).toMatchObject({ value: 0, hit: false })
  })
  it('sets the weigh-in', () => {
    expect(optimistic(weigh, { set: 94.6 })).toMatchObject({ value: 94.6, hit: true })
  })
  it('returns the same habit when nothing changes', () => {
    expect(optimistic(water, { set: 3 })).toBe(water)
  })
})

suite('sync state', () => {
  const now = new Date(2026, 10, 9, 14, 0)
  const at = (h, m, day = 9) => new Date(2026, 10, day, h, m).toISOString()

  it('shows the time of the last read, with the day when it was not today', () => {
    expect(syncTime(at(10, 42), now)).toBe('10:42')
    expect(syncTime(at(22, 5, 7), now)).toBe('Sat 7 Nov 22:05')
    expect(syncTime(null, now)).toBe('never')
  })
  it('says nothing while all is well', () => {
    expect(syncNote({ state: 'ok', stale: false, pending: 0, last_sync: at(13, 59) }, now)).toBeNull()
    expect(syncNote({ state: 'ok', stale: false, pending: 2, last_sync: at(13, 59) }, now)).toBeNull()
    expect(syncNote(null, now)).toBeNull()
  })
  it('marks saved values as stale, with the time and what is waiting', () => {
    const n = syncNote({ state: 'stale', stale: true, pending: 2, last_sync: at(10, 42), error: '99-Strength is not reachable' }, now)
    expect(n.kind).toBe('stale')
    expect(n.text).toBe('Offline · values from 10:42 · 2 changes waiting')
    expect(n.title).toContain('99-Strength is not reachable')
    expect(syncNote({ state: 'stale', stale: true, pending: 1, last_sync: at(10, 42) }, now).text).toBe('Offline · values from 10:42 · 1 change waiting')
    expect(syncNote({ state: 'stale', stale: true, pending: 0, last_sync: at(10, 42) }, now).text).toBe('Offline · values from 10:42')
    expect(syncNote({ state: 'stale', stale: true, pending: 0, last_sync: null }, now).text).toBe('Offline · nothing read yet')
  })
  it('asks for a new token when 99-Strength refused the old one', () => {
    const n = syncNote({ state: 'auth', stale: true, needs_token: true, pending: 1, last_sync: at(10, 42) }, now)
    expect(n).toMatchObject({ kind: 'auth', text: 'Token needs renewing: run tcc strength connect · 1 change waiting' })
  })
  it('passes on a change 99-Strength refused', () => {
    expect(syncNote({ state: 'ok', stale: false, pending: 0, error: '99-Strength refused the change to Water (404)' }, now)).toMatchObject({ kind: 'error' })
  })
  it('explains the source label and the streak', () => {
    expect(sourceTitle({ last_sync: at(13, 59), pending: 0 }, now)).toBe('Habits come from 99-Strength. Last read 13:59. Click to refresh.')
    expect(sourceTitle({ last_sync: at(13, 59), pending: 3 }, now)).toContain('3 changes waiting to be sent')
    expect(streakTitle({ current: 12, best: 19, grace_used: false })).toContain('12-day streak, best 19')
    expect(streakTitle({ current: 12, best: 19, grace_used: true })).toContain('forgiven')
    expect(streakTitle(null)).toContain('minimum')
  })
})
