import { describe, it, expect } from 'vitest'
import { fitSections } from './fit.js'

describe('fitSections', () => {
  const secs = [
    { key: 'timer', h: 60, fixed: true },
    { key: 'habits', h: 120, fixed: true },
    { key: 'today', h: 140 },
    { key: 'week', h: 100 },
    { key: 'month', h: 80 },
  ]
  it('shows everything when it all fits', () => {
    expect(fitSections(secs, 1000, 24)).toEqual({ shown: ['timer', 'habits', 'today', 'week', 'month'], folded: [] })
  })
  it('folds sections from the bottom, keeping room for the fold line', () => {
    // 60+120+140 = 320, + fold line 24 = 344
    expect(fitSections(secs, 350, 24)).toEqual({ shown: ['timer', 'habits', 'today'], folded: ['week', 'month'] })
  })
  it('folds in order: a later section never shows when an earlier one is folded', () => {
    const r = fitSections([...secs.slice(0, 3), { key: 'week', h: 400 }, { key: 'month', h: 10 }], 360, 24)
    expect(r.folded).toEqual(['week', 'month'])
  })
  it('always shows fixed sections even when too short', () => {
    expect(fitSections(secs, 100, 24)).toEqual({ shown: ['timer', 'habits'], folded: ['today', 'week', 'month'] })
  })
  it('expanded keys always show', () => {
    expect(fitSections(secs, 100, 24, ['week']).shown).toEqual(['timer', 'habits', 'week'])
  })
})
