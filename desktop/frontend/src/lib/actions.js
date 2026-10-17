// Svelte actions for inputs that appear on demand.
export const focus = el => { el.focus() }
export const select = el => { el.focus(); el.select() }
