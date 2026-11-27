'use strict';
// All server-provided strings are inserted with textContent / createElement,
// never as markup: profile names come from user-writable files.
const $ = id => document.getElementById(id);
let token = null;
try { token = sessionStorage.getItem('syncamp-token'); } catch (e) {}

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
async function api(path, method) {
  const r = await fetch(path, { method: method || 'GET', headers: token ? { Authorization: 'Bearer ' + token } : {} });
  if (r.status === 401) throw new Error('auth');
  return r.json();
}
function signOut(msg) {
  token = null; try { sessionStorage.removeItem('syncamp-token'); } catch (x) {}
  $('login').hidden = false; $('app').hidden = true; $('loginerr').textContent = msg || '';
}
const GLYPH = { ok: '✓', running: '↻', warn: '!', error: '✕', unknown: '·' };
const HEAD = { ok: 'All backups healthy', running: 'Backing up…', warn: 'Needs a look', error: 'Needs attention' };

function setHero(overall, summary, sub) {
  const ring = $('ring'); ring.className = 'ring ' + overall; $('glyph').textContent = GLYPH[overall] || '·';
  $('headline').textContent = HEAD[overall] || 'SyncAmp';
  $('subline').textContent = sub || summary || '';
}
function profileRow(p) {
  const li = el('li');
  const state = p.running ? 'running' : p.paused ? 'paused' : p.last ? p.last.status : 'unknown';
  li.appendChild(el('span', '', 'dot ' + state));
  const g = el('div', '', 'grow'); g.appendChild(el('b', p.name));
  let line;
  if (p.running) line = 'Backing up now…';
  else if (p.paused) line = 'Paused';
  else if (!p.last) line = p.scheduled ? 'Waiting for first run' : 'Manual · never run';
  else line = (p.last.status === 'success' ? 'Succeeded ' : p.last.status === 'timeout' ? 'Timed out ' : 'Failed ') + ago(p.last.end);
  g.appendChild(el('span', line));
  if (p.last && p.last.status !== 'success' && p.last.error && !p.running) g.appendChild(el('span', p.last.error, 'err'));
  li.appendChild(g);
  const b = el('button', '▶', 'btn play'); b.disabled = p.running; b.title = 'Back up now'; b.setAttribute('aria-label', 'Back up ' + p.name + ' now');
  b.addEventListener('click', async () => { b.disabled = true; try { await api('/api/run/' + encodeURIComponent(p.name), 'POST'); } catch (e) {} load(); });
  li.appendChild(b);
  return li;
}
function historyRow(r) {
  const li = el('li');
  li.appendChild(el('span', '', 'dot ' + r.status));
  const g = el('div', '', 'grow'); g.appendChild(el('b', r.profile));
  g.appendChild(el('span', (r.status === 'success' ? 'Succeeded' : r.status === 'timeout' ? 'Timed out' : 'Failed') + ' · ' + ago(r.end) + ' · ' + r.trigger));
  if (r.error) g.appendChild(el('span', r.error, 'err'));
  li.appendChild(g);
  return li;
}
function fill(ul, rows, emptyText) {
  ul.replaceChildren();
  if (!rows.length) { const li = el('li', emptyText, 'empty'); ul.appendChild(li); return; }
  rows.forEach(r => ul.appendChild(r));
}

async function load() {
  try {
    const sum = await (await fetch('/api/summary')).json();
    if (!token) { setHero(sum.overall, sum.summary); return signOut(); }
    const s = await api('/api/status'), h = await api('/api/history');
    $('login').hidden = true; $('app').hidden = false;
    const finished = s.profiles.map(p => p.last && p.last.end).filter(valid).sort().pop();
    const parts = [];
    if (s.overall === 'ok' || s.overall === 'running') parts.push(finished ? 'Last backup ' + ago(finished) : 'No backups yet');
    else parts.push(s.summary);
    if (s.server) parts.push(s.server.up ? 'Server online' : 'Server unreachable');
    setHero(s.overall, s.summary, parts.join(' · '));
    const nl = $('notices'); nl.replaceChildren();
    (s.findings || []).forEach(f => nl.appendChild(el('li', f.msg, f.level)));
    (s.skippedProfiles || []).forEach(k => nl.appendChild(el('li', 'Skipped ' + k.path + ': ' + k.reason, 'WARN')));
    fill($('profiles'), s.profiles.map(profileRow), 'No Syncrify profiles found');
    fill($('history'), h.slice(0, 10).map(historyRow), 'No activity yet');
    $('meta').textContent = s.os + ' · up ' + s.uptime;
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
load(); setInterval(load, 5000);
