import { describe, it, expect } from 'vitest'
import { PANELS, defaultLayout, move, normalize, where, activate, close, reopen, closedPanels } from './dock.js'

const keys = l => l.cols.map(c => c.groups.map(g => g.panels))

describe('dock', () => {
  it('starts with to-dos + training, projects + OSRS, and Claude + scratch', () => {
    expect(keys(defaultLayout())).toEqual([[['todos', 'training']], [['projects', 'osrs']], [['claude', 'scratch']]])
  })

  it('moves a tab into another group, at a position, and makes it active', () => {
    const l = move(defaultLayout(), 'osrs', { group: 'g3', at: 0 })
    expect(keys(l)).toEqual([[['todos', 'training']], [['projects']], [['osrs', 'claude', 'scratch']]])
    expect(l.cols[2].groups[0].active).toBe('osrs')
  })

  it('reorders within a group', () => {
    const l = move(defaultLayout(), 'scratch', { group: 'g3', at: 0 })
    expect(keys(l)[2]).toEqual([['scratch', 'claude']])
  })

  it('stacks a tab below a group, splitting its column', () => {
    const l = move(defaultLayout(), 'osrs', { below: 'g1' })
    expect(keys(l)).toEqual([[['todos', 'training'], ['osrs']], [['projects']], [['claude', 'scratch']]])
    expect(l.cols[0].groups.map(g => g.size)).toEqual([50, 50])
  })

  it('makes a new column, and removes groups and columns left empty', () => {
    let l = move(move(defaultLayout(), 'training', { group: 'g2' }), 'todos', { newCol: 3 })
    expect(keys(l)).toEqual([[['projects', 'osrs', 'training']], [['claude', 'scratch']], [['todos']]])
    expect(l.cols.reduce((n, c) => n + c.size, 0)).toBeCloseTo(100)
    l = move(l, 'claude', { group: l.cols[0].groups[0].id, at: 3 })
    l = move(l, 'scratch', { group: l.cols[0].groups[0].id, at: 4 })
    expect(keys(l)).toEqual([[['projects', 'osrs', 'training', 'claude', 'scratch']], [['todos']]])
  })

  it('repairs a saved layout: missing panels come back, unknown and duplicate ones go', () => {
    const l = normalize({ cols: [{ size: 1, groups: [{ id: 'x', panels: ['todos', 'nope', 'todos'], active: 'nope', size: 1 }] }] })
    expect(l.cols.flatMap(c => c.groups.flatMap(g => g.panels)).sort()).toEqual([...PANELS].sort())
    expect(l.cols[0].groups[0].active).toBe('todos')
    expect(normalize(null)).toEqual(defaultLayout())
  })

  it('finds a panel and switches tabs', () => {
    const l = activate(defaultLayout(), 'g2', 'osrs')
    expect(where(l, 'osrs')).toEqual({ col: 1, group: 0 })
    expect(l.cols[1].groups[0].active).toBe('osrs')
  })

  it('closes a panel, keeps it closed through a reload, and reopens it', () => {
    let l = close(defaultLayout(), 'training')
    expect(keys(l)[0]).toEqual([['todos']])
    expect(closedPanels(l)).toEqual(['training'])
    l = normalize(JSON.parse(JSON.stringify(l)))
    expect(where(l, 'training')).toBe(null)
    l = reopen(l, 'training')
    expect(where(l, 'training')).toEqual({ col: 0, group: 0 })
    expect(l.cols[0].groups[0].active).toBe('training')
    expect(closedPanels(normalize(l))).toEqual([])
  })

  it("closing a group's only tab drops the group; the last panel stays open", () => {
    let l = close(close(defaultLayout(), 'todos'), 'training')
    expect(keys(l)).toEqual([[['projects', 'osrs']], [['claude', 'scratch']]])
    for (const p of ['projects', 'osrs', 'claude']) l = close(l, p)
    expect(keys(close(l, 'scratch'))).toEqual([[['scratch']]])
  })

  it('adds training to a layout saved before it existed', () => {
    const old = { cols: [{ size: 100, groups: [{ id: 'g1', panels: ['todos', 'projects', 'osrs', 'claude', 'scratch'], active: 'todos', size: 100 }] }] }
    expect(keys(normalize(old))).toEqual([[['todos', 'projects', 'osrs', 'claude', 'scratch', 'training']]])
  })
})