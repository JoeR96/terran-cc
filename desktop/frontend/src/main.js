import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { connect } from './lib/api.js'
import { start } from './lib/store.svelte.js'

const RUNTIME = '/wails/runtime.js'
let startError = ''
try {
  const c = await connect()
  // The Wails runtime makes frameless windows draggable by their headers.
  if (c.shell) import(/* @vite-ignore */ RUNTIME).catch(() => {})
  if (!c.token) startError = 'No token. In a browser, open this page with ?token=<token from daemon.json>.'
  else start()
} catch (e) {
  startError = e.message
}

mount(App, { target: document.getElementById('app'), props: { startError } })
