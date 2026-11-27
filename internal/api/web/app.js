'use strict';
// All server-provided strings are inserted with textContent / createElement,
// never innerHTML: profile names come from user-writable files.
const $ = id => document.getElementById(id);
let token = null;
try { token = sessionStorage.getItem('syncamp-token'); } catch (e) {}

function el(tag, text, cls) {
  const n = document.createElement(tag);
  if (text !== undefined) n.textContent = text;
  if (cls) n.className = cls;
  return n;
}
const fmt = t => (t && !t.startsWith('0001')) ? new Date(t).toLocaleString() : '—';

async function api(path, method) {
  const r = await fetch(path, { method: method || 'GET', headers: token ? { Authorization: 'Bearer ' + token } : {} });
  if (r.status === 401) throw new Error('auth');
  return r.json();
}
function row(table, cells, header) {
  const tr = el('tr');
  cells.forEach(c => tr.appendChild(c instanceof Node ? c : el(header ? 'th' : 'td', c)));
  table.appendChild(tr);
}
function pill(status) { return el('span', status, 'pill ' + status); }
function showLogin(msg) {
  $('login').hidden = false; $('app').hidden = true; $('loginerr').textContent = msg || '';
}

async function load() {
  try {
    const sum = await (await fetch('/api/summary')).json();
    $('overall').className = 'pill ' + sum.overall; $('overall').textContent = sum.overall;
    if (!token) return showLogin();
    const s = await api('/api/status'), h = await api('/api/history');
    $('login').hidden = true; $('app').hidden = false;
    $('summary').textContent = s.summary;
    $('meta').textContent = s.os + ' · client: ' + (s.client || 'not found') + ' · up ' + s.uptime +
      (s.server ? ' · server ' + (s.server.up ? 'up' : 'DOWN') + ' (' + s.server.url + ')' : '');
    const fl = $('findings'); fl.replaceChildren();
    (s.findings || []).forEach(f => fl.appendChild(el('li', f.level + ': ' + f.msg, f.level)));
    (s.skippedProfiles || []).forEach(k => fl.appendChild(el('li', 'skipped ' + k.path + ': ' + k.reason, 'WARN')));

    const pt = $('profiles'); pt.replaceChildren();
    row(pt, ['Profile', 'Last run', 'Result', ''], true);
    s.profiles.forEach(p => {
      const btn = el('button', 'Run'); btn.disabled = p.running;
      btn.addEventListener('click', async () => { await api('/api/run/' + encodeURIComponent(p.name), 'POST'); load(); });
      row(pt, [p.name + (p.scheduled ? '' : ' (manual)'), p.last ? fmt(p.last.end) : 'never',
        p.running ? pill('running') : p.last ? pill(p.last.status) : '', btn]);
    });
    const ht = $('history'); ht.replaceChildren();
    row(ht, ['Profile', 'Started', 'Trigger', 'Result', 'Detail'], true);
    h.forEach(r => row(ht, [r.profile, fmt(r.start), r.trigger, pill(r.status), r.error || '']));
  } catch (e) {
    if (e.message === 'auth') { token = null; try { sessionStorage.removeItem('syncamp-token'); } catch (x) {} showLogin('Token rejected'); }
    else $('summary').textContent = 'Cannot reach SyncAmp service';
  }
}
$('signin').addEventListener('click', () => {
  token = $('token').value.trim(); $('token').value = '';
  try { sessionStorage.setItem('syncamp-token', token); } catch (e) {}
  load();
});
$('signout').addEventListener('click', () => { token = null; try { sessionStorage.removeItem('syncamp-token'); } catch (e) {} load(); });
$('runall').addEventListener('click', async () => { await api('/api/run', 'POST'); load(); });
$('reload').addEventListener('click', async () => { await api('/api/reload', 'POST'); load(); });
load(); setInterval(load, 5000);
