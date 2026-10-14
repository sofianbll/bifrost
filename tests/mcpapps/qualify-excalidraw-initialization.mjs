#!/usr/bin/env node
// Replay eager notifications through the actual built App and SDK; separate from native Codex QA.
import { createRequire } from "node:module";
import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";

const require = createRequire(new URL("../e2e/package.json", import.meta.url));
const { chromium } = require("@playwright/test");
const option = (name) => process.argv[process.argv.indexOf(name) + 1];
for (const flag of ["--html", "--scene", "--out", "--timing"]) {
  if (!process.argv.includes(flag) || !option(flag)) throw new Error(`${flag} is required`);
}
const html = await readFile(option("--html"), "utf8");
const scene = JSON.parse(await readFile(option("--scene"), "utf8"));
const out = resolve(option("--out"));
const timing = option("--timing");
if (!["immediate", "delayed"].includes(timing)) throw new Error("timing must be immediate or delayed");
if (!Array.isArray(scene.elements) || scene.elements.filter(e => e.type === "text" && e.containerId).length < 3) {
  throw new Error("Expected a full checkpoint with three bound labels");
}
await mkdir(out, { recursive: true });
const input = { elements: JSON.stringify([
  { type: "restoreCheckpoint", id: "native-fixture" },
  { type: "cameraUpdate", x: 0, y: 0, width: 800, height: 600 },
]) };
const serialize = (value) => JSON.stringify(value).replace(/</g, "\\u003c");
const host = `<!doctype html>
<style>body{margin:0}iframe{border:0;width:800px;height:600px}</style>
<iframe src="/app"></iframe><script>
const frame = document.querySelector("iframe");
window.calls = []; window.events = [];
function send(message) {
  frame.contentWindow.postMessage({ jsonrpc: "2.0", ...message }, "*");
}
function notify() {
  send({ method: "ui/notifications/tool-input", params: { arguments: ${serialize(input)} } });
  send({ method: "ui/notifications/tool-result", params: {
    content: [{ type: "text", text: "Fixture displayed" }],
    structuredContent: { checkpointId: "restored-fixture" }
  } });
  window.events.push("input/result sent");
}
window.addEventListener("message", event => {
  if (event.source !== frame.contentWindow) return;
  const message = event.data;
  if (!message?.jsonrpc) return;
  window.events.push(message.method || "response");
  if (message.method === "ui/initialize") {
    send({ id: message.id, result: {
      protocolVersion: "2025-11-21",
      hostInfo: { name: "Initialization regression host", version: "1" },
      hostCapabilities: { serverTools: {} },
      hostContext: { theme: "light", displayMode: "inline",
        containerDimensions: { width: 800, maxHeight: 600 } }
    } });
    if (${serialize(timing)} === "immediate") notify();
  } else if (message.method === "ui/notifications/initialized" && ${serialize(timing)} === "delayed") {
    setTimeout(notify, 100);
  } else if (message.method === "tools/call") {
    window.calls.push(message.params);
    if (message.params.name !== "read_checkpoint" || message.params.arguments?.id !== "native-fixture") {
      send({ id: message.id, error: { code: -32602, message: "Unexpected fixture callback" } });
      return;
    }
    send({ id: message.id, result: {
      content: [{ type: "text", text: JSON.stringify(${serialize(scene)}) }]
    } });
  } else if (message.id !== undefined) {
    send({ id: message.id, result: {} });
  }
});
</script>`;
const server = createServer((request, response) => {
  response.writeHead(200, { "content-type": "text/html" });
  response.end(request.url === "/app" ? html : host);
});
await new Promise((resolvePromise, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolvePromise);
});
let browser;
try {
  browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL });
  const page = await browser.newPage({ viewport: { width: 820, height: 640 } });
  const errors = [];
  page.on("pageerror", error => errors.push(String(error)));
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  const frame = page.frameLocator("iframe");
  let passed = true;
  try {
    await frame.locator(".svg-wrapper svg").waitFor({ state: "visible", timeout: 30000 });
    await frame.locator(".svg-wrapper svg text").filter({ hasText: "Excalidraw" })
      .waitFor({ state: "visible", timeout: 30000 });
  } catch {
    passed = false;
  }
  const labels = await frame.locator(".svg-wrapper svg text").allTextContents();
  const calls = await page.evaluate(() => window.calls.map(call => call.name));
  passed = passed && ["Codex", "Bifrost", "Excalidraw"].every(text => labels.includes(text)) &&
    calls.includes("read_checkpoint") && errors.length === 0;
  const report = {
    validation: "isolated_actual_App_initialization_replay",
    appHtmlSha256: createHash("sha256").update(html).digest("hex"),
    browserVersion: browser.version(), timing, passed,
    host: await page.evaluate(() => ({ events: window.events, calls: window.calls.map(call => call.name) })),
    labels, errors,
  };
  await page.screenshot({ path: join(out, "view.png") });
  await writeFile(join(out, "report.json"), JSON.stringify(report, null, 2) + "\n");
  console.log(JSON.stringify(report, null, 2));
  if (!passed) process.exitCode = 1;
} finally {
  await browser?.close();
  await new Promise((resolvePromise) => server.close(resolvePromise));
}
