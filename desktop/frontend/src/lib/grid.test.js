import { describe, it, expect } from 'vitest'
import { snap, collides, place, compact } from './grid.js'

const w = (id, x, y, ww, h) => ({ id, x, y, w: ww, h })

describe('snap', () => {
  it('rounds pixels to the nearest cell', () => {
    expect(snap(0, 50)).toBe(0)
    expect(snap(24, 50)).toBe(0)
    expect(snap(26, 50)).toBe(1)
    expect(snap(-30, 50)).toBe(0) // never negative
  })
})

describe('collides', () => {
  it('detects overlap and ignores touching edges', () => {
    expect(collides(w('a', 0, 0, 2, 2), w('b', 1, 1, 2, 2))).toBe(true)
    expect(collides(w('a', 0, 0, 2, 2), w('b', 2, 0, 2, 2))).toBe(false)
    expect(collides(w('a', 0, 0, 2, 2), w('a', 0, 0, 2, 2))).toBe(false) // same widget
  })
})

describe('place', () => {
  it('pushes overlapped widgets down below the moved one', () => {
    const out = place([w('a', 0, 0, 4, 2), w('b', 0, 2, 4, 2)], w('b', 0, 1, 4, 2), 12)
    const a = out.find(x => x.id === 'a'), b = out.find(x => x.id === 'b')
    expect(b).toMatchObject({ x: 0, y: 1 })
    expect(a.y).toBe(3)
  })
  it('keeps the moved widget inside the columns', () => {
    const out = place([w('a', 0, 0, 4, 2)], w('a', 10, 0, 4, 2), 12)
    expect(out[0].x).toBe(8)
  })
  it('cascades pushes', () => {
    const out = place([w('a', 0, 0, 4, 2), w('b', 0, 2, 4, 2), w('c', 0, 4, 4, 2)], w('n', 0, 0, 4, 3), 12)
    for (const p of out) for (const q of out) expect(collides(p, q)).toBe(false)
  })
})

describe('compact', () => {
  it('floats widgets up into gaps', () => {
    const out = compact([w('a', 0, 5, 4, 2), w('b', 4, 0, 4, 2), w('c', 0, 9, 2, 1)])
    expect(out.find(x => x.id === 'a').y).toBe(0)
    expect(out.find(x => x.id === 'c').y).toBe(2)
    expect(out.find(x => x.id === 'b').y).toBe(0)
  })
})
