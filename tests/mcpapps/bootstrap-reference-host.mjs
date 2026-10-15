#!/usr/bin/env node
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, mkdir, readdir, writeFile } from "node:fs/promises";
import { resolve, join } from "node:path";

const repository = "https://github.com/modelcontextprotocol/ext-apps.git";
const commit = "82221c0c8ce7661efa6771c9d461511b1650495f";
const version = "2.0.3";

function run(command, args, cwd) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { cwd, stdio: "inherit" });
    child.once("error", reject);
    child.once("exit", (code, signal) => code === 0 ? resolvePromise() : reject(new Error(`${command} exited ${code ?? signal}`)));
  });
}

const args = process.argv.slice(2);
if (args.includes("--help")) {
  console.log("Usage: node bootstrap-reference-host.mjs --directory <empty-directory>");
  process.exit(0);
}
const directoryFlag = args.indexOf("--directory");
if (directoryFlag < 0 || !args[directoryFlag + 1]) throw new Error("--directory is required");
const directory = resolve(args[directoryFlag + 1]);
await mkdir(directory, { recursive: true });
if ((await readdir(directory)).length > 0) {
  throw new Error(`Refusing to bootstrap into non-empty directory: ${directory}`);
}

await run("git", ["clone", "--filter=blob:none", "--no-checkout", repository, directory]);
await run("git", ["checkout", "--detach", commit], directory);
const actualCommit = (await import("node:child_process")).execFileSync("git", ["rev-parse", "HEAD"], { cwd: directory, encoding: "utf8" }).trim();
if (actualCommit !== commit) throw new Error(`Reference host commit mismatch: ${actualCommit}`);

const manifest = JSON.parse(await readFile(join(directory, "examples/basic-host/package.json"), "utf8"));
if (manifest.version !== version) throw new Error(`Reference host version mismatch: ${manifest.version}`);
// The workspace's dependency points at the repository-local SDK. Including the
// root runs its pinned prepare/build step and makes that SDK available to Vite.
await run("npm", ["ci", "--workspace=examples/basic-host", "--include-workspace-root=true"], directory);
// The basic host sends complete input only. Add one opt-in test hook before it,
// preserving the pinned SDK, initialization and original final-input/result path.
const sourceFile = "examples/basic-host/src/implementation.ts";
const source = await readFile(join(directory, sourceFile), "utf8");
const marker = "appBridge.sendToolInput({ arguments: input });";
if (source.split(marker).length !== 2) throw new Error("Pinned host input seam changed");
const adapted = source.replace(marker, `const replayPartialInput = Reflect.get(window, "__bifrostReplayPartialInput");
  if (typeof replayPartialInput === "function") await replayPartialInput(appBridge, input);
  ${marker}`);
await writeFile(join(directory, sourceFile), adapted);
const sha256 = (text) => createHash("sha256").update(text).digest("hex");
const adaptation = { sourceFile, sourceSha256: sha256(source), adaptedSha256: sha256(adapted),
  purpose: "Opt-in real AppBridge partial-input hook before the original final input/result" };
await run("npm", ["run", "build", "--workspace=examples/basic-host"], directory);
await writeFile(join(directory, ".bifrost-reference-host.json"), JSON.stringify({ repository, version, commit, adaptation }, null, 2) + "\n", { mode: 0o600 });
console.log(`Reference host ${version} built from ${commit}`);
