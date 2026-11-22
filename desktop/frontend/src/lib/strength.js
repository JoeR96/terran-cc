// Helpers for habits that tccd serves from 99-Strength (source "99-strength").
// Those habits carry more than local ones: a kind (counter, numeric,
// checkbox), a minimum that still counts on a bad day, and how many of their
// changes have not reached 99-Strength yet.

export const REMOTE = '99-strength'

const pad = n => String(n).padStart(2, '0')
const num = n => String(+(+n).toFixed(2))
const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// shortName drops a trailing "(…)" so "Water (532ml bottle)" fits a narrow row.
export const shortName = name => name.replace(/\s*\([^)]*\)\s*$/, '') || name

// counterValue renders "3/6".
export const counterValue = h => `${num(h.value)}/${num(h.target)}`

// pct is how full the bar is.
export const pct = h => Math.max(0, Math.min(100, (h.value / (h.target || 1)) * 100))

// minPct is where the minimum marker sits on the bar, or null when the
// minimum is the target (checkboxes, the weigh-in) and needs no marker.
export function minPct(h) {
  if (!(h.minimum > 0) || !(h.target > 0) || h.minimum >= h.target) return null
  return (h.minimum / h.target) * 100
}

// level is 'hit' (target reached), 'min' (minimum reached) or ''.
export const level = h => (h.hit ? 'hit' : h.min_hit ? 'min' : '')

// describe is the tooltip: "3 of 6 bottles · minimum 4".
export function describe(h) {
  const unit = h.unit ? ' ' + h.unit : ''
  if (h.kind === 'checkbox') return h.name + (h.hit ? ' · done' : ' · not done')
  if (h.kind === 'numeric') return h.name + (h.value > 0 ? ` · ${num(h.value)}${unit}` : ' · not logged')
  const min = minPct(h) === null ? '' : ` · minimum ${num(h.minimum)}`
  return `${h.name} · ${num(h.value)} of ${num(h.target)}${unit}${min}`
}

// parseAmount reads what was typed for a habit. A weigh-in follows the
// phone app: "94,6" or "94.6", 30 to 300 kg, kept to one decimal. Anything
// else is a plain number that is not negative. null means "not a value".
export function parseAmount(raw, h) {
  const text = String(raw).trim().replace(',', '.')
  if (!/^\d+([.]\d*)?$/.test(text)) return null
  const v = Number(text)
  if (!Number.isFinite(v)) return null
  if (h.kind === 'numeric' && (h.unit || '').toLowerCase() === 'kg') {
    if (v < 30 || v > 300) return null
    return Math.round(v * 10) / 10
  }
  return Math.round(v * 100) / 100
}

// optimistic is the habit as it will look once a log has landed, so a tap
// shows at once. tccd's answer replaces it.
export function optimistic(h, op) {
  let v = h.value
  if (h.kind === 'checkbox') {
    v = op.set !== undefined ? (op.set >= 1 ? 1 : 0) : v >= 1 ? 0 : 1
  } else if (op.set !== undefined) {
    v = Math.max(0, op.set)
  } else if (op.delta !== undefined) {
    v = h.kind === 'numeric' ? op.delta : Math.max(0, v + op.delta)
  }
  if (v === h.value) return h
  return { ...h, value: v, hit: v >= h.target, min_hit: v >= h.minimum }
}

// syncTime shows when habits were last read: "10:42", or "Sat 7 Nov 10:42"
// when that was not today.
export function syncTime(iso, now = new Date()) {
  if (!iso) return 'never'
  const d = new Date(iso)
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (d.toDateString() === now.toDateString()) return time
  return `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]} ${time}`
}

const waiting = n => `${n} ${n === 1 ? 'change' : 'changes'} waiting`

// syncNote is the line shown when 99-Strength needs attention, or null.
//   auth   the token was refused: the values are saved ones until it is renewed
//   stale  the API is unreachable: the values are the last ones read
//   error  99-Strength refused a change
export function syncNote(sync, now = new Date()) {
  if (!sync) return null
  const queue = sync.pending ? ' · ' + waiting(sync.pending) : ''
  if (sync.state === 'auth') {
    return {
      kind: 'auth',
      text: 'Token needs renewing: run tcc strength connect' + queue,
      title: 'Values are the last ones read. Changes are kept and sent once there is a new token.',
    }
  }
  if (sync.stale) {
    return {
      kind: 'stale',
      text: (sync.last_sync ? `Offline · values from ${syncTime(sync.last_sync, now)}` : 'Offline · nothing read yet') + queue,
      title: (sync.error ? sync.error + '. ' : '') + 'Changes are kept and sent when 99-Strength is back. Click to try now.',
    }
  }
  if (sync.error) return { kind: 'error', text: sync.error, title: sync.error }
  return null
}

// sourceTitle is the tooltip on the small "99-Strength" label.
export function sourceTitle(sync, now = new Date()) {
  if (!sync) return 'Habits come from 99-Strength'
  const queue = sync.pending ? `. ${waiting(sync.pending)} to be sent` : ''
  return `Habits come from 99-Strength. Last read ${syncTime(sync.last_sync, now)}${queue}. Click to refresh.`
}

// streakTitle explains the streak chip.
export function streakTitle(info) {
  if (!info) return 'Days in a row with every habit at its minimum or better'
  const grace = info.grace_used ? ' One missed day has been forgiven; another in a row ends it.' : ''
  return `${info.current}-day streak, best ${info.best}. A day counts when every habit reaches its minimum.${grace}`
}
