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
