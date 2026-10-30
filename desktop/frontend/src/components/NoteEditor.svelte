<script>
  import { onMount, onDestroy } from 'svelte'
  import { EditorView, Decoration, WidgetType, keymap } from '@codemirror/view'
  import { EditorState, StateField } from '@codemirror/state'
  import { minimalSetup } from 'codemirror'
  import { markdown } from '@codemirror/lang-markdown'
  import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
  import { indentWithTab } from '@codemirror/commands'
  import { search, searchKeymap, highlightSelectionMatches } from '@codemirror/search'
  import { tags as t } from '@lezer/highlight'
  import { embeds } from '../lib/vault.js'
  import { vaultFile } from '../lib/api.js'

  // An always-editable markdown note, like Obsidian's live preview: the
  // text stays editable, headings and emphasis are styled as you type, and
  // embedded images show under their line.
  // focus puts the cursor in the note, for one just made.
  let { path, text, files = [], focus = false, onchange } = $props()

  let host
  let view

  class Image extends WidgetType {
    constructor(src) {
      super()
      this.src = src
    }
    eq(other) {
      return other.src === this.src
    }
    toDOM() {
      const img = document.createElement('img')
      img.src = this.src
      img.className = 'cm-embed'
      img.alt = ''
      return img
    }
  }

  // Image widgets are block decorations, so they live in a state field.
  const images = (notePath, list) =>
    StateField.define({
      create: st => build(st, notePath, list),
      update: (deco, tr) => (tr.docChanged ? build(tr.state, notePath, list) : deco),
      provide: f => EditorView.decorations.from(f),
    })
  function build(st, notePath, list) {
    const doc = st.doc
    const ranges = embeds(notePath, doc.toString(), list).map(e =>
      Decoration.widget({ widget: new Image(vaultFile(e.path)), block: true, side: 1 }).range(doc.lineAt(e.to).to),
    )
    return Decoration.set(ranges, true)
  }

  const style = HighlightStyle.define([
    { tag: t.heading1, fontSize: '1.5em', fontWeight: '700', color: 'var(--strong)' },
    { tag: t.heading2, fontSize: '1.3em', fontWeight: '700', color: 'var(--strong)' },
    { tag: t.heading3, fontSize: '1.15em', fontWeight: '700', color: 'var(--strong)' },
    { tag: [t.heading4, t.heading5, t.heading6], fontWeight: '700', color: 'var(--strong)' },
    { tag: t.strong, fontWeight: '700', color: 'var(--strong)' },
    { tag: t.emphasis, fontStyle: 'italic' },
    { tag: t.strikethrough, textDecoration: 'line-through', color: 'var(--faint)' },
    { tag: [t.link, t.url], color: 'var(--blue)' },
    { tag: t.monospace, color: 'var(--cyan)' },
    { tag: [t.processingInstruction, t.meta, t.contentSeparator], color: 'var(--faint)' },
    { tag: t.quote, color: 'var(--muted)', fontStyle: 'italic' },
    { tag: t.list, color: 'var(--accent)' },
  ])

  const theme = EditorView.theme(
    {
      '&': { height: '100%', background: 'var(--bg)', color: 'var(--text)', fontSize: '12.5px', borderRadius: '4px', border: '1px solid var(--line-2)' },
      '&.cm-focused': { outline: 'none', borderColor: 'var(--accent)' },
      '.cm-scroller': { fontFamily: 'var(--font)', lineHeight: '1.55' },
      '.cm-content': { padding: '8px 4px', caretColor: 'var(--accent)' },
      '.cm-cursor': { borderLeftColor: 'var(--accent)' },
      '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': { background: 'var(--btn-hover) !important' },
      '.cm-activeLine': { background: 'transparent' },
      '.cm-panels': { background: 'var(--bg)', color: 'var(--text)', borderColor: 'var(--line-2)' },
      '.cm-panel.cm-search': { fontFamily: 'var(--font)', fontSize: '12px' },
      '.cm-panel.cm-search input, .cm-panel.cm-search button': { fontFamily: 'var(--font)', fontSize: '12px' },
      '.cm-searchMatch': { background: 'rgba(193, 156, 0, 0.35)' },
      '.cm-searchMatch-selected': { background: 'rgba(193, 156, 0, 0.6)' },
      '.cm-selectionMatch': { background: 'var(--btn)' },
      '.cm-embed': { display: 'block', maxWidth: '100%', margin: '6px 0', borderRadius: '4px' },
    },
    { dark: true },
  )

  function stateFor(notePath, doc) {
    return EditorState.create({
      doc,
      extensions: [
        minimalSetup,
        keymap.of([indentWithTab, ...searchKeymap]),
        search({ top: true }),
        highlightSelectionMatches(),
        EditorView.lineWrapping,
        markdown(),
        syntaxHighlighting(style),
        theme,
        images(notePath, files),
        EditorView.updateListener.of(u => {
          if (u.docChanged) onchange?.(u.state.doc.toString())
        }),
      ],
    })
  }

  onMount(() => {
    view = new EditorView({ state: stateFor(path, text), parent: host })
    if (focus) view.focus()
  })
  onDestroy(() => view?.destroy())

  // A different note, or the text arriving after load, replaces the doc;
  // typing doesn't, since the doc already matches.
  let shown = $state(path)
  $effect(() => {
    const p = path
    const v = text
    if (!view) return
    if (p !== shown || v !== view.state.doc.toString()) {
      if (p === shown && view.hasFocus) return // mid-edit: keep the cursor
      shown = p
      view.setState(stateFor(p, v))
    }
    if (focus && !view.hasFocus) view.focus()
  })
</script>

<div class="ed" bind:this={host}></div>

<style>
  .ed { flex: 1; min-height: 0; display: flex; }
  .ed :global(.cm-editor) { flex: 1; min-width: 0; }
</style>
