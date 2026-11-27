'use strict';
// All server-provided strings are inserted with textContent / createElement,
// never as markup: profile names come from user-writable files.
const $ = id => document.getElementById(id);
let token = null;
// Native mode: running inside the desktop app's own window. There is no server URL, no sign-in and no
// network access from the page; the app injects syncampStart(), which performs the request in Go (with the
// access token attached there) and answers asynchronously through syncampDone(). The async hand-off matters:
// bound functions run on the UI thread, so a slow request must never block inside them.
const native = typeof window.syncampStart === 'function';
const pending = {}; let seq = 0;
window.syncampDone = (id, status, body) => { const p = pending[id]; delete pending[id]; if (p) p({ status, body }); };
function nativeCall(method, path, body) {
  return new Promise(resolve => { const id = ++seq; pending[id] = resolve; window.syncampStart(id, method, path, body || ''); });
}
const open = new Set();   // expanded profile rows survive the 5 s refresh
if (native) token = 'native'; else { try { token = sessionStorage.getItem('syncamp-token'); } catch (e) {} }

function el(tag, text, cls) {
  const n = document.createElement(tag);
  if (text !== undefined && text !== null) n.textContent = text;
  if (cls) n.className = cls;
  return n;
}
const valid = t => t && !t.startsWith('0001');
function ago(t) {
  if (!valid(t)) return 'never';
  const s = Math.max(0, (Date.now() - new Date(t)) / 1000);
  if (s < 45) return 'just now';
  if (s < 3600) return Math.round(s / 60) + ' min ago';
  if (s < 86400) return Math.round(s / 3600) + ' h ago';
  if (s < 172800) return 'yesterday';
  return Math.round(s / 86400) + ' days ago';
}
async function api(path, method, body) {
  if (native) {
    const r = await nativeCall(method || 'GET', path, body ? JSON.stringify(body) : '');
    if (r.status === 401) throw new Error('auth');
    try { return JSON.parse(r.body); } catch (e) { return {}; }
  }
  const headers = token ? { Authorization: 'Bearer ' + token } : {};
  if (body) headers['Content-Type'] = 'application/json';
  const r = await fetch(path, { method: method || 'GET', headers, body: body ? JSON.stringify(body) : undefined });
  if (r.status === 401) throw new Error('auth');
  return r.json();
}
function signOut(msg) {
  if (native) { // nothing to sign out of: the app holds the access itself
    $('login').hidden = false; $('app').hidden = true; $('topactions').hidden = true;
    $('loginerr').textContent = msg || "SyncAmp can't read the background service's access key on this account. Reinstall SyncAmp, or run `syncamp grant <your user name>` as an administrator.";
    return;
  }
  token = null; try { sessionStorage.removeItem('syncamp-token'); } catch (x) {}
  $('login').hidden = false; $('app').hidden = true; $('topactions').hidden = true; $('loginerr').textContent = msg || '';
}
const GLYPH = { ok: '✓', running: '↻', warn: '!', error: '✕', unknown: '·' };
const HEAD = { ok: 'All backups healthy', running: 'Backing up…', warn: 'Needs a look', error: 'Needs attention' };
function setHero(overall, summary, sub) {
  $('ring').className = 'ring ' + overall; $('glyph').textContent = GLYPH[overall] || '·';
  $('headline').textContent = HEAD[overall] || 'SyncAmp';
  $('subline').textContent = sub || summary || '';
}
const outcome = s => s === 'success' ? 'Succeeded' : s === 'warning' ? 'Finished with warnings' : s === 'timeout' ? 'Timed out' : 'Failed';

let mode = 'monitor';
const PRESETS = [['hourly', 'Hourly'], ['every6h', 'Every 6 h'], ['daily', 'Daily']];
const STALE = [['auto', 'Auto'], ['12h', '12 h'], ['24h', '24 h'], ['48h', '48 h'], ['168h', '1 week']];
async function schedule(name, body) { try { await api('/api/profiles/' + encodeURIComponent(name) + '/schedule', 'POST', body); } catch (e) {} load(); }

function staleDrawer(p) {
  const d = el('div', '', 'drawer');
  const t = el('div', '', 't'); t.appendChild(el('b', 'Alert if overdue'));
  t.appendChild(el('span', p.staleAfter ? 'No successful backup in ' + p.staleAfter + (p.staleSetting === 'auto' ? ' (from its schedule)' : '') : 'No expectation known: pick a time below'));
  d.appendChild(t);
  const chips = el('div', '', 'chips');
  STALE.forEach(([id, label]) => {
    const c = el('button', label, 'chip' + (p.staleSetting === id ? ' sel' : ''));
    c.addEventListener('click', () => schedule(p.name, { staleAfter: id }));
    chips.appendChild(c);
  });
  d.appendChild(chips);
  d.appendChild(el('p', 'Last success: ' + (p.lastSuccess ? ago(p.lastSuccess) : 'not seen yet') + '. The Syncrify client runs its own schedule; SyncAmp watches it.', 'note'));
  return d;
}
function drawer(p) {
  if (mode !== 'scheduler') return staleDrawer(p);
  const d = el('div', '', 'drawer');
  const s = el('div', '', 'setting');
  const t = el('div'); t.appendChild(el('b', 'Run on schedule'));
  t.appendChild(el('span', p.paused ? 'Paused · ' + p.schedule : p.schedule));
  s.appendChild(t);
  const sw = el('button', '', 'switch'); sw.setAttribute('role', 'switch');
  sw.setAttribute('aria-checked', String(!p.paused)); sw.setAttribute('aria-label', 'Run ' + p.name + ' on schedule');
  sw.addEventListener('click', () => schedule(p.name, { enabled: p.paused }));
  s.appendChild(sw); d.appendChild(s);
  const chips = el('div', '', 'chips');
  PRESETS.forEach(([id, label]) => {
    const c = el('button', label, 'chip' + (p.preset === id ? ' sel' : ''));
    c.addEventListener('click', () => schedule(p.name, { preset: id }));
    chips.appendChild(c);
  });
  d.appendChild(chips);
  return d;
}
function profileItem(p) {
  const li = el('li', '', 'item' + (open.has(p.name) ? ' open' : ''));
  const row = el('div', '', 'row'); row.tabIndex = 0; row.setAttribute('role', 'button');
  row.setAttribute('aria-expanded', String(open.has(p.name)));
  const state = p.running ? 'running' : p.paused ? 'paused' : p.overdue ? 'overdue' : p.last ? p.last.status : 'unknown';
  row.appendChild(el('span', '', 'dot ' + state));
  const g = el('div', '', 'grow'); g.appendChild(el('b', p.name));
  let line;
  if (p.running) line = 'Backing up now…';
  else if (p.paused) line = 'Paused';
  else if (p.overdue) line = 'Overdue · ' + (p.lastSuccess ? 'last success ' + ago(p.lastSuccess) : 'no success seen yet');
  else if (!p.last) line = p.scheduled ? 'Waiting for first run' : 'Manual · never run';
  else line = outcome(p.last.status) + ' ' + ago(p.last.end);
  g.appendChild(el('span', line));
  if (p.last && p.last.status !== 'success' && p.last.error && !p.running) g.appendChild(el('span', p.last.error, p.last.status === 'warning' ? 'warnmsg' : 'err'));
  row.appendChild(g);
  const b = el('button', '▶', 'play'); b.disabled = p.running; b.title = 'Back up now'; b.setAttribute('aria-label', 'Back up ' + p.name + ' now');
  b.addEventListener('click', async e => { e.stopPropagation(); b.disabled = true; try { await api('/api/run/' + encodeURIComponent(p.name), 'POST'); } catch (x) {} load(); });
  row.appendChild(b);
  row.appendChild(el('span', '›', 'chev'));
  const toggle = () => { open.has(p.name) ? open.delete(p.name) : open.add(p.name); load(); };
  row.addEventListener('click', toggle);
  row.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggle(); } });
  li.appendChild(row);
  if (open.has(p.name)) li.appendChild(drawer(p));
  return li;
}
function historyItem(r) {
  const li = el('li', '', 'item'); const row = el('div', '', 'row');
  row.appendChild(el('span', '', 'dot ' + r.status));
  const g = el('div', '', 'grow'); g.appendChild(el('b', r.profile));
  g.appendChild(el('span', outcome(r.status) + ' · ' + ago(r.end) + ' · ' + r.trigger));
  if (r.error) g.appendChild(el('span', r.error, r.status === 'warning' ? 'warnmsg' : 'err'));
  row.appendChild(g); li.appendChild(row); return li;
}
function fill(ul, rows, emptyText) {
  ul.replaceChildren();
  if (!rows.length) { ul.appendChild(el('li', emptyText, 'empty')); return; }
  rows.forEach(r => ul.appendChild(r));
}

async function load() {
  try {
    const sum = await api('/api/summary');
    if (!token) { setHero(sum.overall, sum.summary); return signOut(); }
    const s = await api('/api/status'), h = await api('/api/history');
    mode = s.mode || 'monitor';
    $('backupsSub').textContent = mode === 'scheduler' ? 'Select a backup to adjust its schedule' : 'Select a backup to set its overdue alert';
    $('login').hidden = true; $('app').hidden = false; $('topactions').hidden = false;
    const finished = s.profiles.map(p => p.last && p.last.end).filter(valid).sort().pop();
    const parts = [];
    if (s.overall === 'ok' || s.overall === 'running') parts.push(finished ? 'Last backup ' + ago(finished) : 'No backups yet');
    else parts.push(s.summary);
    if (s.server) parts.push(s.server.up ? (s.server.identified && s.server.build ? 'Server build ' + s.server.build : 'Server reachable') : 'Can\'t reach server');
    setHero(s.overall, s.summary, parts.join(' · '));
    const nl = $('notices'); nl.replaceChildren();
    (s.findings || []).forEach(f => nl.appendChild(el('li', f.msg, f.level)));
    (s.skippedProfiles || []).forEach(k => nl.appendChild(el('li', 'Skipped ' + k.path + ': ' + k.reason, 'WARN')));
    fill($('profiles'), s.profiles.map(profileItem), 'No Syncrify profiles found');
    fill($('history'), h.slice(0, 5).map(historyItem), 'No activity yet');
    $('runall').disabled = !s.profiles.length;
    $('meta').textContent = s.os + ' · up ' + s.uptime;
    if (typeof window.syncampReport === 'function' && !window.__reported) {
      window.__reported = true;
      window.syncampReport(JSON.stringify({ headline: $('headline').textContent, profiles: s.profiles.length, signedIn: !$('app').hidden, mode: s.mode }));
    }
  } catch (e) {
    if (e.message === 'auth') signOut('Token rejected');
    else setHero('unknown', '', 'Cannot reach the SyncAmp service');
  }
}
$('signin').addEventListener('click', () => {
  token = $('token').value.trim(); $('token').value = '';
  try { sessionStorage.setItem('syncamp-token', token); } catch (e) {}
  load();
});
$('token').addEventListener('keydown', e => { if (e.key === 'Enter') $('signin').click(); });
$('signout').addEventListener('click', () => { signOut(); load(); });
$('runall').addEventListener('click', async () => { await api('/api/run', 'POST'); load(); });
$('reload').addEventListener('click', async () => { await api('/api/reload', 'POST'); load(); });
if (native) {
  $('signout').hidden = true; $('token').hidden = true; $('signin').hidden = true;
  document.querySelector('#login h2').textContent = 'Not connected';
  document.querySelector('#login .sub').textContent = '';
  // Behave like a popover: dismiss on focus loss or Escape.
  if (typeof window.syncampHide === 'function') {
    window.addEventListener('blur', () => window.syncampHide());
    window.addEventListener('keydown', e => { if (e.key === 'Escape') window.syncampHide(); });
  }
}
load(); setInterval(load, 5000);
