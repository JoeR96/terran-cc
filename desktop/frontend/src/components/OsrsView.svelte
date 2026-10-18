<script>
  import { db, act } from '../lib/store.svelte.js'
  import { api, osrsIcon } from '../lib/api.js'
  import { runText, short, skillBar, stockBar, gearGoals } from '../lib/osrs.js'
  import { select } from '../lib/actions.js'

  // The OSRS picture: runs from RuneLite's timers, stock from the bank
  // export, skill targets from the hiscores and goals from the OSRS to-dos.
  let { onopen = null } = $props()

  const o = $derived(db.osrs)
  const goals = $derived(gearGoals(db.items))
  const HZ = [['week', 'Short term'], ['month', 'Medium term'], ['anytime', 'Long term'], ['today', 'Today']]
  const at = t => (t ? new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '–')
  // Goal titles carry notes in brackets; the wiki file is the bare name.
  const goalIcon = title => title.replace(/\s*\(.*\)\s*/g, '').replace(/^\d+\s+/, '').trim().replace(/ /g, '_') + '.png'
  const hide = e => (e.currentTarget.style.visibility = 'hidden')

  // Run cards open to show every patch; which ones stay open is per window.
  let openRuns = $state((() => { try { return JSON.parse(localStorage.getItem('tcc.osrs.runs')) || [] } catch { return [] } })())
  function toggleRun(k) {
    openRuns = openRuns.includes(k) ? openRuns.filter(x => x !== k) : [...openRuns, k]
    try {
      localStorage.setItem('tcc.osrs.runs', JSON.stringify(openRuns))
    } catch {}
  }
  const patchText = (p, now) => (p.state === 'growing' ? runText({ state: 'growing', ready_at: p.ready_at }, now) : p.state)

  // Goals open to show their pieces; new goals and pieces are added here
  // rather than in the to-do tree.
  let openGoals = $state((() => { try { return JSON.parse(localStorage.getItem('tcc.osrs.goals')) || [] } catch { return [] } })())
  function toggleGoal(id) {
    openGoals = openGoals.includes(id) ? openGoals.filter(x => x !== id) : [...openGoals, id]
    try {
      localStorage.setItem('tcc.osrs.goals', JSON.stringify(openGoals))
    } catch {}
  }
  let adding = $state(null) // {when, parent}
  let draft2 = $state('')
  function startAdd(when, parent) {
    adding = { when, parent }
    draft2 = ''
    if (parent && !openGoals.includes(parent)) toggleGoal(parent)
  }
  async function addGoal() {
    const t = draft2.trim()
    if (!t) return (adding = null)
    draft2 = ''
    const a = adding
    await act.addItem({ title: t, kind: 'todo', ...(a.parent ? { parent_id: a.parent } : { context: 'osrs', when: a.when }) })
  }

  // Stock rows with a recipe open to list the materials in the chain.
  let openStock = $state((() => { try { return JSON.parse(localStorage.getItem('tcc.osrs.stock')) || [] } catch { return [] } })())
  function toggleStock(k) {
    openStock = openStock.includes(k) ? openStock.filter(x => x !== k) : [...openStock, k]
    try {
      localStorage.setItem('tcc.osrs.stock', JSON.stringify(openStock))
    } catch {}
  }

  let editing = $state(null) // {key, field}
  let draft = $state('')
  async function saveNumber() {
    const ed = editing
    editing = null
    const v = parseInt(String(draft).replace(/[, ]/g, ''), 10)
    if (!ed || !Number.isFinite(v) || v < 0) return
    const cfg = await api('GET', '/api/osrs/config')
    const it = cfg.stock.find(s => s.key === ed.key)
    if (!it || it[ed.field] === v) return
    // Sufficient follows the target at 5% unless it was set by hand.
    if (ed.field === 'target' && it.sufficient === Math.floor(it.target / 20)) it.sufficient = Math.floor(v / 20)
    it[ed.field] = v
    act.osrsConfig(cfg)
  }
</script>

{#snippet addRow()}
  <div class="row piece">
    <span></span>
    <input class="addin" placeholder={adding.parent ? 'Piece, e.g. "716 adamantite nails" or "Bandos hilt"' : 'New goal'} bind:value={draft2}
      onkeydown={e => { if (e.key === 'Enter') addGoal(); if (e.key === 'Escape') adding = null }}
      onblur={() => { if (!draft2.trim()) adding = null }} use:select />
  </div>
{/snippet}

{#snippet num(s, field)}
  {#if editing?.key === s.key && editing.field === field}
    <input class="numin" bind:value={draft} onblur={saveNumber} onkeydown={e => { if (e.key === 'Enter') e.currentTarget.blur(); if (e.key === 'Escape') editing = null }} use:select />
  {:else}
    <button class="ghost n" title="Click to change" onclick={() => ((editing = { key: s.key, field }), (draft = String(s[field])))}>{short(s[field])}</button>
  {/if}
{/snippet}

{#if !o}
  <div class="faint">OSRS tracking isn't available. Is tccd up to date?</div>
{:else}
  <div class="meta faint">
    {o.account} · timers {at(o.updated.timers)} · bank {at(o.updated.bank)} · skills {at(o.updated.skills)}
  </div>
  {#each o.warnings as w}<div class="warn">{w}</div>{/each}

  <h6>Daily runs</h6>
  <div class="runs">
    {#each o.runs as r (r.key)}
      {@const txt = runText(r, db.now)}
      {@const isOpen = openRuns.includes(r.key)}
      <button class="ghost run" class:ready={txt === 'ready'} class:open={isOpen} class:unknown={r.state === 'unknown'} title={r.issue || `${r.ready}/${r.total} ready`} onclick={() => toggleRun(r.key)}>
        <img src={osrsIcon(r.icon)} alt="" onerror={hide} />
        <span class="rt">
          <span class="rn">{r.name}{#if r.done_today}<span class="tick" title="Done today"> ✓</span>{/if}</span>
          <span class="rs">{txt} · {r.ready}/{r.total}{#if r.note} · {r.note}{/if}{#if r.issue} · <span class="bad">{r.issue}</span>{/if}</span>
        </span>
        <span class="caret">{isOpen ? '▾' : '▸'}</span>
      </button>
      {#if isOpen}
        <div class="patches">
          {#each r.patches as p, i (i)}
            <div class="patch {p.state}">
              <span class="pn">{p.name}</span>
              <span class="pc faint">{p.crop || '–'}</span>
              <span class="ps">{patchText(p, db.now)}</span>
            </div>
          {:else}
            <div class="faint">RuneLite hasn't seen any of these patches yet.</div>
          {/each}
        </div>
      {/if}
    {/each}
  </div>

  <h6>Stock <span>have · sufficient · target</span></h6>
  {#each o.stock as s (s.key)}
    {@const b = stockBar(s)}
    {@const mk = stockBar({ ...s, have: s.have + (s.makeable || 0) })}
    {@const canOpen = s.parts?.length > 0}
    <div class="row">
      <img src={osrsIcon(s.icon)} alt="" onerror={hide} />
      {#if canOpen}
        <button class="ghost nm gname" title="Show the materials" onclick={() => toggleStock(s.key)}><span class="caret">{openStock.includes(s.key) ? '▾' : '▸'}</span>{s.label}</button>
      {:else}
        <span class="nm" title={s.label}>{s.label}</span>
      {/if}
      <div class="bar {s.status}"><em style="width:{mk.fill}%"></em><i style="width:{b.fill}%"></i>{#if b.mark}<b style="left:{b.mark}%"></b>{/if}</div>
      <span class="vals"><strong>{short(s.have)}</strong>{#if s.makeable} <span class="good" title="More your materials could make">+{short(s.makeable)} makeable</span>{/if}{#if s.raw_have} <span class="cook" title="Raw in the bank, not counted until cooked">+{short(s.raw_have)} raw · cook</span>{/if} · {@render num(s, 'sufficient')} · {@render num(s, 'target')}</span>
    </div>
    {#if canOpen && openStock.includes(s.key)}
      <div class="parts">
        {#each s.parts as p (p.name)}
          <div class="part" class:none={!p.have}><img src={osrsIcon(p.name.replace(/ /g, '_') + '.png')} alt="" onerror={hide} /><span>{p.name}</span><strong>{short(p.have)}</strong></div>
        {/each}
      </div>
    {/if}
  {/each}

  <h6>Skills <span>start · now · goal</span></h6>
  {#each o.skills as s (s.skill + s.level)}
    {@const b = skillBar(s)}
    <div class="row">
      <img src={osrsIcon(s.icon)} alt="" onerror={hide} />
      <span class="nm">{s.skill} → {s.level}</span>
      <div class="bar skill" class:full={s.done}><span class="st" style="width:{b.start}%"></span><i style="left:{b.start}%;width:{b.gain}%"></i></div>
      <span class="vals"><strong>{s.current_level}</strong> · {s.done ? 'done' : short(s.to_go) + ' to go'}{#if s.progress} · <span class="good">+{short(s.progress)}</span>{/if}{#if s.skill === 'Prayer' && o.bones?.xp} · <span class="cook" title="{o.bones.count} bones in the bank, at a gilded altar (3.5x)">bones ≈ {short(o.bones.xp)}</span>{/if}</span>
    </div>
  {:else}
    <div class="faint">Waiting for the hiscores…</div>
  {/each}

  <h6>Goals <span>pieces</span></h6>
  {#each HZ as [w, label] (w)}
    {@const list = goals.filter(g => g.when === w)}
    <div class="hz faint">{label} <button class="ghost add" title="Add a {label.toLowerCase()} goal" onclick={() => startAdd(w, null)}>+</button></div>
    {#each list as g (g.id)}
      {@const isOpen = openGoals.includes(g.id)}
      {@const kids = db.items.filter(k => k.parent_id === g.id)}
      <div class="row goal">
        <img src={osrsIcon(goalIcon(g.title))} alt="" onerror={hide} />
        <button class="ghost nm gname" title={g.title} onclick={() => toggleGoal(g.id)}><span class="caret">{kids.length ? (isOpen ? '▾' : '▸') : ''}</span>{g.title}</button>
        <div class="bar pieces" class:full={g.done === g.total}><i style="width:{(100 * g.done) / g.total}%"></i></div>
        <span class="vals"><strong>{g.done}</strong> / {g.total} <button class="ghost open" title="Open" onclick={() => onopen?.(db.items.find(i => i.id === g.id))}>↗</button></span>
      </div>
      {#if isOpen}
        {#each kids as k (k.id)}
          {@const pc = o.pieces?.[k.id]}
          <div class="row piece" class:done={k.status === 'done'}>
            <button class="box" class:on={k.status === 'done'} aria-label="Done" onclick={() => (k.status === 'done' ? act.reopen(k.id) : act.done(k.id))}></button>
            <span class="nm" title={k.title}>{k.title}</span>
            {#if pc}
              <div class="bar pieces" class:full={pc.have >= pc.need}><i style="width:{Math.min(100, (100 * pc.have) / pc.need)}%"></i></div>
              <span class="vals"><strong>{short(pc.have)}</strong> / {short(pc.need)}</span>
            {:else}
              <span></span><span></span>
            {/if}
          </div>
        {/each}
        {#if adding?.parent === g.id}{@render addRow()}{:else}
          <div class="row piece"><span></span><button class="ghost add left" onclick={() => startAdd(g.when, g.id)}>+ piece</button></div>
        {/if}
      {/if}
    {/each}
    {#if adding && !adding.parent && adding.when === w}{@render addRow()}{/if}
  {/each}

  <h6>AFK fallbacks</h6>
  <div class="afk">{#each o.afk as a}<span>{a}</span>{/each}</div>
{/if}

<style>
  .meta { font-size: 10.5px; margin-bottom: 4px; }
  .warn { color: var(--yellow); font-size: 11px; margin-bottom: 3px; }
  h6 { margin-top: 12px; }
  .runs { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 6px; }
  .run { display: flex; gap: 7px; align-items: center; background: var(--panel-2); border: 1px solid var(--line); border-radius: 6px; padding: 5px 7px; text-align: left; color: var(--text); }
  .run:hover { background: var(--card); }
  .run.open { border-style: dashed; }
  .run .caret { margin-left: auto; color: var(--muted); font-size: 10px; }
  .rt { display: flex; flex-direction: column; }
  .patches { grid-column: 1 / -1; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; padding: 4px 8px; display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 0 14px; }
  .patch { display: grid; grid-template-columns: 1fr auto auto; gap: 8px; font-size: 11px; min-height: 19px; align-items: center; }
  .pn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ps { min-width: 52px; text-align: right; }
  .patch.ready .ps { color: var(--green); }
  .patch.empty .ps { color: var(--yellow); }
  .patch.dead .ps, .patch.diseased .ps { color: var(--red); }
  .patch.growing .ps { color: var(--muted); }
  .cook { color: var(--yellow); }
  .run.ready { border-color: var(--green); }
  .run.unknown { opacity: .55; }
  .run img { width: 24px; height: 24px; object-fit: contain; flex: none; }
  .rt { min-width: 0; }
  .rn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .rs { font-size: 10.5px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ready .rs { color: var(--green); }
  .tick { color: var(--green); }
  .bad { color: var(--red); }
  .good { color: var(--green); }
  .row { display: grid; grid-template-columns: 22px minmax(90px, 160px) 1fr auto; gap: 8px; align-items: center; min-height: 24px; }
  .row img { width: 20px; height: 20px; object-fit: contain; }
  .nm { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .gname { text-align: left; color: var(--text); padding: 0; display: flex; gap: 4px; }
  .gname:hover { background: none; color: var(--strong); }
  .gname .caret { width: 10px; color: var(--muted); flex: none; }
  .piece { padding-left: 30px; grid-template-columns: 14px minmax(90px, 1fr) 120px auto; }
  .piece.done .nm { text-decoration: line-through; color: var(--faint); }
  .open { padding: 0 3px; font-size: 10px; }
  .add { padding: 0 5px; font-size: 11px; }
  .add.left { justify-self: start; color: var(--faint); }
  .addin { grid-column: 2 / -1; padding: 1px 5px; }
  .bar { position: relative; height: 10px; background: var(--line); border-radius: 5px; overflow: hidden; }
  .bar i { position: absolute; top: 0; bottom: 0; left: 0; background: var(--accent); }
  .bar em { position: absolute; top: 0; bottom: 0; left: 0; background: var(--green-fill); }
  .parts { margin: 0 0 4px 30px; padding: 3px 8px; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: 0 12px; }
  .part { display: grid; grid-template-columns: 18px 1fr auto; gap: 6px; align-items: center; font-size: 11px; min-height: 20px; }
  .part img { width: 16px; height: 16px; object-fit: contain; }
  .part strong { font-weight: 600; }
  .part.none strong { color: var(--red); }
  .bar b { position: absolute; top: 0; bottom: 0; width: 2px; background: var(--red); }
  .bar.low i { background: var(--red); }
  .bar.ok i { background: var(--yellow); }
  .bar.full i, .bar.pieces.full i { background: var(--green); }
  .bar.pieces i { background: var(--blue); }
  .bar.skill .st { position: absolute; top: 0; bottom: 0; left: 0; background: var(--line-2); }
  .vals { font-size: 11px; color: var(--muted); white-space: nowrap; text-align: right; }
  .vals strong { color: var(--text); font-weight: 600; }
  .n { padding: 0 2px; font-size: 11px; color: var(--muted); }
  .numin { width: 64px; padding: 0 3px; font-size: 11px; }
  .hz { font-size: 10.5px; margin: 4px 0 1px; }
  .afk { display: flex; flex-wrap: wrap; gap: 5px; }
  .afk span { background: var(--panel-2); border: 1px solid var(--line); border-radius: 10px; padding: 1px 8px; font-size: 11px; }
</style>
