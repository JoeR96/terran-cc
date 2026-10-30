const IMAGE_EXT = /\.(png|jpe?g|gif|webp|svg|bmp|avif)$/i
const EMBED = /!\[\[([^\]|]+)(?:\|[^\]]*)?\]\]|!\[[^\]]*\]\(([^)]+)\)/g

function resolveImage(notePath, ref, files) {
  const clean = decodeURIComponent(ref).replaceAll('\\', '/').split('?')[0].split('#')[0]
  if (!IMAGE_EXT.test(clean)) return ''
  const list = files || []
  const dir = notePath.includes('/') ? notePath.slice(0, notePath.lastIndexOf('/')) : ''
  const found = []
  const add = p => {
    if (list.includes(p) && !found.includes(p)) found.push(p)
  }
  add(clean)
  if (dir) add(`${dir}/${clean}`)
  if (!clean.includes('/')) {
    for (const f of list) {
      if (f === clean || f.endsWith('/' + clean)) add(f)
    }
  }
  return found.length === 1 ? found[0] : ''
}

export function noteParts(notePath, text, files) {
  const src = text || ''
  const parts = []
  let last = 0
  for (const m of src.matchAll(EMBED)) {
    const path = resolveImage(notePath, (m[1] ?? m[2]).trim(), files)
    if (!path) continue
    if (m.index > last) parts.push({ kind: 'text', text: src.slice(last, m.index) })
    parts.push({ kind: 'image', path })
    last = m.index + m[0].length
  }
  if (last < src.length) parts.push({ kind: 'text', text: src.slice(last) })
  return parts
}

// embeds lists each image embed in a note with where it sits, for the
// editor to draw the image under that line.
export function embeds(notePath, text, files) {
  const out = []
  for (const m of (text || '').matchAll(EMBED)) {
    const path = resolveImage(notePath, (m[1] ?? m[2]).trim(), files)
    if (path) out.push({ from: m.index, to: m.index + m[0].length, path })
  }
  return out
}

// A name typed for a new or renamed vault entry: one path segment, not
// hidden.
function cleanName(name) {
  const n = (name || '').trim()
  return !n || n.startsWith('.') || /[\\/:*?"<>|]/.test(n) ? '' : n
}

const join = (dir, name) => (dir ? `${dir}/${name}` : name)
const parentOf = p => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '')

// newPath is where a new note or folder called name goes in dir, or ''
// when the name won't do.
export function newPath(dir, name, kind) {
  let n = cleanName(name)
  if (!n) return ''
  if (kind === 'note' && !/\.md$/i.test(n)) n += '.md'
  return join(dir, n)
}

// renamed is path with its last segment renamed, keeping a file's
// extension.
export function renamed(path, name, dir) {
  let n = cleanName(name)
  if (!n) return ''
  const ext = dir ? '' : (path.match(/\.[^./]+$/) || [''])[0]
  if (ext && !n.toLowerCase().endsWith(ext.toLowerCase())) n += ext
  return join(parentOf(path), n)
}

// folderOf is the folder new notes go in when entry is selected.
export function folderOf(entry) {
  if (!entry) return ''
  return entry.dir ? entry.path : parentOf(entry.path)
}

// moved is p after from was renamed to to.
export function moved(p, from, to) {
  if (p === from) return to
  return p.startsWith(from + '/') ? to + p.slice(from.length) : p
}

// untitled is a note name not yet used in dir.
export function untitled(dir, paths) {
  const taken = new Set(paths.map(p => p.toLowerCase()))
  for (let i = 0; ; i++) {
    const name = i ? `Untitled ${i}` : 'Untitled'
    if (!taken.has(join(dir, name + '.md').toLowerCase())) return name
  }
}
