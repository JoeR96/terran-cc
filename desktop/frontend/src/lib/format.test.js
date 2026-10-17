import { describe, it, expect } from 'vitest'
import { mmss, habitValue, dayLabel, isoDate } from './format.js'

describe('mmss', () => {
  it('formats seconds', () => {
    expect(mmss(0)).toBe('00:00')
    expect(mmss(1453)).toBe('24:13')
    expect(mmss(3600)).toBe('60:00')
    expect(mmss(-5)).toBe('00:00')
  })
})

describe('habitValue', () => {
  it('shows ml as litres', () => {
    expect(habitValue({ type: 'amount', unit: 'ml', value: 1750, target: 3000 })).toBe('1.75/3L')
    expect(habitValue({ type: 'amount', unit: 'ml', value: 0, target: 3000 })).toBe('0/3L')
  })
  it('shows other units after the target', () => {
    expect(habitValue({ type: 'amount', unit: 'g', value: 112, target: 180 })).toBe('112/180g')
    expect(habitValue({ type: 'count', unit: '', value: 1, target: 2 })).toBe('1/2')
  })
})

describe('dates', () => {
  it('labels and iso-formats local dates', () => {
    const d = new Date(2026, 9, 17, 23, 30)
    expect(dayLabel(d)).toBe('Sat 17 Oct')
    expect(isoDate(d)).toBe('2026-10-17')
  })
})
