// Pane sizes for the side widget's resizable splits, in percent.

// drag moves the divider after pane i by delta percent, keeping both
// neighbours at least min.
export function drag(sizes, i, delta, min) {
  const out = [...sizes]
  const pair = out[i] + out[i + 1]
  const a = Math.min(Math.max(out[i] + delta, min), pair - min)
  out[i] = a
  out[i + 1] = pair - a
  return out
}

// fit turns saved sizes into n sizes adding to 100: equal shares when the
// pane count changed, scaled otherwise.
export function fit(sizes, n) {
  if (!Array.isArray(sizes) || sizes.length !== n || sizes.some(s => !(s > 0))) return Array(n).fill(100 / n)
  const total = sizes.reduce((a, b) => a + b, 0)
  return sizes.map(s => (100 * s) / total)
}
