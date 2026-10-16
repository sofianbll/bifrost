#!/usr/bin/env node
import { spawn, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFile, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repository = "https://github.com/modelcontextprotocol/ext-apps.git";
const commit = "82221c0c8ce7661efa6771c9d461511b1650495f";
const version = "2.0.3";
const here = dirname(fileURLToPath(import.meta.url));
const bifrostRoot = resolve(here, "../..");

function run(command, args, cwd) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { cwd, stdio: "inherit" });
    child.once("error", reject);
    child.once("exit", (code, signal) => code === 0 ? resolvePromise() : reject(new Error(`${command} exited ${code ?? signal}`)));
  });
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const args = process.argv.slice(2);
if (args.includes("--help")) {
  console.log("Usage: node tests/mcpapps/bootstrap-reference-host.mjs --directory <empty-directory>");
  process.exit(0);
}
const directoryFlag = args.indexOf("--directory");
if (directoryFlag < 0 || !args[directoryFlag + 1]) throw new Error("--directory is required");
const directory = resolve(args[directoryFlag + 1]);
await mkdir(directory, { recursive: true });
if ((await readdir(directory)).length > 0) throw new Error(`Refusing to bootstrap into non-empty directory: ${directory}`);

await run("git", ["clone", "--filter=blob:none", "--no-checkout", repository, directory], bifrostRoot);
await run("git", ["checkout", "--detach", commit], directory);
const actualCommit = execFileSync("git", ["rev-parse", "HEAD"], { cwd: directory, encoding: "utf8" }).trim();
if (actualCommit !== commit) throw new Error(`Reference host commit mismatch: ${actualCommit}`);

const manifest = JSON.parse(await readFile(join(directory, "examples/basic-host/package.json"), "utf8"));
if (manifest.version !== version) throw new Error(`Reference host version mismatch: ${manifest.version}`);
const packageLock = JSON.parse(await readFile(join(directory, "package-lock.json"), "utf8"));
const sdkLink = packageLock.packages?.["node_modules/@modelcontextprotocol/ext-apps"];
if (packageLock.packages?.[""]?.version !== version || sdkLink?.link !== true) {
  throw new Error("Pinned SDK must resolve from the version-matched ext-apps workspace recorded by package-lock.json");
}
await run("npm", ["ci", "--workspace=examples/basic-host", "--include-workspace-root=true"], directory);

// The official host takes URL strings only. This isolated adapter obtains one
// synthetic fixture credential from its local host API, matches it to exactly
// one configured gateway URL, and gives it to the existing MCP transport as an
// Authorization header. It never changes a server URL or gateway policy.
const servePath = "examples/basic-host/serve.ts";
const serveSource = await readFile(join(directory, servePath), "utf8");
const serversMarker = 'hostApp.get("/api/servers", (_req, res) => {\n  res.json(SERVERS);\n});';
if (serveSource.split(serversMarker).length !== 2) throw new Error("Pinned host server-list seam changed");
const serveAdapter = `${serversMarker}\n\n// Test-only endpoint: the value is accepted only as a synthetic fixture key.\nhostApp.get("/api/fixture-auth", (_req, res) => {\n  const serverUrl = process.env.BIFROST_FIXTURE_SERVER_URL;\n  const bearer = process.env.BIFROST_FIXTURE_BEARER;\n  if (!serverUrl || !bearer) {\n    res.status(404).end();\n    return;\n  }\n  if (!/^sk-bf-mcp-apps-(full|limited|limited-no-app|denied)$/.test(bearer)) {\n    res.status(500).end();\n    return;\n  }\n  res.set("Cache-Control", "no-store");\n  res.json({ serverUrl, bearer });\n});`;
const adaptedServe = serveSource.replace(serversMarker, serveAdapter);
await writeFile(join(directory, servePath), adaptedServe);

const transportPath = "examples/basic-host/src/implementation.ts";
const transportSource = await readFile(join(directory, transportPath), "utf8");
const connectMarker = "async function connectWithFallback(serverUrl: URL): Promise<Client> {";
if (transportSource.split(connectMarker).length !== 2) throw new Error("Pinned host transport seam changed");
const authHelper = `async function fixtureTransportOptions(serverUrl: URL) {\n  const response = await fetch("/api/fixture-auth");\n  if (response.status === 404) return undefined;\n  if (!response.ok) throw new Error("Fixture auth adapter rejected its synthetic test configuration");\n  const fixture = await response.json() as { serverUrl: string; bearer: string };\n  if (new URL(fixture.serverUrl).href !== serverUrl.href) return undefined;\n  const headers = new Headers();\n  headers.set("Authorization", \`Bearer \${fixture.bearer}\`);\n  return { requestInit: { headers } };\n}\n\n`;
let adaptedTransport = transportSource.replace(connectMarker, `${authHelper}${connectMarker}`);
const streamableMarker = "new StreamableHTTPClientTransport(serverUrl)";
const sseMarker = "new SSEClientTransport(serverUrl)";
if (adaptedTransport.split(streamableMarker).length !== 2 || adaptedTransport.split(sseMarker).length !== 2) {
  throw new Error("Pinned host transport constructor seams changed");
}
adaptedTransport = adaptedTransport.replace(connectMarker, `${connectMarker}\n  const fixtureOptions = await fixtureTransportOptions(serverUrl);`);
adaptedTransport = adaptedTransport.replace(streamableMarker, "new StreamableHTTPClientTransport(serverUrl, fixtureOptions)");
adaptedTransport = adaptedTransport.replace(sseMarker, "new SSEClientTransport(serverUrl, fixtureOptions)");

// The pinned basic host does not advertise MCP Apps by default. The fixture
// qualification host explicitly opts into the official UI extension before
// either transport initializes; ordinary host behavior remains untouched.
const clientMarker = "const client = new Client(IMPLEMENTATION);";
if (adaptedTransport.split(clientMarker).length !== 3) throw new Error("Pinned host client creation seams changed");
const negotiatedTransport = adaptedTransport.replaceAll(
  clientMarker,
  `${clientMarker}\n    client.registerCapabilities({ extensions: { "io.modelcontextprotocol/ui": { mimeTypes: ["text/html;profile=mcp-app"] } } });`,
);

// Separately gated controlled replay through the pinned host's official
// AppBridge API. It is a deterministic protocol control, not model streaming.
const partialReplayMarker = "appBridge.sendToolInput({ arguments: input });";
if (negotiatedTransport.split(partialReplayMarker).length !== 2) throw new Error("Pinned host full-input seam changed");
const adaptedPartialReplay = negotiatedTransport.replace(
  partialReplayMarker,
  `const replayPartialInput = Reflect.get(window, "__bifrostReplayPartialInput");
  if (typeof replayPartialInput === "function") await replayPartialInput(appBridge, input);
  ${partialReplayMarker}`,
);
await writeFile(join(directory, transportPath), adaptedPartialReplay);

const fixtureEntry = join(directory, "examples/basic-host/src/bifrost-fixture-app.ts");
await copyFile(join(bifrostRoot, "tests/mcpapps/fixture-app.ts"), fixtureEntry);
await run("npm", ["run", "build", "--workspace=examples/basic-host"], directory);
await run("./node_modules/.bin/esbuild", [
  "examples/basic-host/src/bifrost-fixture-app.ts", "--bundle", "--minify", "--target=es2022",
  "--format=esm", "--legal-comments=inline", "--outfile=examples/basic-host/dist/mcp-app-fixture.js",
], directory);

const generatedBundle = await readFile(join(directory, "examples/basic-host/dist/mcp-app-fixture.js"));
const faviconBytes = Buffer.from("AAABAAEAAQEAAAEAIAAwAAAAFgAAACgAAAABAAAAAgAAAAEAIAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==", "base64");
const faviconPath = "examples/basic-host/dist/favicon.ico";
const faviconSha256 = "5bfe18012b31dfdba82299e1e2f69c5f6ca42e17928ffa8ee25079d55a24df0a";
if (faviconBytes.byteLength !== 70 || sha256(faviconBytes) !== faviconSha256 ||
    faviconBytes.readUInt16LE(0) !== 0 || faviconBytes.readUInt16LE(2) !== 1 ||
    faviconBytes.readUInt16LE(4) !== 1 || faviconBytes.readUInt32LE(18) !== 22) {
  throw new Error("Deterministic test-host favicon bytes are invalid");
}
await writeFile(join(directory, faviconPath), faviconBytes);
const adapter = {
  serveFile: servePath,
  serveSourceSha256: sha256(serveSource),
  serveAdaptedSha256: sha256(adaptedServe),
  transportFile: transportPath,
  transportSourceSha256: sha256(transportSource),
  transportAdaptedSha256: sha256(adaptedTransport),
  patchSha256: sha256(`${serveSource}\0${adaptedServe}\0${transportSource}\0${adaptedTransport}`),
  purpose: "Per-URL synthetic fixture Bearer header via requestInit in the pinned MCP transports",
};
const negotiationAdapter = {
  transportFile: transportPath,
  authAdaptedSourceSha256: sha256(adaptedTransport),
  negotiatedSourceSha256: sha256(negotiatedTransport),
  patchSha256: sha256(`${adaptedTransport}\0${negotiatedTransport}`),
  capability: { extension: "io.modelcontextprotocol/ui", mimeTypes: ["text/html;profile=mcp-app"] },
  purpose: "Explicitly negotiate MCP Apps support in the test-only pinned reference-host client",
};
const output = {
  repository,
  version,
  commit,
  sdkDependency: {
    hostDeclaredRange: manifest.dependencies["@modelcontextprotocol/ext-apps"],
    workspaceVersion: packageLock.packages[""].version,
    packageLockWorkspaceLink: sdkLink.link,
    provenance: "SDK source is built from the pinned repository workspace; no npm tarball integrity is claimed",
  },
  hostFixtureAuthAdapter: adapter,
  hostFixtureNegotiationAdapter: negotiationAdapter,
  controlledPartialReplay: {
    transportFile: transportPath,
    negotiatedSourceSha256: sha256(negotiatedTransport),
    partialReplayAdaptedSourceSha256: sha256(adaptedPartialReplay),
    patchSha256: sha256(`${negotiatedTransport}\0${adaptedPartialReplay}`),
    seam: "window.__bifrostReplayPartialInput(AppBridge,input) is an opt-in test hook before pinned host final-input/result calls",
  },
  appBundle: {
    entry: "tests/mcpapps/fixture-app.ts",
    entrySha256: sha256(await readFile(join(bifrostRoot, "tests/mcpapps/fixture-app.ts"))),
    output: "examples/basic-host/dist/mcp-app-fixture.js",
    sha256: sha256(generatedBundle),
    bytes: generatedBundle.byteLength,
    source: "@modelcontextprotocol/ext-apps 2.0.3 App built from the pinned repository workspace link",
  },
  faviconAsset: {
    path: faviconPath,
    bytes: faviconBytes.byteLength,
    sha256: faviconSha256,
    mediaType: "image/x-icon",
    purpose: "Deterministic transparent test-host favicon for Chromium's default /favicon.ico request",
  },
};
await writeFile(join(directory, ".bifrost-reference-host.json"), `${JSON.stringify(output, null, 2)}\n`, { mode: 0o600 });
console.log(`Reference host ${version} built from ${commit}`);
console.log(`Fixture App SDK bundle SHA-256 ${output.appBundle.sha256}`);
