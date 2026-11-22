import { describe, it, expect } from 'vitest'
import { onReturn } from './focus.js'

function fakes() {
  const win = new EventTarget()
  const doc = new EventTarget()
  doc.visibilityState = 'visible'
  let t = 0
  return { win, doc, now: () => t, advance: ms => (t += ms) }
}

describe('onReturn', () => {
  it('refetches on focus and when the page becomes visible', () => {
    const f = fakes()
    let n = 0
    onReturn(() => n++, { ...f, gap: 1000 })
    f.win.dispatchEvent(new Event('focus'))
    expect(n).toBe(1)
    f.advance(1500)
    f.doc.dispatchEvent(new Event('visibilitychange'))
    expect(n).toBe(2)
  })
  it('folds focus and visibility arriving together into one', () => {
    const f = fakes()
    let n = 0
    onReturn(() => n++, { ...f, gap: 1000 })
    f.doc.dispatchEvent(new Event('visibilitychange'))
    f.win.dispatchEvent(new Event('focus'))
    expect(n).toBe(1)
  })
  it('ignores the page going hidden', () => {
    const f = fakes()
    let n = 0
    onReturn(() => n++, { ...f })
    f.doc.visibilityState = 'hidden'
    f.doc.dispatchEvent(new Event('visibilitychange'))
    expect(n).toBe(0)
  })
  it('stops when asked', () => {
    const f = fakes()
    let n = 0
    const stop = onReturn(() => n++, { ...f })
    stop()
    f.win.dispatchEvent(new Event('focus'))
    expect(n).toBe(0)
  })
})
