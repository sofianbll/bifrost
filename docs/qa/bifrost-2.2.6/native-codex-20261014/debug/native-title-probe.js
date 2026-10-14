// Run only in cua_repl, against the already-open native QA view.
// First initialization: let nativeAppsBrowser = await cua.getBrowser({id:"mcpapps"});
// Bind the observed tab: let nativeProbeTab = await nativeAppsBrowser.tabs.get("1");
// Reproduction command: await probeNativeTitle();
async function probeNativeTitle(timeoutMs = 2000) {
  const started = Date.now();
  try {
    const title = await nativeProbeTab.playwright.evaluate(() => document.title, undefined, {timeoutMs});
    if (typeof title !== "string") throw new Error("Native document title unavailable");
    const verdict = {status:"green", elapsedMs:Date.now()-started, title};
    nodeRepl.write(verdict);
    return verdict;
  } catch (error) {
    const verdict = {status:"red", elapsedMs:Date.now()-started, error:String(error)};
    nodeRepl.write(verdict);
    return verdict;
  }
}
