<script>
  import { onMount, onDestroy } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { termURL } from '../lib/api.js'

  // Claude Code, live. tccd runs the one session and keeps it going; this
  // only shows it, so closing the widget or the app never ends it.
  let host
  let term, fit, ws, retry, observer
  let dead = false
  let status = $state('connecting…')

  // Windows Terminal "Campbell", like the rest of the app.
  const theme = {
    background: '#0c0c0c', foreground: '#cccccc', cursor: '#fb7e14', cursorAccent: '#0c0c0c',
    selectionBackground: '#383838',
    black: '#0c0c0c', red: '#c50f1f', green: '#13a10e', yellow: '#c19c00', blue: '#0037da', magenta: '#881798', cyan: '#3a96dd', white: '#cccccc',
    brightBlack: '#767676', brightRed: '#e74856', brightGreen: '#16c60c', brightYellow: '#f9f1a5', brightBlue: '#3b78ff', brightMagenta: '#b4009e', brightCyan: '#61d6d6', brightWhite: '#f2f2f2',
  }

  const send = m => ws?.readyState === WebSocket.OPEN && ws.send(JSON.stringify(m))
  const sendSize = () => term && send({ t: 'r', c: term.cols, r: term.rows })

  function connect() {
    ws = new WebSocket(termURL())
    ws.binaryType = 'arraybuffer'
    ws.onopen = () => {
      status = ''
      term.reset() // the server replays the scrollback on every connect
      sendSize()
    }
    ws.onmessage = e => term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data))
    ws.onclose = () => {
      if (dead) return
      status = 'reconnecting…'
      clearTimeout(retry)
      retry = setTimeout(connect, 2000)
    }
  }

  onMount(() => {
    term = new Terminal({
      theme,
      fontFamily: '"CaskaydiaCove NF", "CaskaydiaCove Nerd Font", "Cascadia Code", Consolas, monospace',
      fontSize: 12,
      cursorBlink: true,
      scrollback: 5000,
      allowProposedApi: true,
    })
    fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    term.onData(d => send({ t: 'i', d }))
    observer = new ResizeObserver(() => {
      try {
        fit.fit()
        sendSize()
      } catch {}
    })
    observer.observe(host)
    connect()
  })

  onDestroy(() => {
    dead = true
    clearTimeout(retry)
    observer?.disconnect()
    ws?.close()
    term?.dispose()
  })
</script>

<div class="claude">
  <div class="term" bind:this={host}></div>
  {#if status}<div class="st faint">{status}</div>{/if}
</div>

<style>
  .claude { position: relative; flex: 1; width: 100%; min-width: 0; height: 100%; min-height: 0; display: flex; }
  .term { flex: 1; min-width: 0; min-height: 0; background: #0c0c0c; border-radius: 4px; padding: 4px; }
  .term :global(.xterm) { height: 100%; }
  .st { position: absolute; right: 8px; bottom: 6px; font-size: 10px; }
</style>
