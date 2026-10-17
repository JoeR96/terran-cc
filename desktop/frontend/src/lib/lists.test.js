import { describe, it, expect } from 'vitest'
import { todos, goals, buildTree, ACTIVE } from './lists.js'

const TODAY = '2026-10-17'
const item = (id, o = {}) => ({ id, title: 't' + id, kind: 'task', project_id: null, parent_id: null, status: 'backlog', when: 'anytime', context: 'irl', done_at: null, ...o })

describe('todos', () => {
  const items = [
    item(1, { when: 'today' }),
    item(2, { when: 'today', context: 'work' }),
    item(3, { when: 'today', project_id: 9, context: 'work' }), // a goal, not a to-do
    item(4, { when: 'today', status: 'done', done_at: '2026-10-17T09:00:00Z' }),
    item(5, { when: 'today', status: 'done', done_at: '2026-10-10T09:00:00Z' }),
    item(6, { when: 'week', context: 'work' }),
    item(7, { when: 'today', status: 'someday' }),
  ]
  it('splits personal and work to-dos and leaves out project tickets', () => {
    expect(todos(items, 'today', 'irl', TODAY).map(i => i.id)).toEqual([1, 4])
    expect(todos(items, 'today', 'work', TODAY).map(i => i.id)).toEqual([2])
    expect(todos(items, 'week', 'work', TODAY).map(i => i.id)).toEqual([6])
  })
  it('lists open ones before ones finished today', () => {
    const r = todos([item(1, { when: 'today', status: 'done', done_at: '2026-10-17T08:00:00Z' }), item(2, { when: 'today' })], 'today', 'irl', TODAY)
    expect(r.map(i => i.id)).toEqual([2, 1])
  })
})

describe('goals', () => {
  it('lists project tickets set for today, then week, then month', () => {
    const items = [
      item(1, { project_id: 1, when: 'month' }),
      item(2, { project_id: 1, when: 'today' }),
      item(3, { project_id: 1, when: 'week' }),
      item(4, { project_id: 1, when: 'anytime' }),
      item(5, { when: 'today' }), // personal to-do
      item(6, { project_id: 1, when: 'today', status: 'done', done_at: '2026-10-01T08:00:00Z' }),
      item(7, { project_id: 1, when: 'week', status: 'someday' }),
    ]
    expect(goals(items, TODAY).map(i => i.id)).toEqual([2, 3, 1])
  })
})

describe('buildTree', () => {
  const projects = [
    { id: 1, name: 'Bonanza', archived: false },
    { id: 2, name: 'Empty', archived: false },
    { id: 3, name: 'Old', archived: true },
  ]
  const items = [
    item(10, { project_id: 1, kind: 'feature', status: 'in_progress', title: 'Weapons' }),
    item(11, { project_id: 1, parent_id: 10, status: 'review' }),
    item(12, { project_id: 1, parent_id: 10, status: 'backlog' }), // hidden by the filter
    item(13, { project_id: 1, status: 'in_progress' }), // no feature
    item(14, { project_id: 1, kind: 'feature', status: 'in_progress', title: 'Idle feature' }),
    item(15, { project_id: 1, parent_id: 14, status: 'done' }),
    item(16, { project_id: 2, status: 'backlog' }),
    item(17, { project_id: 3, status: 'in_progress' }),
    item(18, { status: 'in_progress' }), // personal
  ]
  it('nests tickets under their feature and their project', () => {
    const t = buildTree(projects, items, ACTIVE)
    expect(t.map(p => [p.id, p.count])).toEqual([[1, 2]])
    const [weapons, loose] = t[0].children
    expect(weapons.item.id).toBe(10)
    expect(weapons.children.map(c => c.item.id)).toEqual([11])
    expect(weapons.count).toBe(1)
    expect(loose.item.id).toBe(13)
    expect(loose.children).toEqual([])
  })
  it('shows everything open when asked', () => {
    const t = buildTree(projects, items, ['inbox', 'backlog', 'up_next', 'in_progress', 'review'])
    expect(t.map(p => [p.id, p.count])).toEqual([[1, 3], [2, 1]])
  })
  it('shows a feature with no sub-tickets as a ticket', () => {
    const t = buildTree(projects, [item(30, { project_id: 1, kind: 'feature', status: 'review' })], ACTIVE)
    expect(t[0].children).toEqual([{ item: expect.objectContaining({ id: 30 }), count: 0, children: [] }])
  })
  it('keeps a ticket whose parent is not a feature under the project', () => {
    const t = buildTree(projects, [item(20, { project_id: 1, parent_id: 99, status: 'review' })], ACTIVE)
    expect(t[0].children.map(c => c.item.id)).toEqual([20])
  })
})
