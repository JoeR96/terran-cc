// Hash routing: #/side, #/board/<id>, #/project/<id>, #/widget/<type>?project=<id>.
// The ?win= query names the window for the shell and survives navigation.

const parse = () => {
  const [path, query = ''] = location.hash.replace(/^#/, '').split('?')
  return { parts: path.split('/').filter(Boolean), query: new URLSearchParams(query) }
}

export const route = $state(parse())

addEventListener('hashchange', () => Object.assign(route, parse()))

export function go(path) {
  const win = route.query.get('win')
  location.hash = path + (win ? (path.includes('?') ? '&' : '?') + 'win=' + win : '')
}
