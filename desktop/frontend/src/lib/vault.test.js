import { describe, it, expect } from 'vitest'
import { noteParts, embeds } from './vault.js'

const files = [
  'Pasted image 20261022223151.png',
  'Hobbies/OSRS/shot.png',
  'Projects/BDTV Bonanza/local.png',
]

describe('noteParts', () => {
  it('splits a note around a wikilink image resolved by file name', () => {
    expect(noteParts('Big Daves Bonanza.md', 'before ![[Pasted image 20261022223151.png]] after', files)).toEqual([
      { kind: 'text', text: 'before ' },
      { kind: 'image', path: 'Pasted image 20261022223151.png' },
      { kind: 'text', text: ' after' },
    ])
  })

  it('resolves a nested file named only by its file name', () => {
    expect(noteParts('Home.md', '![[shot.png]]', files)).toEqual([
      { kind: 'image', path: 'Hobbies/OSRS/shot.png' },
    ])
  })

  it('ignores a wikilink size suffix', () => {
    expect(noteParts('Home.md', '![[shot.png|100]]', files)).toEqual([
      { kind: 'image', path: 'Hobbies/OSRS/shot.png' },
    ])
  })

  it('resolves a markdown image relative to the note', () => {
    expect(noteParts('Projects/BDTV Bonanza/note.md', 'see ![x](local.png)', files)).toEqual([
      { kind: 'text', text: 'see ' },
      { kind: 'image', path: 'Projects/BDTV Bonanza/local.png' },
    ])
  })

  it('leaves a missing image as text', () => {
    expect(noteParts('Home.md', '![[nope.png]]', files)).toEqual([
      { kind: 'text', text: '![[nope.png]]' },
    ])
  })

  it('does not guess when two files share a name', () => {
    expect(noteParts('Home.md', '![[shot.png]]', ['a/shot.png', 'b/shot.png'])).toEqual([
      { kind: 'text', text: '![[shot.png]]' },
    ])
  })
})

describe('embeds', () => {
  it('finds image embeds with their positions and resolved paths', () => {
    const files = ['Hobbies/OSRS/map.png', 'pic.jpg']
    const text = 'a ![[map.png]] b ![x](pic.jpg) c ![[nope.png]] ![[doc.md]]'
    const got = embeds('Hobbies/OSRS/goals.md', text, files)
    expect(got).toEqual([
      { from: 2, to: 14, path: 'Hobbies/OSRS/map.png' },
      { from: 17, to: 30, path: 'pic.jpg' },
    ])
  })
})
