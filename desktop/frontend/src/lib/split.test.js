import { describe, it, expect } from 'vitest'
import { drag, fit } from './split.js'

describe('split', () => {
  it('moves the divider between two panes and keeps the total', () => {
    expect(drag([30, 40, 30], 0, 10, 10)).toEqual([40, 30, 30])
    expect(drag([30, 40, 30], 1, -5, 10)).toEqual([30, 35, 35])
  })
  it('stops at the minimum size', () => {
    expect(drag([30, 40, 30], 0, 50, 10)).toEqual([60, 10, 30])
    expect(drag([30, 40, 30], 0, -50, 10)).toEqual([10, 60, 30])
  })
  it('fits saved sizes to the pane count and to 100', () => {
    expect(fit([50, 50], 3)).toEqual([100 / 3, 100 / 3, 100 / 3])
    expect(fit([20, 30], 2)).toEqual([40, 60])
    expect(fit(null, 2)).toEqual([50, 50])
  })
})
