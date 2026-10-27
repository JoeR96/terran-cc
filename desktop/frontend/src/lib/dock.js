// The side widget's dockable layout: columns side by side, each a stack of
// tab groups, each group showing one of its panels. Panels move by drag:
// into a group's tab bar, below a group (stacking it), or into a new column.
// Sizes are percentages: columns across the row, groups down a column.

export const PANELS = ['todos', 'projects', 'osrs', 'claude', 'scratch']

export const LABELS = { todos: 'To-dos', projects: 'Projects', osrs: 'OSRS', claude: 'Claude', scratch: 'Scratch' }

export function defaultLayout() {
  return {
    cols: [
      { size: 27, groups: [{ id: 'g1', panels: ['todos'], active: 'todos', size: 100 }] },
      { size: 40, groups: [{ id: 'g2', panels: ['projects', 'osrs'], active: 'projects', size: 100 }] },
      { size: 33, groups: [{ id: 'g3', panels: ['claude', 'scratch'], active: 'claude', size: 100 }] },
    ],
  }
}

const clone = l => JSON.parse(JSON.stringify(l))
const scaled = items => {
  const total = items.reduce((n, it) => n + (it.size > 0 ? it.size : 0), 0)
  items.forEach(it => (it.size = total > 0 && it.size > 0 ? (100 * it.size) / total : 100 / items.length))
}

function nextId(l) {
  const n = l.cols.flatMap(c => c.groups.map(g => parseInt(String(g.id).slice(1), 10) || 0))
  return 'g' + (Math.max(0, ...n) + 1)
}

// tidy drops empty groups and columns, fixes active tabs and refits sizes.
function tidy(l) {
  for (const c of l.cols) {
    c.groups = c.groups.filter(g => g.panels.length)
    for (const g of c.groups) if (!g.panels.includes(g.active)) g.active = g.panels[0]
    scaled(c.groups)
  }
  l.cols = l.cols.filter(c => c.groups.length)
  scaled(l.cols)
  return l
}

export function where(l, panel) {
  for (let ci = 0; ci < l.cols.length; ci++) {
    const gi = l.cols[ci].groups.findIndex(g => g.panels.includes(panel))
    if (gi >= 0) return { col: ci, group: gi }
  }
  return null
}

function findGroup(l, id) {
  for (const c of l.cols) {
    const g = c.groups.find(g => g.id === id)
    if (g) return { col: c, group: g }
  }
  return null
}

// move puts panel somewhere new. to is one of:
//   { group: id, at: index }  into that group's tabs
//   { below: id }             in a new group under that one
//   { newCol: index }         in a new column at that position
export function move(layout, panel, to) {
  const l = clone(layout)
  const from = where(l, panel)
  if (!from) return layout
  const src = l.cols[from.col].groups[from.group]
  const srcIndex = src.panels.indexOf(panel)
  src.panels.splice(srcIndex, 1)

  if (to.group) {
    const t = findGroup(l, to.group)
    if (!t) return layout
    let at = Math.max(0, Math.min(to.at ?? t.group.panels.length, t.group.panels.length))
    if (t.group === src && srcIndex < at) at-- // the panel left a gap before the drop point
    t.group.panels.splice(at, 0, panel)
    t.group.active = panel
  } else if (to.below) {
    const t = findGroup(l, to.below)
    if (!t) return layout
    const g = { id: nextId(l), panels: [panel], active: panel, size: t.group.size / 2 }
    t.group.size /= 2
    t.col.groups.splice(t.col.groups.indexOf(t.group) + 1, 0, g)
  } else if (to.newCol != null) {
    const col = { size: 100 / (l.cols.length + 1), groups: [{ id: nextId(l), panels: [panel], active: panel, size: 100 }] }
    l.cols.forEach(c => (c.size *= l.cols.length / (l.cols.length + 1)))
    l.cols.splice(Math.max(0, Math.min(to.newCol, l.cols.length)), 0, col)
  } else {
    return layout
  }
  return tidy(l)
}

export function activate(layout, groupId, panel) {
  const l = clone(layout)
  const t = findGroup(l, groupId)
  if (t && t.group.panels.includes(panel)) t.group.active = panel
  return l
}

// resize sets the sizes of a column's groups, or of the columns.
export function resize(layout, colIndex, sizes) {
  const l = clone(layout)
  const items = colIndex == null ? l.cols : l.cols[colIndex]?.groups
  if (!items || items.length !== sizes.length) return layout
  items.forEach((it, i) => (it.size = sizes[i]))
  return l
}

// normalize repairs a saved layout: unknown or repeated panels go, missing
// ones join the last group, and ids and sizes are made sane.
export function normalize(saved) {
  if (!saved || !Array.isArray(saved.cols) || !saved.cols.length) return defaultLayout()
  const l = { cols: [] }
  const seen = new Set()
  for (const c of saved.cols) {
    if (!c || !Array.isArray(c.groups)) continue
    const col = { size: Number(c.size) || 0, groups: [] }
    for (const g of c.groups) {
      const panels = (g?.panels || []).filter(p => PANELS.includes(p) && !seen.has(p) && seen.add(p))
      col.groups.push({ id: String(g?.id || ''), panels, active: g?.active, size: Number(g?.size) || 0 })
    }
    l.cols.push(col)
  }
  const missing = PANELS.filter(p => !seen.has(p))
  tidy(l)
  if (!l.cols.length) return defaultLayout()
  const last = l.cols[l.cols.length - 1].groups.at(-1)
  last.panels.push(...missing)
  // Give every group a unique id.
  const ids = new Set()
  for (const c of l.cols)
    for (const g of c.groups) {
      if (!g.id || ids.has(g.id)) g.id = nextId({ cols: [{ groups: [...ids].map(id => ({ id })) }] })
      ids.add(g.id)
    }
  return l
}

