// fitSections decides which side-widget sections show at a given height.
// Sections stack in order; fixed ones always show. Once one section doesn't
// fit, it and everything after it fold into a single line of height foldH,
// unless the user expanded it.
export function fitSections(sections, available, foldH, expanded = []) {
  const shown = [], folded = []
  let used = 0
  for (const s of sections) if (s.fixed) { shown.push(s.key); used += s.h }
  let folding = false
  const flex = sections.filter(s => !s.fixed)
  flex.forEach((s, i) => {
    if (expanded.includes(s.key)) { shown.push(s.key); used += s.h; return }
    if (!folding) {
      const restH = flex.slice(i).reduce((n, r) => n + r.h, 0)
      if (used + restH <= available || used + s.h + foldH <= available) {
        shown.push(s.key); used += s.h; return
      }
      folding = true
    }
    folded.push(s.key)
  })
  return { shown, folded }
}
