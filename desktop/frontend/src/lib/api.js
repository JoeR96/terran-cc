// Talks to tccd. In the desktop app the shell hands over the address and
// token at /shell/conn; in a browser (dev) they come from ?base=&token=.

let c = null

export async function connect() {
  try {
    const r = await fetch('/shell/conn')
    if (r.ok && r.headers.get('content-type')?.includes('json')) c = { ...(await r.json()), shell: true }
  } catch {}
  if (!c) {
    const q = new URLSearchParams(location.search)
    c = { base: q.get('base') || 'http://127.0.0.1:47821', token: q.get('token') || '', shell: false }
  }
  c.win = new URLSearchParams(location.hash.split('?')[1] || '').get('win') || ''
  return c
}

export const conn = () => c

export class APIError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

export async function api(method, path, body) {
  const headers = { Authorization: 'Bearer ' + c.token }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const r = await fetch(c.base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) })
  const text = await r.text()
  const data = text ? JSON.parse(text) : null
  if (!r.ok) throw new APIError(r.status, data?.error || r.statusText)
  return data
}

export const fileURL = (id, name) =>
  `${c.base}/api/items/${id}/attachments/${encodeURIComponent(name)}?token=${encodeURIComponent(c.token)}`

// osrsIcon is an OSRS Wiki image, fetched and cached by tccd.
export const osrsIcon = name => `${c.base}/api/osrs/icon/${encodeURIComponent(name)}?token=${encodeURIComponent(c.token)}`

export async function upload(id, file) {
  const fd = new FormData()
  fd.append('file', file, file.name || 'pasted.png')
  const r = await fetch(`${c.base}/api/items/${id}/attachments`, {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + c.token },
    body: fd,
  })
  const data = await r.json()
  if (!r.ok) throw new APIError(r.status, data?.error || r.statusText)
  return data
}

const EVENTS = ['item.changed', 'project.changed', 'habit.changed', 'timer.tick', 'timer.phase', 'ui.changed', 'osrs.changed', 'osrs.ready']

// listen streams tccd events. onOpen runs on every (re)connect so callers can
// reload anything they missed while the service was away.
export function listen(onEvent, onOpen, onDown) {
  let es, retry
  const open = () => {
    es = new EventSource(`${c.base}/api/events?token=${encodeURIComponent(c.token)}`)
    es.onopen = () => onOpen()
    es.onerror = () => {
      es.close()
      onDown()
      clearTimeout(retry)
      retry = setTimeout(open, 2000)
    }
    for (const t of EVENTS) es.addEventListener(t, e => onEvent(JSON.parse(e.data)))
  }
  open()
  return () => {
    clearTimeout(retry)
    es?.close()
  }
}

// shell asks the desktop shell to do window things. Outside the app it falls
// back to browser tabs so the UI still works in dev.
export async function shell(action, spec = {}) {
  if (!c.shell) {
    if (action === 'open') window.open(location.pathname + location.search + '#' + hashFor(spec), '_blank')
    if (action === 'close') window.close()
    return null
  }
  const q = new URLSearchParams({ win: c.win, spec: JSON.stringify(spec) })
  const r = await fetch(`/shell/${action}?${q}`)
  return r.ok ? r.json().catch(() => null) : null
}

export function hashFor(spec) {
  if (spec.kind === 'side') return '/side'
  if (spec.kind === 'widget') {
    const w = spec.widget
    return `/widget/${w.type}` + (w.project_id ? `?project=${w.project_id}` : '')
  }
  return `/board/${spec.board_id || 'main'}`
}
