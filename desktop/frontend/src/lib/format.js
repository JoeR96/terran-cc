const pad = n => String(n).padStart(2, '0')

export function mmss(s) {
  s = Math.max(0, Math.round(s))
  return `${pad(Math.floor(s / 60))}:${pad(s % 60)}`
}

const num = n => String(+n.toFixed(2))

// habitValue renders "value/target" the way the side widget shows it.
export function habitValue(h) {
  if (h.unit === 'ml') return `${num(h.value / 1000)}/${num(h.target / 1000)}L`
  return `${num(h.value)}/${num(h.target)}${h.unit || ''}`
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export const dayLabel = d => `${DAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]}`
export const isoDate = d => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`

export function timeOf(iso) {
  const d = new Date(iso)
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}
