import { describe, it, expect } from 'vitest'
import { left, runText, short, skillBar, stockBar, gearGoals } from './osrs.js'

describe('osrs helpers', () => {
  it('formats time left', () => {
    expect(left(59)).toBe('1m')
    expect(left(3600 + 12 * 60)).toBe('1h 12m')
    expect(left(26 * 3600)).toBe('1d 2h')
    expect(left(0)).toBe('now')
  })
  it('describes a run', () => {
    const now = 1000
    expect(runText({ state: 'ready', ready: 4, total: 4 }, now)).toBe('ready')
    expect(runText({ state: 'growing', ready_at: now + 1800 }, now)).toBe('30m')
    expect(runText({ state: 'growing', ready_at: now - 5 }, now)).toBe('ready')
    expect(runText({ state: 'unknown' }, now)).toBe('not seen yet')
  })
  it('shortens big numbers', () => {
    expect(short(950)).toBe('950')
    expect(short(1500)).toBe('1.5k')
    expect(short(20000)).toBe('20k')
    expect(short(1_500_160)).toBe('1.50M')
  })
  it('splits a skill bar into start and gain', () => {
    // Level 1 is 0 XP; a target with start 50 and now 75 of goal 100.
    expect(skillBar({ start_xp: 50, xp: 75, goal_xp: 100 })).toEqual({ start: 50, gain: 25 })
    expect(skillBar({ start_xp: 50, xp: 150, goal_xp: 100 })).toEqual({ start: 50, gain: 50 })
  })
  it('places stock against its target', () => {
    expect(stockBar({ have: 600, sufficient: 60, target: 1200 })).toEqual({ fill: 50, mark: 5 })
    expect(stockBar({ have: 5000, sufficient: 60, target: 1200 })).toEqual({ fill: 100, mark: 5 })
    expect(stockBar({ have: 1, sufficient: 0, target: 0 })).toEqual({ fill: 100, mark: 0 })
  })
  it('counts gear goal pieces from OSRS to-dos', () => {
    const items = [
      { id: 1, title: 'Voidwaker', context: 'osrs', when: 'week', parent_id: null, status: 'backlog' },
      { id: 2, title: 'Hilt', context: 'osrs', parent_id: 1, status: 'done' },
      { id: 3, title: 'Gem', context: 'osrs', parent_id: 1, status: 'backlog' },
      { id: 4, title: 'Colosseum', context: 'osrs', when: 'month', parent_id: null, status: 'backlog' },
      { id: 5, title: 'Walk', context: 'irl', parent_id: null, status: 'backlog' },
      { id: 6, title: 'Old', context: 'osrs', parent_id: null, status: 'done' },
    ]
    expect(gearGoals(items)).toEqual([
      { id: 1, title: 'Voidwaker', when: 'week', done: 1, total: 2 },
      { id: 4, title: 'Colosseum', when: 'month', done: 0, total: 1 },
    ])
  })
})
