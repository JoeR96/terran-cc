// Pure list logic for the side widget: personal to-dos, project goals and
// the project › feature › ticket tree.
import { isoDate } from './format.js'

// ACTIVE is what the project tree shows by default.
export const ACTIVE = ['up_next', 'in_progress', 'review']
const HORIZON = { today: 0, week: 1, month: 2 }

const doneOn = (it, today) => it.status === 'done' && it.done_at && isoDate(new Date(it.done_at)) === today
const live = (it, today) => it.status !== 'someday' && (it.status !== 'done' || doneOn(it, today))
const openFirst = (a, b) => (a.status === 'done') - (b.status === 'done') || a.id - b.id

// todos lists personal to-dos (no project) for one "when" and one context:
// 'work', or 'irl' for everything that isn't work.
export function todos(items, when, context, today = isoDate(new Date())) {
  return items
    .filter(it => !it.project_id && it.when === when && (it.context === 'work') === (context === 'work') && live(it, today))
    .sort(openFirst)
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
