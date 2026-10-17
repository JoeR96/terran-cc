// Pure list logic for the side widget: personal to-dos, project goals and
// the project › feature › ticket tree.
import { isoDate } from './format.js'

// ACTIVE is what the project tree shows by default.
export const ACTIVE = ['up_next', 'in_progress', 'review']
const HORIZON = { today: 0, week: 1, month: 2 }

const doneOn = (it, today) => it.status === 'done' && it.done_at && isoDate(new Date(it.done_at)) === today
const live = (it, today) => it.status !== 'someday' && (it.status !== 'done' || doneOn(it, today))
// Open before done, then by position (0 means never placed, so last), then id.
const pos = it => it.position || Infinity
const openFirst = (a, b) => (a.status === 'done') - (b.status === 'done') || pos(a) - pos(b) || a.id - b.id

const ctxOf = it => it.context || 'irl'

// todos lists personal to-dos (no project) for one "when" and one context:
// 'irl', 'work' or 'osrs'. A to-do with no context counts as irl.
export function todos(items, when, context, today = isoDate(new Date())) {
  return items.filter(it => !it.project_id && it.when === when && ctxOf(it) === context && live(it, today)).sort(openFirst)
}

// todoTree is todos() with sub to-dos nested under their parents:
// [{item, open, children}], open counting the open to-dos below. A sub to-do
// whose parent is not in the same list sits at the top level.
export function todoTree(items, when, context, today = isoDate(new Date())) {
  const list = todos(items, when, context, today)
  const ids = new Set(list.map(it => it.id))
  const node = it => {
    const children = list.filter(c => c.parent_id === it.id).map(node)
    const open = children.reduce((n, c) => n + c.open + (c.item.status !== 'done'), 0)
    return { item: it, open, children }
  }
  return list.filter(it => !it.parent_id || !ids.has(it.parent_id)).map(node)
}

// goals lists project tickets set for today, this week or this month.
export function goals(items, today = isoDate(new Date())) {
  return items
    .filter(it => it.project_id && it.when in HORIZON && live(it, today))
    .sort((a, b) => HORIZON[a.when] - HORIZON[b.when] || openFirst(a, b))
}

// buildTree returns [{id, project, count, children}] where each child is
// {item, count, children}: a feature with its tickets, or a ticket on its own.
// Only tickets whose status is in `statuses` count; empty branches drop out.
export function buildTree(projects, items, statuses) {
  const show = new Set(statuses)
  const out = []
  for (const p of projects) {
    if (p.archived) continue
    const mine = items.filter(it => it.project_id === p.id)
    // A feature groups the tickets under it; one with nothing under it is
    // just a ticket.
    const parents = new Set(mine.map(it => it.parent_id))
    const features = new Map(mine.filter(it => it.kind === 'feature' && !it.parent_id && parents.has(it.id)).map(f => [f.id, { item: f, count: 0, children: [] }]))
    const loose = []
    for (const it of mine) {
      if (features.has(it.id) || !show.has(it.status)) continue
      const f = features.get(it.parent_id)
      if (f) f.children.push({ item: it, count: 0, children: [] })
      else loose.push({ item: it, count: 0, children: [] })
    }
    const kids = [...features.values()].filter(f => f.children.length).map(f => ({ ...f, count: f.children.length }))
    const children = [...kids, ...loose]
    const count = kids.reduce((n, f) => n + f.count, 0) + loose.length
    if (count) out.push({ id: p.id, project: p, count, children })
  }
  return out
}

// moveBefore returns keys with `key` moved in front of `target` (to the end
// when target is null). An unknown key leaves the list as it was.
export function moveBefore(keys, key, target) {
  if (!keys.includes(key) || key === target) return keys
  const rest = keys.filter(k => k !== key)
  const i = target == null ? rest.length : rest.indexOf(target)
  return [...rest.slice(0, i < 0 ? rest.length : i), key, ...rest.slice(i < 0 ? rest.length : i)]
}
