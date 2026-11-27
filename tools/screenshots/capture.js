// Usage: node capture.js <port> <token> <state: attention|healthy|signin> <outfile>
const { chromium } = require('playwright');
const [port, token, state, out] = process.argv.slice(2);
(async () => {
  const b = await chromium.launch();
  // 400 px wide = the real popover window size (full-bleed below 440 px).
  const ctx = await b.newContext({ viewport: { width: 400, height: 700 }, deviceScaleFactor: 2,
    colorScheme: state === 'healthy' ? 'dark' : 'light' });
  const p = await ctx.newPage();
  await p.goto(`http://127.0.0.1:${port}/`);
  if (state !== 'signin') {
    await p.fill('#token', token); await p.click('#signin'); await p.waitForTimeout(1500);
    if (state === 'attention') { await p.click('.row:has(b:text-is("Photos"))'); await p.waitForTimeout(400); }
  } else { await p.waitForTimeout(800); }
  await p.mouse.move(0, 0); await p.waitForTimeout(150); // no stray hover highlight
  await p.screenshot({ path: out, fullPage: state !== 'signin', clip: state === 'signin' ? { x: 0, y: 0, width: 400, height: 330 } : undefined });
  await b.close();
})();
