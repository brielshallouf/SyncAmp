'use strict';
// All server-provided strings are inserted with textContent / createElement,
// never as markup: profile names come from user-writable files.
const $ = id => document.getElementById(id);
let token = null;
const open = new Set();   // expanded profile rows survive the 5 s refresh
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
async function api(path, method, body) {
  const headers = token ? { Authorization: 'Bearer ' + token } : {};
  if (body) headers['Content-Type'] = 'application/json';
  const r = await fetch(path, { method: method || 'GET', headers, body: body ? JSON.stringify(body) : undefined });
  if (r.status === 401) throw new Error('auth');
  return r.json();
}
function signOut(msg) {
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

const PRESETS = [['hourly', 'Hourly'], ['every6h', 'Every 6 h'], ['daily', 'Daily']];
async function schedule(name, body) { try { await api('/api/profiles/' + encodeURIComponent(name) + '/schedule', 'POST', body); } catch (e) {} load(); }

function drawer(p) {
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
  const state = p.running ? 'running' : p.paused ? 'paused' : p.last ? p.last.status : 'unknown';
  row.appendChild(el('span', '', 'dot ' + state));
  const g = el('div', '', 'grow'); g.appendChild(el('b', p.name));
  let line;
  if (p.running) line = 'Backing up now…';
  else if (p.paused) line = 'Paused';
  else if (!p.last) line = p.scheduled ? 'Waiting for first run' : 'Manual · never run';
  else line = outcome(p.last.status) + ' ' + ago(p.last.end);
  g.appendChild(el('span', line));
  if (p.last && p.last.status !== 'success' && p.last.error && !p.running) g.appendChild(el('span', p.last.error, 'err'));
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
  if (r.error) g.appendChild(el('span', r.error, 'err'));
  row.appendChild(g); li.appendChild(row); return li;
}
function fill(ul, rows, emptyText) {
  ul.replaceChildren();
  if (!rows.length) { ul.appendChild(el('li', emptyText, 'empty')); return; }
  rows.forEach(r => ul.appendChild(r));
}

async function load() {
  try {
    const sum = await (await fetch('/api/summary')).json();
    if (!token) { setHero(sum.overall, sum.summary); return signOut(); }
    const s = await api('/api/status'), h = await api('/api/history');
    $('login').hidden = true; $('app').hidden = false; $('topactions').hidden = false;
    const finished = s.profiles.map(p => p.last && p.last.end).filter(valid).sort().pop();
    const parts = [];
    if (s.overall === 'ok' || s.overall === 'running') parts.push(finished ? 'Last backup ' + ago(finished) : 'No backups yet');
    else parts.push(s.summary);
    if (s.server) parts.push(s.server.up ? 'Server reachable' : 'Can\'t reach server');
    setHero(s.overall, s.summary, parts.join(' · '));
    const nl = $('notices'); nl.replaceChildren();
    (s.findings || []).forEach(f => nl.appendChild(el('li', f.msg, f.level)));
    (s.skippedProfiles || []).forEach(k => nl.appendChild(el('li', 'Skipped ' + k.path + ': ' + k.reason, 'WARN')));
    fill($('profiles'), s.profiles.map(profileItem), 'No Syncrify profiles found');
    fill($('history'), h.slice(0, 5).map(historyItem), 'No activity yet');
    $('runall').disabled = !s.profiles.length;
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
