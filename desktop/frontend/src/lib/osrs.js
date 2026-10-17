// Display maths for the OSRS tab: countdowns, bar fills and goal pieces.

export function left(secs) {
  if (secs <= 0) return 'now'
  const m = Math.ceil(secs / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ${m % 60}m`
  return `${Math.floor(h / 24)}d ${h % 24}h`
}

// runText is what a run's card says: ready, a countdown, or not seen.
export function runText(run, now) {
  if (run.state === 'unknown') return 'not seen yet'
  if (run.state === 'growing' && run.ready_at > now) return left(run.ready_at - now)
  return 'ready'
}

export function short(n) {
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M'
  if (n >= 1e4) return Math.round(n / 1e3) + 'k'
  if (n >= 1e3) return (n / 1e3).toFixed(1).replace(/\.0$/, '') + 'k'
  return String(n)
}

const pct = (a, b) => (b > 0 ? Math.min(100, Math.max(0, (100 * a) / b)) : 100)

// skillBar splits a skill bar into where the target started and what was
// gained since, both as percentages of the goal XP.
export function skillBar(s) {
  const start = pct(s.start_xp, s.goal_xp)
  return { start, gain: Math.max(0, pct(s.xp, s.goal_xp) - start) }
}

// stockBar is how full an item is against its target, and where the
// "sufficient" mark sits.
export function stockBar(s) {
  return { fill: pct(s.have, s.target), mark: s.target > 0 ? pct(s.sufficient, s.target) : 0 }
}

// gearGoals lists open top-level OSRS to-dos with how many of their sub
// to-dos are done (a goal with none counts as one piece).
export function gearGoals(items) {
  return items
    .filter(it => it.context === 'osrs' && !it.parent_id && it.status !== 'done' && it.status !== 'someday')
    .map(g => {
      const kids = items.filter(k => k.parent_id === g.id)
      return {
        id: g.id,
        title: g.title,
        when: g.when,
        done: kids.filter(k => k.status === 'done').length,
        total: Math.max(1, kids.length),
      }
    })
}
