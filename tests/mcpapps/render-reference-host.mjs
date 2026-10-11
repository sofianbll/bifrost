#!/usr/bin/env node
import { createRequire } from "node:module";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer, request as httpRequest } from "node:http";
import { request as httpsRequest } from "node:https";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createServer as createTcpServer } from "node:net";
import { resolve, join } from "node:path";
import { performance } from "node:perf_hooks";

const HOST_REPOSITORY = "https://github.com/modelcontextprotocol/ext-apps";
const HOST_VERSION = "2.0.3";
const HOST_COMMIT = "82221c0c8ce7661efa6771c9d461511b1650495f";
const FORM_ACTION_TIMEOUT_MS = 10000;
const require = createRequire(new URL("../e2e/package.json", import.meta.url));

function option(name, fallback) {
  const index = process.argv.indexOf(name);
  return index < 0 ? fallback : process.argv[index + 1];
}

function fail(message) {
  throw new Error(message);
}

function redact(value, secrets) {
  let text = String(value);
  for (const secret of secrets) if (secret) text = text.split(secret).join("[REDACTED]");
  return text
    .replace(/(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;]+/gi, "$1[REDACTED]")
    .replace(/(api[_-]?key|token|secret)(\s*[:=]\s*)[^\s,;]+/gi, "$1$2[REDACTED]");
}

function parseJsonRpc(body) {
  const text = Buffer.isBuffer(body) ? body.toString("utf8") : String(body);
  const candidates = [text, ...text.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim())];
  for (const candidate of candidates) {
    try { return JSON.parse(candidate); } catch {}
  }
  return null;
}

async function getBody(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  return Buffer.concat(chunks);
}

function readHeaderProfiles(credentials, appId) {
  const routes = credentials.routes ?? {};
  const profiles = {
    direct: routes.direct?.headers ?? credentials.headers ?? {},
    virtual: routes.virtual?.headers ?? credentials.headers ?? {},
    aggregate: routes.aggregate?.headers ?? credentials.headers ?? {},
    code_mode: routes.code_mode?.headers ?? routes.direct?.headers ?? credentials.headers ?? {},
  };
  for (const [profile, headers] of Object.entries(profiles)) {
    if (!headers || typeof headers !== "object" || Array.isArray(headers) || Object.values(headers).some((v) => typeof v !== "string")) {
      fail(`Invalid credential headers for ${profile} route (${appId})`);
    }
  }
  return profiles;
}

function routePath(manifest, route, appId) {
  const value = route === "direct" || route === "code_mode"
    ? manifest.routes[route]?.[appId]
    : manifest.routes[route];
  if (typeof value !== "string" || !value.startsWith("/mcp")) fail(`Missing ${route} route for ${appId}`);
  return value;
}

async function portAvailable(port) {
  const probe = createTcpServer();
  return new Promise((resolvePromise, reject) => {
    probe.once("error", (error) => {
      if (error.code === "EADDRINUSE") resolvePromise(false);
      else reject(error);
    });
    probe.listen(port, "127.0.0.1", () => {
      probe.close((error) => error ? reject(error) : resolvePromise(true));
    });
  });
}

function successfulToolResult(result) {
  return Boolean(result && result.hasResult && !result.rpcError && !result.isError);
}

function actionTimeout(timeoutMs) {
  return Math.min(timeoutMs, FORM_ACTION_TIMEOUT_MS);
}

function nativeToolName(app, mode) {
  const name = mode === "code_mode" ? app.code_native_name : app.native_name;
  if (typeof name !== "string" || !name) fail(`Missing native tool for ${mode}`);
  return name;
}

async function selfCheck() {
  const manifest = { routes: {
    direct: { excalidraw: "/mcp/excalidraw" },
    code_mode: { excalidraw: "/mcp/excalidraw-code" },
    virtual: "/mcp/codeapps",
    aggregate: "/mcp",
  } };
  const app = { native_name: "debug-debug-tool", code_native_name: "code_debug-debug-tool" };
  assert.equal(nativeToolName(app, "normal"), "debug-debug-tool");
  assert.equal(nativeToolName(app, "code_mode"), "code_debug-debug-tool");
  assert.throws(() => nativeToolName({ native_name: app.native_name }, "code_mode"), /Missing native tool/);
  assert.equal(routePath(manifest, "direct", "excalidraw"), "/mcp/excalidraw");
  assert.equal(routePath(manifest, "code_mode", "excalidraw"), "/mcp/excalidraw-code");
  assert.equal(routePath(manifest, "virtual", "excalidraw"), "/mcp/codeapps");
  assert.equal(routePath(manifest, "aggregate", "excalidraw"), "/mcp");
  assert.deepEqual(parseJsonRpc('{"jsonrpc":"2.0","id":4,"result":{}}'), { jsonrpc: "2.0", id: 4, result: {} });
  assert.deepEqual(parseJsonRpc('event: message\ndata: {"jsonrpc":"2.0","id":5,"result":{}}\n\n'), { jsonrpc: "2.0", id: 5, result: {} });
  assert.equal(redact("Authorization: Bearer test-secret", ["test-secret"]), "Authorization: [REDACTED]");
  assert.equal(successfulToolResult({ hasResult: true, rpcError: false, isError: false }), true);
  assert.equal(successfulToolResult({ hasResult: false, rpcError: true, isError: false }), false);
  assert.equal(successfulToolResult({ hasResult: false, rpcError: false, isError: false }), false);
  assert.equal(successfulToolResult({ hasResult: true, rpcError: false, isError: true }), false);
  assert.equal(actionTimeout(120000), FORM_ACTION_TIMEOUT_MS);
  assert.equal(actionTimeout(5000), 5000);
  await checkProxyCleanup();
  assert.equal(await portAvailable(0), true);
  const occupied = createTcpServer();
  const occupiedPort = await listen(occupied);
  try { assert.equal(await portAvailable(occupiedPort), false); }
  finally { await new Promise((resolvePromise) => occupied.close(resolvePromise)); }
  console.log("PASS renderer self-check (route and native tool mapping, JSON-RPC failures, credential redaction, bounded form actions, SSE cleanup, port availability); no host was launched");
}

function classifyFailure(appId, error) {
  if (error?.failure_class) return error.failure_class;
  const message = String(error?.message ?? error);
  if (appId === "pdf" && /content.security.policy|worker-src|data:.*(?:worker|script)|worker.*blocked/i.test(message)) {
    return "app_server_or_browser_package";
  }
  if (appId === "host" || /Reference host|basic-host|four MCP connections/i.test(message)) return "reference_host";
  return "gateway_or_app_qualification";
}

function listen(server) {
  return new Promise((resolvePromise, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolvePromise(server.address().port);
    });
  });
}

async function openPorts(count) {
  const servers = Array.from({ length: count }, () => createTcpServer());
  const ports = await Promise.all(servers.map(listen));
  await Promise.all(servers.map((server) => new Promise((resolvePromise) => server.close(resolvePromise))));
  return ports;
}

function createGatewayProxy(baseUrl, path, headers, secrets) {
  const calls = [];
  const upstreamRequests = new Set();
  const target = new URL(path, baseUrl);
  const server = createServer(async (incoming, outgoing) => {
    if (incoming.method === "OPTIONS") {
      outgoing.writeHead(204, {
        "access-control-allow-origin": "*",
        "access-control-allow-methods": "GET,POST,DELETE,OPTIONS",
        "access-control-allow-headers": incoming.headers["access-control-request-headers"] ?? "content-type,mcp-session-id,mcp-protocol-version",
        "access-control-max-age": "600",
      }).end();
      return;
    }
    try {
      const body = await getBody(incoming);
      const rpc = parseJsonRpc(body);
      const rpcRequests = Array.isArray(rpc) ? rpc : rpc ? [rpc] : [];
      const tracked = rpcRequests.filter((item) => item.method === "tools/call").map((item) => ({ id: item.id, name: item.params?.name, arguments: item.params?.arguments ?? {} }));
      const event = { method: incoming.method, rpcMethod: rpcRequests.map((item) => item.method).filter(Boolean), toolCalls: tracked, results: [] };
      calls.push(event);

      const upstreamHeaders = { ...incoming.headers, ...headers, host: target.host };
      delete upstreamHeaders.origin;
      delete upstreamHeaders.referer;
      delete upstreamHeaders.connection;
      delete upstreamHeaders["transfer-encoding"];
      if (body.length) upstreamHeaders["content-length"] = String(body.length);
      const sender = target.protocol === "https:" ? httpsRequest : httpRequest;
      const destination = new URL(target);
      destination.search = new URL(incoming.url ?? "/", "http://127.0.0.1").search;
      const upstream = sender(destination, { method: incoming.method, headers: upstreamHeaders }, (response) => {
        outgoing.writeHead(response.statusCode ?? 502, {
          ...response.headers,
          "access-control-allow-origin": "*",
          "access-control-expose-headers": "mcp-session-id,mcp-protocol-version",
          "access-control-allow-headers": "content-type,mcp-session-id,mcp-protocol-version",
        });
        const chunks = [];
        let capturedBytes = 0;
        response.on("data", (chunk) => {
          const bytes = Buffer.from(chunk);
          outgoing.write(bytes);
          if (capturedBytes < 4 * 1024 * 1024) {
            const retained = bytes.subarray(0, Math.max(0, 4 * 1024 * 1024 - capturedBytes));
            chunks.push(retained);
            capturedBytes += retained.length;
          }
        });
        response.on("end", () => {
          const payload = Buffer.concat(chunks);
          for (const request of tracked) {
            const parsed = parseJsonRpc(payload);
            const candidates = Array.isArray(parsed) ? parsed : parsed ? [parsed] : [];
            const result = candidates.find((item) => item.id === request.id);
            if (result) event.results.push({
              name: request.name,
              arguments: request.arguments,
              hasResult: Object.hasOwn(result, "result"),
              rpcError: Boolean(result.error),
              isError: result.result?.isError === true,
              result: result.result,
            });
          }
          if (!outgoing.destroyed) outgoing.end();
        });
        response.on("error", (error) => { if (!outgoing.destroyed) outgoing.destroy(error); });
      });
      upstreamRequests.add(upstream);
      upstream.once("close", () => upstreamRequests.delete(upstream));
      outgoing.once("close", () => {
        if (!outgoing.writableEnded) upstream.destroy();
      });
      upstream.on("error", (error) => {
        if (outgoing.destroyed) return;
        outgoing.writeHead(502, { "access-control-allow-origin": "*", "content-type": "text/plain" });
        outgoing.end(redact(error.message, secrets));
      });
      if (body.length) upstream.write(body);
      upstream.end();
    } catch (error) {
      if (!outgoing.headersSent) outgoing.writeHead(500, { "access-control-allow-origin": "*" });
      outgoing.end(redact(error.message, secrets));
    }
  });
  return { server, calls, target, upstreamRequests };
}

function closeProxy(proxy) {
  if (!proxy.server.listening) return Promise.resolve();
  const closed = new Promise((resolvePromise, reject) => {
    proxy.server.close((error) => error && error.code !== "ERR_SERVER_NOT_RUNNING" ? reject(error) : resolvePromise());
  });
  proxy.server.closeAllConnections();
  for (const upstream of proxy.upstreamRequests) upstream.destroy();
  return closed;
}

async function within(promise, timeoutMs, message) {
  let timer;
  try {
    await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(message)), timeoutMs);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

async function checkProxyCleanup() {
  let upstreamStartCount = 0;
  let upstreamCloseCount = 0;
  const upstreamServer = createServer((_request, response) => {
    upstreamStartCount++;
    response.writeHead(200, { "content-type": "text/event-stream" });
    response.write("event: ready\ndata: {}\n\n");
    response.on("close", () => { upstreamCloseCount++; });
  });
  const upstreamPort = await listen(upstreamServer);
  const proxy = createGatewayProxy(`http://127.0.0.1:${upstreamPort}`, "/mcp", {}, []);
  let client;
  try {
    const proxyPort = await listen(proxy.server);
    const openStream = () => {
      const request = httpRequest(`http://127.0.0.1:${proxyPort}/mcp`, {
        method: "POST",
        headers: { "content-type": "application/json", accept: "text/event-stream" },
      });
      request.on("error", () => {});
      request.end(JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/list", params: {} }));
      return request;
    };
    client = openStream();
    client.once("response", (response) => response.once("data", () => client.destroy()));
    await waitFor(() => upstreamStartCount === 1, 1000, "SSE self-check did not open an upstream request");
    await waitFor(() => upstreamCloseCount === 1, 1000, "Closing the downstream SSE client did not close its upstream response");
    client = openStream();
    await waitFor(() => upstreamStartCount === 2, 1000, "Second SSE self-check did not open an upstream request");
    await within(closeProxy(proxy), 1000, "Closing the proxy did not finish within one second");
    await waitFor(() => upstreamCloseCount === 2, 1000, "Closing the proxy did not close its upstream SSE response");
  } finally {
    client?.destroy();
    await within(closeProxy(proxy), 1000, "Proxy cleanup hung during self-check teardown");
    upstreamServer.closeAllConnections();
    await within(new Promise((resolvePromise) => upstreamServer.close(resolvePromise)), 1000,
      "Upstream self-check server did not close");
  }
}

async function startHost(repositoryDir, servers) {
  const hostDir = join(repositoryDir, "examples/basic-host");
  const [hostPort] = await openPorts(1);
  const sandboxPort = 8081;
  if (!await portAvailable(sandboxPort)) fail("Required basic-host sandbox port 8081 is occupied; refusing to touch the existing process");
  const child = spawn(join(repositoryDir, "node_modules/.bin/tsx"), ["serve.ts"], {
    cwd: hostDir,
    env: { ...process.env, HOST_PORT: String(hostPort), SANDBOX_PORT: String(sandboxPort), SERVERS: JSON.stringify(servers) },
    stdio: ["ignore", "pipe", "pipe"],
    detached: true,
  });
  const logs = [];
  let launchError;
  child.once("error", (error) => { launchError = error; });
  for (const stream of [child.stdout, child.stderr]) stream.on("data", (chunk) => logs.push(chunk.toString()));
  const started = performance.now();
  while (performance.now() - started < 15000) {
    if (launchError) fail(`Could not launch the pinned reference host: ${launchError.message}`);
    if (child.exitCode !== null) fail(`Reference host exited before listening: ${logs.join("")}`);
    try {
      const response = await fetch(`http://127.0.0.1:${hostPort}/api/servers`);
      if (response.ok) return { child, hostUrl: `http://127.0.0.1:${hostPort}`, logs };
    } catch {}
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 100));
  }
  await stopChild(child);
  fail(`Reference host did not listen within 15 seconds: ${logs.join("")}`);
}

async function stopChild(child) {
  if (!child) return;
  const signalOwnedGroup = (signal) => {
    if (process.platform !== "win32") {
      try { process.kill(-child.pid, signal); }
      catch (error) { if (error.code !== "ESRCH") throw error; }
    } else if (child.exitCode === null) child.kill(signal);
  };
  signalOwnedGroup("SIGTERM");
  if (process.platform !== "win32") {
    const deadline = performance.now() + 5000;
    while (performance.now() < deadline) {
      try { process.kill(-child.pid, 0); }
      catch (error) { if (error.code === "ESRCH") return; throw error; }
      await new Promise((resolvePromise) => setTimeout(resolvePromise, 50));
    }
    signalOwnedGroup("SIGKILL");
  } else if (child.exitCode === null) {
    await Promise.race([
      new Promise((resolvePromise) => child.once("exit", resolvePromise)),
      new Promise((resolvePromise) => setTimeout(resolvePromise, 5000)),
    ]);
    if (child.exitCode === null) child.kill("SIGKILL");
  }
}

async function startProxy(baseUrl, route, headers, secrets) {
  const proxy = createGatewayProxy(baseUrl, route, headers, secrets);
  const port = await listen(proxy.server);
  proxy.url = `http://127.0.0.1:${port}/mcp`;
  return proxy;
}

async function pageLogs(browser, target, runCase, timeoutMs, secrets) {
  const formTimeout = actionTimeout(timeoutMs);
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  const logs = [];
  page.on("console", (message) => logs.push({ type: message.type(), text: redact(message.text(), secrets) }));
  page.on("pageerror", (error) => logs.push({ type: "pageerror", text: redact(error.message, secrets) }));
  page.on("requestfailed", (request) => logs.push({ type: "requestfailed", text: redact(`${request.method()} ${request.url()}: ${request.failure()?.errorText ?? "unknown"}`, secrets) }));
  try {
    await page.goto(`${target.hostUrl}/?tool=${encodeURIComponent(runCase.tool)}&theme=hide`, { waitUntil: "domcontentloaded", timeout: formTimeout });
    // Wrapped controls contribute their option/value text to Playwright label matching.
    const toolSelect = page.getByLabel(/^Tool/);
    const serverSelect = page.getByLabel(/^Server/);
    await toolSelect.waitFor({ state: "visible", timeout: formTimeout });
    await serverSelect.locator("option").first().waitFor({ state: "attached", timeout: formTimeout });
    try {
      await waitFor(async () => (await toolSelect.locator("option").evaluateAll((options) => options.map((option) => option.value)))
        .includes(runCase.tool), formTimeout, `Reference host did not expose tool ${runCase.tool}`);
    } catch (error) {
      const [servers, tools] = await Promise.all([
        serverSelect.locator("option").evaluateAll((options) => options.map((option) => option.textContent?.trim() ?? "")),
        toolSelect.locator("option").evaluateAll((options) => options.map((option) => option.value)),
      ]);
      fail(`Reference host form did not expose ${runCase.tool}; servers=${JSON.stringify(servers)}, tools=${JSON.stringify(tools)}; cause=${redact(error.message, secrets)}`);
    }
    await toolSelect.selectOption(runCase.tool, { timeout: formTimeout });
    await page.getByLabel(/^Input/).fill(JSON.stringify(runCase.input), { timeout: formTimeout });
    const callButton = page.getByRole("button", { name: "Call Tool" });
    await callButton.waitFor({ state: "visible", timeout: formTimeout });
    try {
      await waitFor(() => callButton.isEnabled(), formTimeout, `Call Tool stayed disabled for ${runCase.tool}`);
    } catch {
      const [selectedTool, input, disabled] = await Promise.all([
        toolSelect.inputValue(),
        page.getByLabel(/^Input/).inputValue(),
        callButton.isDisabled(),
      ]);
      let inputValid = true;
      try { JSON.parse(input); } catch { inputValid = false; }
      fail(`Reference host did not enable ${runCase.tool}; selectedTool=${JSON.stringify(selectedTool)}, inputValid=${inputValid}, buttonDisabled=${disabled}`);
    }
    await callButton.click({ timeout: formTimeout });
    const iframe = page.locator("iframe").first();
    await iframe.waitFor({ state: "visible", timeout: formTimeout });
    return { context, page, iframe, logs };
  } catch (error) {
    await context.close();
    throw error;
  }
}

// Selectors and loaded states come from the pinned official App sources.
async function assertAppContent(iframe, appId, result, timeoutMs) {
  const inner = iframe.contentFrame().frameLocator("iframe");
  if (appId === "excalidraw") {
    await inner.locator("svg").filter({ hasText: "MCP Apps" }).first().waitFor({ state: "visible", timeout: timeoutMs });
  } else if (appId === "debug") {
    await inner.locator("#event-log .log-entry").filter({ has: inner.locator(".log-type.ontoolresult") }).filter({ hasText: "Debug text content" }).first().waitFor({ state: "visible", timeout: timeoutMs });
  } else if (appId === "monitor") {
    const hostname = result.structuredContent?.hostname;
    if (typeof hostname !== "string" || !hostname) fail("Monitor result omitted its hostname");
    await waitFor(async () => (await inner.locator("#info-hostname").textContent()) === hostname && /^\d+(?:\.\d+)?%$/.test((await inner.locator("#memory-percent").textContent()) ?? ""), timeoutMs, "Monitor did not show returned system information and live memory metrics");
  } else if (appId === "pdf") {
    await inner.locator("#viewer").waitFor({ state: "visible", timeout: timeoutMs });
    await inner.locator("#loading").waitFor({ state: "hidden", timeout: timeoutMs });
    await inner.locator("#error").waitFor({ state: "hidden", timeout: timeoutMs });
    await waitFor(async () => inner.locator("#pdf-canvas").evaluate((canvas) => {
      const pixels = canvas.getContext("2d").getImageData(0, 0, canvas.width, canvas.height).data;
      return pixels.some((value, index) => index % 4 !== 3 && value < 240);
    }), timeoutMs, "PDF canvas never rendered document pixels");
  } else fail(`Unknown App content assertion: ${appId}`);
}

async function screenshotAndAssert(page, iframe, screenshotPath, timeoutMs) {
  const inner = iframe.contentFrame().frameLocator("iframe");
  await inner.locator("body").waitFor({ state: "visible", timeout: timeoutMs });
  await waitFor(async () => {
    const bounds = await iframe.boundingBox();
    return bounds && bounds.width > 0 && bounds.height > 0;
  }, timeoutMs, "App iframe has no visible dimensions");
  const pixels = await inner.locator("body").screenshot({ path: screenshotPath, animations: "disabled", timeout: timeoutMs });
  if (pixels.length < 1000) fail("App iframe screenshot is unexpectedly small");
  const nonBlankPixels = await page.evaluate(async (base64) => {
    const image = new Image();
    image.src = `data:image/png;base64,${base64}`;
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = image.naturalWidth;
    canvas.height = image.naturalHeight;
    const context = canvas.getContext("2d", { willReadFrequently: true });
    context.drawImage(image, 0, 0);
    const data = context.getImageData(0, 0, canvas.width, canvas.height).data;
    let count = 0;
    for (let i = 0; i < data.length; i += 4) {
      if (data[i + 3] > 0 && (data[i] < 245 || data[i + 1] < 245 || data[i + 2] < 245)) count++;
    }
    return count;
  }, pixels.toString("base64"));
  if (nonBlankPixels < 100) fail(`App iframe is blank (${nonBlankPixels} non-white pixels)`);
  return { screenshot: screenshotPath, nonBlankPixels };
}

async function waitFor(predicate, timeoutMs, message) {
  const started = performance.now();
  while (performance.now() - started < timeoutMs) {
    if (await predicate()) return;
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 100));
  }
  fail(message);
}

async function renderCase(browser, manifest, credentials, appId, route, mode, outDir, secrets, timeoutMs) {
  const app = manifest.apps[appId];
  if (!app || typeof app.native_name !== "string" || !app.native_name) fail(`Missing native tool for ${appId}`);
  const toolName = nativeToolName(app, mode);
  const routeKey = mode === "code_mode" ? "code_mode" : route;
  const gatewayPath = routePath(manifest, routeKey, appId);
  const headers = credentials[routeKey] ?? credentials[route] ?? {};
  const proxy = await startProxy(manifest.base_url, gatewayPath, headers, secrets);
  let target;
  let browserState;
  const id = `${appId}-${mode === "code_mode" ? "direct-codemode" : route}`;
  try {
    target = await startHost(option("--host-dir", ""), [proxy.url]);
    browserState = await pageLogs(browser, target, { tool: toolName, input: app.native_arguments ?? {} }, timeoutMs, secrets);
    await waitFor(() => proxy.calls.some((call) => call.results.some((result) => result.name === toolName)), timeoutMs, "The native App tool result did not complete");
    const calls = proxy.calls.flatMap((call) => call.toolCalls);
    if (!calls.some((call) => call.name === toolName)) fail(`Reference host did not call ${toolName}`);
    const result = proxy.calls.flatMap((call) => call.results).find((item) => item.name === toolName);
    if (!successfulToolResult(result)) fail(`Reference host tool call ${toolName} did not complete successfully`);
    await assertAppContent(browserState.iframe, appId, result.result, timeoutMs);
    const screenshot = await screenshotAndAssert(browserState.page, browserState.iframe, join(outDir, `${id}.png`), timeoutMs);
    return { id, app: appId, route: mode === "code_mode" ? "direct" : route, mode, status: "passed", ...screenshot, gatewayPath, hostLogs: redact(target.logs.join(""), secrets), console: browserState.logs };
  } catch (error) {
    if (appId === "pdf" && browserState?.logs.some((entry) => /content.security.policy|worker-src|worker.*blocked|blocked.*worker/i.test(entry.text))) {
      error.failure_class = "app_server_or_browser_package";
    }
    throw error;
  } finally {
    await writeFile(join(outDir, `${id}.console.json`), JSON.stringify({ console: browserState?.logs ?? [], host: redact(target?.logs.join("") ?? "", secrets), calls: proxy.calls.map((call) => ({ rpcMethod: call.rpcMethod, toolCalls: call.toolCalls.map((tool) => tool.name), results: call.results.map((result) => ({ name: result.name, isError: result.isError })) })) }, null, 2) + "\n", { mode: 0o600 });
    if (browserState) await browserState.context.close();
    if (target) await stopChild(target.child);
    await closeProxy(proxy);
  }
}

async function renderCheckpoint(browser, manifest, credentials, appId, outDir, secrets, timeoutMs) {
  const app = manifest.apps[appId];
  if (!app || !/create_view/i.test(app.native_name)) fail("Checkpoint round-trip requires Excalidraw create_view in the manifest");
  const route = "direct";
  const proxy = await startProxy(manifest.base_url, routePath(manifest, route, appId), credentials.direct ?? {}, secrets);
  let target;
  let browserState;
  const createInput = { ...(app.native_arguments ?? {}), elements: JSON.stringify([{ type: "rectangle", id: "ci-seed", x: 12, y: 14, width: 80, height: 50, label: { text: "render gate" } }]) };
  try {
    target = await startHost(option("--host-dir", ""), [proxy.url]);
    browserState = await pageLogs(browser, target, { tool: app.native_name, input: createInput }, timeoutMs, secrets);
    await screenshotAndAssert(browserState.page, browserState.iframe, join(outDir, "excalidraw-checkpoint-before.png"), timeoutMs);

    const inner = browserState.page.frameLocator("iframe").frameLocator("iframe");
    await inner.getByRole("button", { name: /Edit/ }).click({ timeout: timeoutMs });
    const canvas = inner.locator(".excalidraw").first();
    await canvas.waitFor({ state: "visible", timeout: timeoutMs });
    const bounds = await canvas.boundingBox();
    if (!bounds || bounds.width < 250 || bounds.height < 200) fail("Excalidraw editor canvas did not become usable");
    await canvas.click({ position: { x: bounds.width * 0.12, y: bounds.height * 0.12 } });
    await browserState.page.keyboard.press("r");
    await browserState.page.mouse.move(bounds.x + bounds.width * 0.35, bounds.y + bounds.height * 0.35);
    await browserState.page.mouse.down();
    await browserState.page.mouse.move(bounds.x + bounds.width * 0.48, bounds.y + bounds.height * 0.48, { steps: 5 });
    await browserState.page.mouse.up();
    await waitFor(() => proxy.calls.some((call) => call.results.some((result) => /save_checkpoint/.test(result.name))), timeoutMs, "Excalidraw save_checkpoint callback did not complete after the edit");
    const save = proxy.calls.flatMap((call) => call.results).find((item) => /save_checkpoint/.test(item.name));
    if (save?.isError) fail("Excalidraw save_checkpoint callback returned isError");
    const saveInput = proxy.calls.flatMap((call) => call.toolCalls).find((item) => /save_checkpoint/.test(item.name))?.arguments;
    if (!saveInput?.id || !saveInput?.data) fail("save_checkpoint callback omitted its id or edited data");
    let savedData;
    try { savedData = JSON.parse(saveInput.data); } catch { fail("save_checkpoint callback data was not JSON"); }
    const savedElements = Array.isArray(savedData) ? savedData : savedData.elements;
    if (!Array.isArray(savedElements) || !savedElements.some((element) => element?.id === "ci-seed") || !savedElements.some((element) => element?.id !== "ci-seed")) {
      fail("save_checkpoint callback did not include both the seed and a new edited element");
    }

    // Excalidraw's fullscreen editor overlays the host form until Escape returns it inline.
    await browserState.page.keyboard.press("Escape");
    await inner.getByRole("button", { name: /Edit/ }).waitFor({ state: "visible", timeout: actionTimeout(timeoutMs) });
    await browserState.page.getByLabel(/^Input/).fill(JSON.stringify({ elements: JSON.stringify([{ type: "restoreCheckpoint", id: saveInput.id }]) }));
    await browserState.page.getByRole("button", { name: "Call Tool" }).click();
    await waitFor(() => proxy.calls.some((call) => call.results.some((result) => /read_checkpoint/.test(result.name))), timeoutMs, "Excalidraw read_checkpoint callback did not complete from the App view");
    const read = proxy.calls.flatMap((call) => call.results).find((item) => /read_checkpoint/.test(item.name));
    if (!read || read.isError) fail("Excalidraw read_checkpoint callback failed");
    const readText = (read.result?.content ?? []).filter((block) => block.type === "text").map((block) => block.text).join("\n");
    let readData;
    try { readData = JSON.parse(readText); } catch { fail("read_checkpoint callback did not return JSON checkpoint data"); }
    const elements = Array.isArray(readData) ? readData : readData.elements;
    if (!Array.isArray(elements) || !elements.some((element) => element?.id === "ci-seed") || !elements.some((element) => element?.id !== "ci-seed")) fail("read_checkpoint did not return the saved edited diagram");
    const editedScreenshot = await screenshotAndAssert(browserState.page, browserState.page.locator("iframe").last(), join(outDir, "excalidraw-checkpoint-after.png"), timeoutMs);
    return { id: "excalidraw-checkpoint-roundtrip", app: appId, route, status: "passed", checkpointId: saveInput.id, editedElementCount: elements.length, ...editedScreenshot, callbackCalls: [save.name, read.name], hostLogs: redact(target.logs.join(""), secrets), console: browserState.logs };
  } finally {
    await writeFile(join(outDir, "excalidraw-checkpoint.console.json"), JSON.stringify({ console: browserState?.logs ?? [], host: redact(target?.logs.join("") ?? "", secrets), calls: proxy.calls.map((call) => ({ rpcMethod: call.rpcMethod, toolCalls: call.toolCalls.map((tool) => tool.name), results: call.results.map((result) => ({ name: result.name, isError: result.isError })) })) }, null, 2) + "\n", { mode: 0o600 });
    if (browserState) await browserState.context.close();
    if (target) await stopChild(target.child);
    await closeProxy(proxy);
  }
}

async function checkFourConnections(browser, manifest, credentials, outDir, secrets, timeoutMs) {
  const appIds = Object.keys(manifest.apps);
  if (appIds.length !== 4) fail(`Expected four Apps for the connection-stall check; found ${appIds.length}`);
  const proxies = await Promise.all(appIds.map((appId) => startProxy(manifest.base_url, routePath(manifest, "direct", appId), credentials.direct ?? {}, secrets)));
  let target;
  let context;
  let browserState;
  try {
    target = await startHost(option("--host-dir", ""), proxies.map((proxy) => proxy.url));
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const page = await context.newPage();
    const logs = [];
    page.on("console", (message) => logs.push({ type: message.type(), text: redact(message.text(), secrets) }));
    page.on("pageerror", (error) => logs.push({ type: "pageerror", text: redact(error.message, secrets) }));
    await page.goto(target.hostUrl, { waitUntil: "domcontentloaded", timeout: timeoutMs });
    const serverSelect = page.getByLabel(/^Server/);
    await waitFor(async () => (await serverSelect.locator("option").count()) === 4, timeoutMs, "Reference host did not load all four MCP connections");
    for (const proxy of proxies) await waitFor(() => proxy.calls.some((call) => call.rpcMethod.includes("tools/list")), timeoutMs, "One of the four connections stalled before tools/list");
    await page.screenshot({ path: join(outDir, "four-connections.png"), fullPage: true, animations: "disabled" });
    browserState = { context, logs };
    return { id: "four-connections-stall", status: "passed", connections: proxies.length, allListedTools: proxies.every((proxy) => proxy.calls.some((call) => call.rpcMethod.includes("tools/list"))), hostLogs: redact(target.logs.join(""), secrets), console: logs };
  } finally {
    await writeFile(join(outDir, "four-connections.console.json"), JSON.stringify({ console: browserState?.logs ?? [], host: redact(target?.logs.join("") ?? "", secrets), calls: proxies.map((proxy) => proxy.calls.map((call) => ({ rpcMethod: call.rpcMethod, toolCalls: call.toolCalls.map((tool) => tool.name) }))) }, null, 2) + "\n", { mode: 0o600 });
    if (context) await context.close();
    if (target) await stopChild(target.child);
    await Promise.all(proxies.map(closeProxy));
  }
}

async function main() {
  if (process.argv.includes("--self-check")) {
    await selfCheck();
    return;
  }
  if (process.argv.includes("--help")) {
    console.log("Usage: node tests/mcpapps/render-reference-host.mjs --manifest <qualification-manifest.json> --host-dir <pinned-ext-apps-root> --out <artifact-directory> [--timeout-ms 120000]");
    return;
  }
  const manifestPath = option("--manifest", null);
  const hostDir = option("--host-dir", null);
  const outDir = resolve(option("--out", "artifacts/mcpapps-render"));
  const timeoutMs = Number(option("--timeout-ms", "120000"));
  if (!manifestPath || !hostDir) fail("--manifest and --host-dir are required; run bootstrap-reference-host.mjs first");
  if (!Number.isFinite(timeoutMs) || timeoutMs < 1000) fail("--timeout-ms must be at least 1000");
  await mkdir(outDir, { recursive: true });

  const hostRoot = resolve(hostDir);
  const hostReceipt = JSON.parse(await readFile(join(hostRoot, ".bifrost-reference-host.json"), "utf8").catch(() => "null"));
  if (hostReceipt?.repository !== `${HOST_REPOSITORY}.git` || hostReceipt.version !== HOST_VERSION || hostReceipt.commit !== HOST_COMMIT) fail("Reference host was not bootstrapped from the pinned official commit");
  const manifest = JSON.parse(await readFile(resolve(manifestPath), "utf8"));
  if (typeof manifest.base_url !== "string" || !manifest.apps || !manifest.routes || typeof manifest.credentials_file !== "string") fail("Qualification manifest must include base_url, apps, routes and credentials_file");
  const expectedApps = ["excalidraw", "pdf", "debug", "monitor"];
  if (JSON.stringify(Object.keys(manifest.apps).sort()) !== JSON.stringify(expectedApps.slice().sort())) fail("Renderer requires exactly the Excalidraw, PDF, Debug and Monitor Apps");
  for (const appId of expectedApps) {
    if (typeof manifest.apps[appId].native_name !== "string" || typeof manifest.apps[appId].native_arguments !== "object") fail(`Invalid render configuration for ${appId}`);
    for (const route of ["direct", "virtual", "aggregate", "code_mode"]) routePath(manifest, route, appId);
  }
  const credentials = JSON.parse(await readFile(resolve(manifest.credentials_file), "utf8"));
  const profiles = readHeaderProfiles(credentials);
  const secrets = [...Object.values(profiles).flatMap((headers) => Object.values(headers)), ...(credentials.secrets ?? [])].filter((value) => typeof value === "string");

  let chromium;
  try { ({ chromium } = require("@playwright/test")); }
  catch { ({ chromium } = require("playwright")); }
  if (!chromium) fail("Playwright is missing; run npm ci in tests/e2e first");
  const started = performance.now();
  const browser = await chromium.launch({ headless: true });
  const results = [];
  try {
    for (const appId of Object.keys(manifest.apps)) {
      for (const route of ["direct", "virtual", "aggregate"]) {
        const headers = profiles[route];
        credentials[route] = headers;
        try { results.push(await renderCase(browser, manifest, credentials, appId, route, "normal", outDir, secrets, timeoutMs)); }
        catch (error) { results.push({ id: `${appId}-${route}`, app: appId, route, mode: "normal", status: "failed", failure_class: classifyFailure(appId, error), error: redact(error.message, secrets) }); }
      }
      credentials.code_mode = profiles.code_mode;
      try { results.push(await renderCase(browser, manifest, credentials, appId, "direct", "code_mode", outDir, secrets, timeoutMs)); }
      catch (error) { results.push({ id: `${appId}-direct-codemode`, app: appId, route: "direct", mode: "code_mode", status: "failed", failure_class: classifyFailure(appId, error), error: redact(error.message, secrets) }); }
    }
    const excalidrawId = Object.keys(manifest.apps).find((id) => /excalidraw/i.test(id));
    if (excalidrawId) {
      try { results.push(await renderCheckpoint(browser, manifest, profiles, excalidrawId, outDir, secrets, timeoutMs)); }
      catch (error) { results.push({ id: "excalidraw-checkpoint-roundtrip", status: "failed", failure_class: classifyFailure(excalidrawId, error), error: redact(error.message, secrets) }); }
    } else results.push({ id: "excalidraw-checkpoint-roundtrip", status: "failed", error: "Manifest has no Excalidraw App" });
    try { results.push(await checkFourConnections(browser, manifest, profiles, outDir, secrets, timeoutMs)); }
    catch (error) { results.push({ id: "four-connections-stall", status: "failed", failure_class: classifyFailure("host", error), error: redact(error.message, secrets) }); }
  } finally {
    await browser.close();
  }

  const report = {
    referenceHost: { repository: HOST_REPOSITORY, version: HOST_VERSION, commit: HOST_COMMIT },
    runtimeSeconds: Math.round((performance.now() - started) / 1000),
    results,
    passed: results.filter((result) => result.status === "passed").length,
    failed: results.filter((result) => result.status !== "passed").length,
  };
  await writeFile(join(outDir, "render-report.json"), JSON.stringify(report, null, 2) + "\n", { mode: 0o600 });
  console.log(`MCP Apps reference-host render: ${report.passed}/${results.length} passed in ${report.runtimeSeconds}s; report: ${join(outDir, "render-report.json")}`);
  if (report.failed) process.exitCode = 1;
}

main().catch((error) => {
  console.error(redact(error.stack ?? error.message, []));
  process.exitCode = 1;
});
