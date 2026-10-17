<script>
  import { db } from '../lib/store.svelte.js'

  // Plays a short two-note chime whenever a timer phase ends.
  let seen = db.phaseEnded

  $effect(() => {
    if (db.phaseEnded === seen) return
    seen = db.phaseEnded
    try {
      const ctx = new AudioContext()
      ;[660, 880].forEach((f, i) => {
        const o = ctx.createOscillator()
        const g = ctx.createGain()
        o.frequency.value = f
        g.gain.setValueAtTime(0.0001, ctx.currentTime + i * 0.22)
        g.gain.exponentialRampToValueAtTime(0.25, ctx.currentTime + i * 0.22 + 0.02)
        g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + i * 0.22 + 0.5)
        o.connect(g).connect(ctx.destination)
        o.start(ctx.currentTime + i * 0.22)
        o.stop(ctx.currentTime + i * 0.22 + 0.55)
      })
      setTimeout(() => ctx.close(), 1500)
    } catch {}
  })
</script>
