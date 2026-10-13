#!/usr/bin/env node
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join, resolve } from "node:path";

const require = createRequire(new URL("../e2e/package.json", import.meta.url));
const ts = require("typescript");
const { chromium } = require("@playwright/test");
const option = (name) => process.argv[process.argv.indexOf(name) + 1];
for (const name of ["--app-source", "--scene", "--out"]) assert.ok(process.argv.includes(name) && option(name), `${name} is required`);
const sourcePath = resolve(option("--app-source"));
const source = await readFile(sourcePath, "utf8");
const ast = ts.createSourceFile(sourcePath, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const normalizer = ast.statements.find((node) => ts.isFunctionDeclaration(node) && node.name?.text === "convertRawElements");
assert.ok(normalizer, "App source has no convertRawElements function");
// Run the actual shared App function, not a copy of its conversion algorithm.
const code = ts.transpileModule(normalizer.getText(ast), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
const scene = JSON.parse(await readFile(resolve(option("--scene")), "utf8"));
const elements = Array.isArray(scene) ? scene : scene.elements;
assert.ok(Array.isArray(elements) && elements.some((element) => element.type === "text" && element.version > 0), "Scene must contain complete saved Excalidraw text elements");
const out = resolve(option("--out"));
await mkdir(out, { recursive: true });
const module = `
import { convertToExcalidrawElements, FONT_FAMILY, exportToSvg } from "https://esm.sh/@excalidraw/excalidraw@0.18.0?deps=react@19.0.0,react-dom@19.0.0";
${code}
const saved = ${JSON.stringify(elements)};
const checks = [];
function check(name, passed) { checks.push({name, passed: Boolean(passed)}); }
function sameElements(left, right) { return left.length === right.length && left.every((element, index) =>
  Object.keys(element).every(key => JSON.stringify(element[key]) === JSON.stringify(right[index]?.[key]))); }
await document.fonts.load("20px Excalifont");
const restored = convertRawElements(saved);
check("complete checkpoint geometry, order and metadata preserved", sameElements(saved, restored));
const text = saved.find(element => element.type === "text");
const box = saved.find(element => element.id === text.containerId);
const minimal = [box, text];
check("one bound text restores unchanged", sameElements(minimal, convertRawElements(minimal)));
check("normalization is idempotent across repeated restores", sameElements(restored, convertRawElements(restored)));
const alternativeFont = saved.map(element => element.type === "text" ? {...element, fontFamily: 2} : element);
check("saved font choice preserved", sameElements(alternativeFont, convertRawElements(alternativeFont)));
const raw = {type: "rectangle", id: "new-box", x: 710, y: 70, width: 180, height: 80,
  label: {text: "Nouveau", fontSize: 20}, seed: 42};
const convertedRaw = convertRawElements([raw]);
const bound = convertedRaw.find(element => element.type === "text" && element.containerId === raw.id);
check("shorthand still creates a centered bound label", bound && Math.abs(bound.x + bound.width / 2 - raw.x - raw.width / 2) < .01 &&
  Math.abs(bound.y + bound.height / 2 - raw.y - raw.height / 2) < .01 && convertedRaw[0].boundElements.some(element => element.id === bound.id));
const mixed = convertRawElements([raw, ...saved]);
check("mixed shorthand and saved elements preserve saved geometry", sameElements(saved, mixed.filter(element => saved.some(original => original.id === element.id))));
check("mixed input preserves drawing order", JSON.stringify(mixed.filter(element => element.id === raw.id || saved.some(original => original.id === element.id)).map(element => element.id)) ===
  JSON.stringify([raw.id, ...saved.map(element => element.id)]));
const camera = {type: "cameraUpdate", x: 0, y: 0, width: 800, height: 600};
const withCamera = convertRawElements([...saved, camera]);
check("camera pseudo-element preserved", JSON.stringify(withCamera.at(-1)) === JSON.stringify(camera));
const moved = restored.map(element => element.id === text.id ? {...element, x: element.x + 24} : element);
check("deliberate displacement rejected by preservation check", !sameElements(saved, moved));
for (const [name, data] of [["before", saved], ["after", restored]]) {
  const svg = await exportToSvg({elements: data, appState: {exportBackground: false}, files: null, exportPadding: 20, skipInliningFonts: true});
  svg.id = name; document.body.append(svg);
}
window.__qa = {checks, deltas: saved.filter(element => element.type === "text").map(element => {
  const after = restored.find(other => other.id === element.id);
  return {text: element.text, width: element.width, height: element.height,
    beforeFont: element.fontFamily, afterFont: after.fontFamily, dx: after.x-element.x, dy: after.y-element.y};
})};
`;
const server = createServer((request, response) => {
  const isModule = request.url === "/check.js";
  response.writeHead(200, { "content-type": isModule ? "text/javascript" : "text/html" });
  response.end(isModule ? module : '<!doctype html><style>@font-face{font-family:Excalifont;src:url(https://esm.sh/@excalidraw/excalidraw@0.18.0/dist/prod/fonts/Excalifont/Excalifont-Regular-a88b72a24fb54c9f94e3b5fdaa7481c9.woff2)}svg{width:800px;height:600px;display:block}</style><script type="module" src="/check.js"></script>');
});
await new Promise((resolvePromise) => server.listen(0, "127.0.0.1", resolvePromise));
let browser;
try {
  browser = await chromium.launch({ headless: true, channel: process.env.PLAYWRIGHT_CHANNEL });
  const page = await browser.newPage({ viewport: { width: 1000, height: 1400 } });
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  await page.waitForFunction(() => window.__qa, {}, { timeout: 30000 });
  const result = await page.evaluate(() => window.__qa);
  for (const name of ["before", "after"]) await page.locator(`#${name}`).screenshot({ path: join(out, `${name}.png`) });
  const report = {
    validation: "isolated_excalidraw_app_normalizer", appRepository: "https://github.com/excalidraw/excalidraw-mcp",
    appCommit: execFileSync("git", ["rev-parse", "HEAD"], { cwd: dirname(sourcePath), encoding: "utf8" }).trim(),
    appSourceSha256: createHash("sha256").update(source).digest("hex"),
    normalizerSha256: createHash("sha256").update(code).digest("hex"),
    dependency: "@excalidraw/excalidraw@0.18.0", browserVersion: browser.version(),
    ...result, passed: result.checks.filter((check) => check.passed).length,
    failed: result.checks.filter((check) => !check.passed).length,
  };
  await writeFile(join(out, "report.json"), JSON.stringify(report, null, 2) + "\n");
  console.log(`${report.passed}/${report.checks.length} App normalizer checks passed`);
  for (const check of report.checks) if (!check.passed) console.log("FAIL " + check.name);
  console.log(JSON.stringify(report.deltas));
  if (report.failed) process.exitCode = 1;
} finally {
  await browser?.close();
  await new Promise((resolvePromise) => server.close(resolvePromise));
}
