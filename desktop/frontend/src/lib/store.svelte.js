import { api, listen } from './api.js'
import { isoDate } from './format.js'

// db is the live copy of tccd's state that every view reads from.
export const db = $state({
  ready: false,
  online: false,
  error: '',
  date: '',
  streak: 0,
  habits: [],
  items: [],
  projects: [],
  timer: null,
  boards: [],
  notes: '', // the side widget scratchpad
  phaseEnded: 0, // bumps on timer.phase so a view can chime
})

const loaders = {
  async habits() {
    const h = await api('GET', '/api/habits')
    db.date = h.date
    db.streak = h.streak
    db.habits = h.habits
  },
  async items() {
    db.items = await api('GET', '/api/items')
  },
  async projects() {
    db.projects = await api('GET', '/api/projects')
  },
  async timer() {
    db.timer = await api('GET', '/api/timer')
  },
  async notes() {
    db.notes = (await api('GET', '/api/ui/notes')) || ''
  },
  async boards() {
    db.boards = (await api('GET', '/api/ui/boards')) || defaultBoards()
  },
}

const pending = {}
function reload(what) {
  clearTimeout(pending[what])
  pending[what] = setTimeout(() => loaders[what]().catch(fail), 60)
}

export function fail(e) {
  const msg = e?.message || String(e)
  db.error = msg
  setTimeout(() => {
    if (db.error === msg) db.error = ''
  }, 5000)
}

export async function loadAll() {
  await Promise.all(Object.values(loaders).map(f => f()))
  db.ready = true
}

export function start() {
  let day = isoDate(new Date())
  // Habits and Today reset at local midnight; reload when the date changes.
  setInterval(() => {
    const now = isoDate(new Date())
    if (now !== day) {
      day = now
      reload('habits')
      reload('items')
    }
  }, 30000)
  return listen(
    e => {
      switch (e.type) {
        case 'item.changed': reload('items'); break
        case 'project.changed': reload('projects'); break
        case 'habit.changed': reload('habits'); break
        case 'timer.tick': db.timer = e.data; break
        case 'timer.phase': db.timer = e.data; db.phaseEnded++; break
        case 'ui.changed': if (e.data === 'boards' || e.data === 'notes') reload(e.data); break
      }
    },
    () => {
      db.online = true
      loadAll().catch(fail)
    },
    () => {
      db.online = false
    },
  )
}

export function defaultBoards() {
  return [
    {
      id: 'main',
      name: 'Main',
      widgets: [
        { id: 'w1', type: 'today', x: 0, y: 0, w: 4, h: 6 },
        { id: 'w2', type: 'timer', x: 4, y: 0, w: 3, h: 3 },
        { id: 'w3', type: 'habits', x: 4, y: 3, w: 3, h: 3 },
        { id: 'w4', type: 'inprogress', x: 7, y: 0, w: 5, h: 3 },
        { id: 'w5', type: 'inbox', x: 7, y: 3, w: 5, h: 3 },
        { id: 'w6', type: 'week', x: 0, y: 6, w: 4, h: 4 },
        { id: 'w7', type: 'month', x: 4, y: 6, w: 3, h: 4 },
        { id: 'w8', type: 'projects', x: 7, y: 6, w: 5, h: 4 },
      ],
    },
  ]
}

export function saveBoards(boards) {
  db.boards = boards
  return api('PUT', '/api/ui/boards', $state.snapshot(boards)).catch(fail)
}

// --- actions: each calls tccd; the event stream refreshes everyone else ---

export const act = {
  logHabit: (h, op) =>
    api('POST', `/api/habits/${h.id}/log`, op)
      .then(d => {
        const i = db.habits.findIndex(x => x.id === h.id)
        if (i >= 0) db.habits[i] = d
      })
      .catch(fail),
  addItem: n => api('POST', '/api/items', n).catch(fail),
  edit: (id, patch) => api('PATCH', `/api/items/${id}`, patch).catch(fail),
  done: id => api('POST', `/api/items/${id}/done`).catch(fail),
  reopen: id => api('POST', `/api/items/${id}/reopen`).catch(fail),
  remove: id => api('DELETE', `/api/items/${id}`).catch(fail),
  note: (id, text) => api('POST', `/api/items/${id}/note`, { text }).catch(fail),
  addProject: p => api('POST', '/api/projects', p).catch(fail),
  editProject: (id, p) => api('PATCH', `/api/projects/${id}`, p).catch(fail),
  timer: (action, body) =>
    api('POST', `/api/timer/${action}`, body)
      .then(t => (db.timer = t))
      .catch(fail),
}

// --- selectors ---

const doneToday = it => it.status === 'done' && it.done_at && isoDate(new Date(it.done_at)) === isoDate(new Date())

// bucket returns a "when" list: open items, then ones finished today.
export function bucket(when) {
  return db.items
    .filter(it => it.when === when && it.status !== 'someday' && (it.status !== 'done' || doneToday(it)))
    .sort((a, b) => (a.status === 'done') - (b.status === 'done') || a.id - b.id)
}

export const projectOf = id => db.projects.find(p => p.id === id)
export const itemTitle = id => db.items.find(i => i.id === id)?.title || ''
export const openItems = projectId => db.items.filter(i => i.project_id === projectId && i.status !== 'done')
