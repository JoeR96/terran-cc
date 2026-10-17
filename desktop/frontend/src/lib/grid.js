// Snap-grid layout maths. Widgets are {id, x, y, w, h} in grid cells.

export const snap = (px, cell) => Math.max(0, Math.round(px / cell))

export function collides(a, b) {
  if (a.id === b.id) return false
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
}

// place puts moved (a new or changed widget) where it asks to be, clamped to
// the columns, and pushes anything it overlaps further down.
export function place(widgets, moved, cols) {
  const m = { ...moved, w: Math.min(moved.w, cols) }
  m.x = Math.min(Math.max(0, m.x), cols - m.w)
  m.y = Math.max(0, m.y)
  const placed = [m]
  const rest = widgets.filter(w => w.id !== m.id).map(w => ({ ...w })).sort((a, b) => a.y - b.y || a.x - b.x)
  for (const w of rest) {
    let hit
    while ((hit = placed.find(p => collides(p, w)))) w.y = hit.y + hit.h
    placed.push(w)
  }
  return placed
}

// compact floats every widget up as far as it can go.
export function compact(widgets) {
  const placed = []
  for (const w of widgets.map(w => ({ ...w })).sort((a, b) => a.y - b.y || a.x - b.x)) {
    while (w.y > 0 && !placed.some(p => collides(p, { ...w, y: w.y - 1 }))) w.y--
    placed.push(w)
  }
  return placed
}

// firstFree finds the highest free spot for a w×h widget.
export function firstFree(widgets, w, h, cols) {
  for (let y = 0; ; y++)
    for (let x = 0; x + w <= cols; x++) {
      const c = { id: '\0', x, y, w, h }
      if (!widgets.some(p => collides(p, c))) return { x, y }
    }
}
