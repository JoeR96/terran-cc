// onReturn calls fn when the window gets focus back or the page becomes
// visible again: whatever changed in 99-Strength meanwhile (the phone, the
// web app) is read then rather than at the next timer. Calls closer together
// than gap are folded into one. It returns a function that stops listening.
export function onReturn(fn, { win = globalThis.window, doc = globalThis.document, gap = 2000, now = Date.now } = {}) {
  let last = -Infinity
  const fire = () => {
    if (doc?.visibilityState === 'hidden') return
    const t = now()
    if (t - last < gap) return
    last = t
    fn()
  }
  win?.addEventListener('focus', fire)
  doc?.addEventListener('visibilitychange', fire)
  return () => {
    win?.removeEventListener('focus', fire)
    doc?.removeEventListener('visibilitychange', fire)
  }
}
