// Run with a browser Tab provided by cua_repl; checks the actual rendered App.
export async function assertPdfRendered(tab) {
  const snapshot = await tab.playwright.domSnapshot();
  if (snapshot.includes('Setting up fake worker failed')) {
    throw new Error('FAIL: PDF shows warning; bundled worker failed to load');
  }
  if (!snapshot.includes('of 15') || !snapshot.includes('Attention Is All You Need')) {
    throw new Error('FAIL: expected PDF first page absent');
  }
  return 'PASS: PDF first page rendered, 15 pages detected';
}
