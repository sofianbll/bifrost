#!/usr/bin/env node
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import { createServer } from "node:net";
import { readFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const hostDirectory = resolve(process.env.MCP_APP_HOST_DIRECTORY ?? "");
const gatewayUrl = process.env.BIFROST_FIXTURE_SERVER_URL;
const bearer = process.env.BIFROST_FIXTURE_BEARER;
const expectedInstance = process.env.BIFROST_FIXTURE_EXPECTED_INSTANCE;
const fixtureClientName = process.env.BIFROST_FIXTURE_CLIENT_NAME;
const candidateSource = process.env.BIFROST_CANDIDATE_SOURCE;
const partialReplay = process.env.BIFROST_FIXTURE_PARTIAL_REPLAY === "true";
if (process.env.BIFROST_FIXTURE_PARTIAL_REPLAY !== undefined && !["true", "false"].includes(process.env.BIFROST_FIXTURE_PARTIAL_REPLAY)) {
  throw new Error("BIFROST_FIXTURE_PARTIAL_REPLAY must be true or false when provided");
}
if (!gatewayUrl || !bearer || !expectedInstance || !fixtureClientName || !candidateSource) {
  throw new Error("BIFROST_FIXTURE_SERVER_URL, BIFROST_FIXTURE_BEARER, BIFROST_FIXTURE_EXPECTED_INSTANCE, BIFROST_FIXTURE_CLIENT_NAME, and BIFROST_CANDIDATE_SOURCE are required");
}
if (!/^sk-bf-mcp-apps-(full|limited|limited-no-app|denied)$/.test(bearer)) throw new Error("Only a synthetic MCP Apps fixture key is accepted");
if (!/^[0-9a-f]{40}$/.test(candidateSource)) throw new Error("BIFROST_CANDIDATE_SOURCE must be an immutable commit SHA");
if (!/^[A-Za-z0-9_-]+$/.test(fixtureClientName)) throw new Error("BIFROST_FIXTURE_CLIENT_NAME is invalid");

function assertNotificationOrder(actual, partial) {
  const expected = partial ? ["partial", "partial", "input", "result"] : ["input", "result"];
  assert.deepEqual(actual, expected, "App SDK handlers must receive the expected host notification order");
}

const validOrders = [["input", "result"], ["partial", "partial", "input", "result"]];
assertNotificationOrder(validOrders[0], false);
assertNotificationOrder(validOrders[1], true);
for (const invalid of [
  ["partial", "input", "result"],
  ["input", "partial", "partial", "result"],
  ["partial", "partial", "result", "input"],
]) {
  assert.throws(() => assertNotificationOrder(invalid, true), /expected host notification order/,
    `Order checker must reject missing or reordered notifications: ${JSON.stringify(invalid)}`);
}

const manifest = JSON.parse(await readFile(join(hostDirectory, ".bifrost-reference-host.json"), "utf8"));
if (manifest.commit !== "82221c0c8ce7661efa6771c9d461511b1650495f" || manifest.version !== "2.0.3") {
  throw new Error("Reference host does not match the approved ext-apps source pin");
}
if (!manifest.hostFixtureAuthAdapter?.patchSha256 || !manifest.controlledPartialReplay?.patchSha256 || !manifest.appBundle?.sha256) {
  throw new Error("Reference host manifest is missing adapter, partial replay, or generated SDK artifact identity");
}

const require = createRequire(join(hostDirectory, "package.json"));
const { chromium } = require("playwright");

async function portAvailable(port) {
  const server = createServer();
  return new Promise((resolvePromise, reject) => {
    server.once("error", (error) => error.code === "EADDRINUSE" ? resolvePromise(false) : reject(error));
    server.listen(port, "127.0.0.1", () => server.close((error) => error ? reject(error) : resolvePromise(true)));
  });
}

for (const port of [8080, 8081]) {
  if (!(await portAvailable(port))) throw new Error(`Pinned reference host requires localhost:${port}, which is already occupied`);
}

const serverUrl = new URL(gatewayUrl);
if (!/^https?:$/.test(serverUrl.protocol) || serverUrl.username || serverUrl.password) {
  throw new Error("Fixture gateway URL must be HTTP(S) and contain no embedded credentials");
}
const host = spawn("./node_modules/.bin/tsx", ["examples/basic-host/serve.ts"], {
  cwd: hostDirectory,
  env: {
    ...process.env,
    HOST_PORT: "8080",
    SANDBOX_PORT: "8081",
    SERVERS: JSON.stringify([serverUrl.href]),
    BIFROST_FIXTURE_SERVER_URL: serverUrl.href,
    BIFROST_FIXTURE_BEARER: bearer,
    BIFROST_FIXTURE_PARTIAL_REPLAY: String(partialReplay),
  },
  stdio: ["ignore", "pipe", "pipe"],
});
let hostOutput = "";
host.stdout.on("data", (chunk) => { hostOutput += chunk; });
host.stderr.on("data", (chunk) => { hostOutput += chunk; });
const redact = (value) => String(value).split(bearer).join("[REDACTED]")
  .replace(/(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;]+/gi, "$1[REDACTED]");

let browser;
try {
  let ready = false;
  for (let attempt = 0; attempt < 80; attempt++) {
    if (host.exitCode !== null) throw new Error(`Pinned host exited before ready: ${redact(hostOutput)}`);
    try {
      const response = await fetch("http://localhost:8080/api/servers");
      if (response.ok) { ready = true; break; }
    } catch {}
    await delay(250);
  }
  if (!ready) throw new Error(`Pinned host did not become ready: ${redact(hostOutput)}`);

  browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } : {}) });
  const page = await browser.newPage();
  const pageErrors = [];
  page.on("pageerror", (error) => pageErrors.push(redact(error.message)));
  page.on("console", (message) => {
    if (message.type() === "error") pageErrors.push(redact(message.text()));
  });
  if (partialReplay) {
    await page.exposeFunction("__bifrostWaitForPartialInput", async (message) => {
      const appFrame = page.frameLocator("iframe").frameLocator("iframe");
      await appFrame.getByTestId("tool-partial-input").getByText(`\"message\": \"${message}\"`, { exact: false }).waitFor();
    });
    await page.addInitScript((messages) => {
      window.__bifrostReplayPartialInput = async (bridge, input) => {
        for (const message of messages) {
          await bridge.sendToolInputPartial({ arguments: { ...input, message } });
          await window.__bifrostWaitForPartialInput(message);
        }
      };
    }, ["partial-one", "partial-two"]);
  }
  const hostUrl = new URL("http://localhost:8080/");
  hostUrl.searchParams.set("tool", `${fixtureClientName}-open_view`);
  await page.goto(hostUrl.href, { waitUntil: "domcontentloaded" });
  await page.getByLabel("Input").fill('{"message":"host-input"}');
  await page.getByRole("button", { name: "Call Tool" }).click();

  const app = page.frameLocator("iframe").frameLocator("iframe");
  const inputView = app.getByTestId("tool-input");
  const partialInputView = app.getByTestId("tool-partial-input");
  const resultView = app.getByTestId("tool-result");
  await inputView.waitFor();
  if (partialReplay) {
    await partialInputView.getByText('"message": "partial-one"', { exact: false }).waitFor();
  }
  let input;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { input = JSON.parse(await inputView.textContent()); } catch {}
    if (input?.message === "host-input") break;
    await delay(100);
  }
  assert.deepEqual(input, { message: "host-input" }, "Pinned host must deliver its full final input to the App SDK");
  let result;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { result = JSON.parse(await resultView.textContent()); } catch {}
    if (result?.structuredContent?.instance === expectedInstance) break;
    await delay(100);
  }
  assert.equal(result?.structuredContent?.instance, expectedInstance, "App SDK must receive the native result");
  const native = result.structuredContent;
  assert.equal(native?.instance, expectedInstance, "App receives the expected source's native structured result");
  assert.equal(native?.message, "host-input");
  assert.notEqual(result?.isError, true, "A successful native result may omit isError or set it false");
  assert.equal(result?.meta?.fixture, expectedInstance);
  assert.equal(result?.content?.[0]?.text, JSON.stringify({ instance: expectedInstance, message: "host-input" }));
  const appRoot = app.getByTestId("mcp-app-fixture");
  const notificationOrder = JSON.parse(await appRoot.getAttribute("data-notification-order"));
  assertNotificationOrder(notificationOrder, partialReplay);
  if (partialReplay) {
    assert.deepEqual(JSON.parse(await app.getByTestId("tool-partial-input").textContent()), { message: "partial-two" },
      "The second controlled partial update must render before final input/result");
  }

  const callbackResult = app.getByTestId("callback-result");
  const readState = async (expected) => {
    await app.getByTestId("read-state").click();
    await callbackResult.getByText(expected, { exact: false }).waitFor();
    const callback = JSON.parse(await callbackResult.textContent());
    assert.equal(callback.name, `${fixtureClientName}-read_state`, "App must invoke the source-qualified read callback");
    assert.equal(callback.structuredContent?.instance, expectedInstance);
    assert.equal(callback.structuredContent?.state, expected);
    assert.equal(callback.content?.[0]?.text, `${expectedInstance}:read_state`);
    return callback;
  };
  await app.getByTestId("save-state").click();
  await callbackResult.getByText("first-state", { exact: false }).waitFor();
  await readState("first-state");
  await app.getByTestId("change-state").click();
  await callbackResult.getByText("changed-state", { exact: false }).waitFor();
  await readState("changed-state");
  await app.getByTestId("restore-state").click();
  await callbackResult.getByText("first-state", { exact: false }).waitFor();
  await readState("first-state");

  // Negative control: corrupt a known-good save and prove the same state
  // assertion rejects the returned value. This is not evidence of gateway
  // behavior beyond the successful callback path above.
  await app.getByTestId("corrupt-state").click();
  await callbackResult.getByText("corrupt-state", { exact: false }).waitFor();
  await app.getByTestId("read-state").click();
  await callbackResult.getByText("corrupt-state", { exact: false }).waitFor();
  const corrupted = JSON.parse(await callbackResult.textContent());
  assert.throws(() => assert.equal(corrupted.structuredContent?.state, "first-state"), /first-state/);
  assert.equal(corrupted.structuredContent?.state, "corrupt-state");
  assert.deepEqual(pageErrors, [], `Reference-host page errors: ${pageErrors.join("; ")}`);

  console.log(JSON.stringify({
    evidenceLabel: "pinned reference host with fixture-auth adapter",
    partialEvidenceLabel: partialReplay ? "controlled partial-input replay through pinned AppBridge; not live-model/native Codex streaming" : undefined,
    hostCommit: manifest.commit,
    hostFixtureAuthPatchSha256: manifest.hostFixtureAuthAdapter.patchSha256,
    appBundleSha256: manifest.appBundle.sha256,
    candidateSource,
    gatewayRoute: `${serverUrl.origin}${serverUrl.pathname}`,
    expectedInstance,
    fixtureClientName,
    checks: {
      sdkHandshakeAndViewRendered: true,
      fullInputDelivered: true,
      controlledPartialInputDelivered: partialReplay,
      notificationOrderChecked: true,
      missingAndReorderedNotificationsRejected: true,
      nativeResultSourceAndPayload: true,
      saveReadChangeRestore: true,
      deliberateCorruptStateDetected: true,
      pageErrors: pageErrors.length,
    },
  }, null, 2));
} catch (error) {
  throw new Error(redact(error?.stack ?? error));
} finally {
  if (browser) await browser.close();
  host.kill("SIGTERM");
  await Promise.race([new Promise((resolvePromise) => host.once("exit", resolvePromise)), delay(5000)]);
  if (host.exitCode === null) host.kill("SIGKILL");
}
