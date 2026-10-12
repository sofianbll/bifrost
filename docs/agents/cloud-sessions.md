# Cloud coding sessions

Fork-only: this belongs to the fork plumbing (branch d of the stack in #4) and never goes into an upstream PR.

The repository supports both Claude Code remote sessions and Codex Cloud for preparing the MCP Go tests. The same `.claude/cloud/setup.sh` script keeps Claude's existing best-effort behavior and adds an explicit `--codex` mode for Codex Cloud. It does nothing for local Claude invocations; Codex mode is selected only by passing the argument. No provider credentials or secrets are needed.

## Codex Cloud

Codex Cloud does not use the Claude SessionStart hook. When preparing the Codex Cloud environment, use this setup command in its install/setup phase:

```bash
bash .claude/cloud/setup.sh --codex
```

The script verifies that `go` is a real compiler supporting automatic toolchain selection (Go 1.21+). If it is missing, too old, or another program with the same name, it installs Go 1.27.0 from the official archive into `${XDG_CACHE_HOME:-$HOME/.cache}/bifrost-go/1.27.0`, verifies its pinned SHA-256 and compiler version, and reuses that isolated installation on later starts. It then prepares MCP test dependencies, reports any failed step with a nonzero exit status, and prints the platform and elapsed time. Detailed output goes to `$TMPDIR/cloud-setup-codex.log`. It accepts `gotestsum` already available on `PATH`; otherwise it checks the configured `go env GOBIN`, falling back to `go env GOPATH` plus `/bin`.

For a runtime with a read-only home directory, set these ordinary cache variables in the environment configuration. Use a verified writable directory outside the checkout; do not override `HOME` or change filesystem permissions.

| Environment variable | Value for the inspected runtime |
| --- | --- |
| `XDG_CACHE_HOME` | `/workspace/.cache` |
| `GOPATH` | `/workspace/.cache/go` |
| `GOCACHE` | `/workspace/.cache/go-build` |
| `npm_config_cache` | `/workspace/.cache/npm` |
| `GONOPROXY` | `github.com/klauspost/compress` |

These values must apply in both the install and agent phases. They contain no credentials. The scoped `GONOPROXY` uses Go's official VCS source transport for the one public module whose proxy archive redirects to a denied Google Storage host; GitHub stays behind the inherited outbound proxy and module checksum validation remains enabled. See [Go module environment variables](https://go.dev/ref/mod#environment-variables). Verify cache persistence in the second task; setting a writable path does not prove snapshot reuse.

The current Codex Cloud guide describes an Install script and a published environment snapshot; if the setup flow instead opens a setup terminal, run the same command there. Resolve the test executable again in the agent phase instead of relying on the setup process to preserve its shell environment. Run tests in one shell block with the Go binary directory added explicitly:

```bash
set -e
bifrost_go="${XDG_CACHE_HOME:-$HOME/.cache}/bifrost-go/1.27.0"
if [[ $("$bifrost_go/bin/go" version 2>/dev/null) == "go version go1.27.0 "* ]]; then
  export GOROOT="$bifrost_go"
  export PATH="$GOROOT/bin:$PATH"
fi
export GOTOOLCHAIN=auto
if ! command -v gotestsum >/dev/null 2>&1; then
  go_bin="$(go env GOBIN)"
  if [ -z "$go_bin" ]; then go_bin="$(go env GOPATH)/bin"; fi
  export PATH="$go_bin:$PATH"
fi
command -v gotestsum
go version
(cd core && go test -count=1 ./mcp/...)
make test-mcp
```

This resolves the isolated compiler before calling `go env` in the new shell. It lets the Makefile's `install-gotestsum` check and the later direct `gotestsum` invocation use the same executable, without assuming or changing a system-wide PATH. Codex Cloud documents Install script setup and isolated task workspaces in its [Cloud environments guide](https://learn.chatgpt.com/docs/environments/cloud-environments); add this script in the environment configuration, review the setup report, then publish or republish the environment before starting a new task. Package-manager network access covers the official Go archive host (`dl.google.com`), Go proxy/sums and npm registries needed here; this workflow needs no provider APIs.

The Start skill is a Cloud configuration field; if a task cannot discover it as a filesystem skill, use the documented shell block above directly. The tests need local sockets and Chromium needs socket system calls. Where the execution tool requires approval outside its command sandbox, request approval for those test commands only; keep the environment's network policy, proxy, mounts and global permissions intact.

Verify the prepared environment before publication, then run the command block above in two new Codex Cloud tasks: the first from the published snapshot and a second after preparation is cached. Record task creation, runtime readiness, the `cloud-setup (codex)` platform/timing line and test results separately; script time alone is not VM startup time. Editing a draft does not rerun setup in an existing task. Actual cloud qualification is tracked in [the recovery ledger](mcp-apps-recovery.md).

## Verified Codex qualification — 2026-10-12

The private environment is published at code SHA `62b1e58fcd1df57a5c719ce16e16b8534cf0c6e3`. It preserves the Package managers preset, no additional domains and no provider secrets. The prepared runtime passes bootstrap (9 seconds with warm dependencies), core/mcp and the full MCP suite, and launches system Chromium.

| Check | First new runtime | Second new runtime |
| --- | --- | --- |
| Creation to first terminal | 50.028 seconds | 51 seconds |
| Prepared caches present before any installation | yes | yes |
| core/mcp, `-count=1` | pass, 8.583 seconds | pass, 9.005 seconds |
| MCP race suite | pass, 128.081 seconds | pass, 127.859 seconds |
| Fresh JUnit | 704 total / 675 passed / 29 skipped / 0 failures or errors | 704 total / 675 passed / 29 skipped / 0 failures or errors |
| Chromium launch/close | pass, 0.689 seconds | pass, 0.706 seconds |

Both are new runtimes restored from the same prepared snapshot. These observations do not measure dependency installation from zero or pure VM provisioning; creation-to-terminal includes agent planning. The earlier 75-second bootstrap failed at a proxy redirect and must not be reported as a successful cold bootstrap. Normal-task tests used approved execution limited to the commands, plus `make -o install-gotestsum -o setup-mcp-tests test-mcp EXPOSE_ENV=:` to check already-prepared fixtures without installation or credential `.env` loading. Source files and global permissions were unchanged. See [the recovery ledger](mcp-apps-recovery.md) for the evidence and remaining final gates.

## Claude Code Cloud

The no-argument mode remains for Claude Code Cloud. It does nothing unless `CLAUDE_CODE_REMOTE=true`, and continues never to exit non-zero so a failing SessionStart hook does not block the session. It runs from two places:

- **Environment setup script** (claude.ai, cached): warms the toolchain and caches once. A setup script is snapshotted only if it finishes in about five minutes, and rebuilt after about seven days or when you edit it.
- **SessionStart hook** (`.claude/settings.json`, every start and resume): re-runs the same steps, which is cheap but not free when the cache is warm (14s on a warm local machine), and does the full work when the cache is cold. It is also the only place `CLAUDE_ENV_FILE` exists, so it puts `$GOPATH/bin` on `PATH` for `gotestsum`.

What it does: resolves the Go toolchain pinned by `go.work` (Go 1.21+ downloads it through `proxy.golang.org` itself), warms the modules the two suites load with `go list -deps -test` in `core` (plain `go mod download` fetches the whole module graph and overruns the cache limit), then runs `make install-gotestsum setup-mcp-tests` (gotestsum, the example MCP servers, their npm dependencies). Output goes to `$TMPDIR/cloud-setup.log`; the one summary line it prints (`cloud-setup: ok in 42s`) lands in the session context, so the start-up cost is visible there.

## Owner step: environment settings in claude.ai

| Setting | Value |
| --- | --- |
| Network access | **Trusted** (default). It covers `proxy.golang.org`, `sum.golang.org`, `registry.npmjs.org` and the Ubuntu apt mirrors. |
| Setup script | `bash .claude/cloud/setup.sh \|\| true` (assumes the repo is the working directory when the script runs; unverified, see below) |
| Environment variables | None. No secrets are needed for these suites. |

## Playwright (opt-in)

Set `BIFROST_CLOUD_PLAYWRIGHT=1` in the install command to check a headless browser. The script reuses `chromium` or `chromium-browser` already on PATH, installs the locked npm dependencies and passes its executable path explicitly to Playwright. This requires no browser CDN access.

When no system Chromium exists, the existing `make install-playwright` fallback downloads Playwright Chromium and its OS dependencies. That fallback needs browser CDN access (`cdn.playwright.dev` and `playwright.download.prss.microsoft.com`) and apt permissions; request any required network change before applying it. A successful browser launch is a dependency check, not MCP App rendering proof. #13 has separate GitHub Actions rendering evidence.

## Verify in the first cloud session

Not verifiable from a local machine, so check these in a real session:

```bash
head -3 "${TMPDIR:-/tmp}/cloud-setup.log"      # was the repo the cwd when the setup script ran?
go version && command -v gotestsum gcc          # toolchain 1.27.x, gotestsum on PATH, gcc (make test-mcp uses -race)
(cd core && go test ./mcp/...)                  # expected green
make test-mcp                                   # see the known failures below
```

**Historical failures on the preserved branch.** The October 5 CI and local run on `feature/mcp-apps-native` recorded: 7 `TestCodeMode_STDIO_ErrorTestServer_*` tests in `core/internal/mcptests/codemode_stdio_test.go` still assert the pre-`50e9bd199` behavior (a failing nested tool must not raise an execution error). A local run of 698 tests (29 skipped) had only those 7 failures. The active 2.2.6 stack preserves upstream Chat tool-error values, as chosen by the owner on October 11. All tests must pass on the new stack; the seven historical failures are not accepted failures for it. See [the recovery ledger](mcp-apps-recovery.md) for current verification gates.

**Start-up overhead** (a criterion of #3). The `cloud-setup: ok in Ns` line times the script only, not VM provisioning. Compare two sessions: the first in a fresh environment (cold: the setup script runs, then the hook) and a second started after the environment cache exists (warm: the hook only). Compare each session's start-to-first-prompt time at claude.ai/code, and post both numbers and the `cloud-setup` lines as a comment on #3. If the setup script exceeds roughly five minutes, the environment is not cached; drop the slowest step from it and let the hook do that step.

With the Playwright opt-in on, use the same installed browser in the agent phase:

```bash
bifrost_chromium=$(command -v chromium || command -v chromium-browser || true)
(cd tests/e2e && node -e "require('@playwright/test').chromium.launch(process.argv[1] ? {executablePath: process.argv[1]} : {}).then(b => b.close())" "$bifrost_chromium")
```

Exit 0 confirms a headless launch; retain any failure as a failed cloud gate.

If `go version` fails because `GOTOOLCHAIN` is `local`, add `GOTOOLCHAIN=auto` as an environment variable. If the setup script does not find the repo, the hook still does the same work on every session; fix the path in the setup script field.

The environment facts above (Trusted domains incl. the Ubuntu apt mirrors, the setup script running as root, the roughly five-minute cache limit and seven-day expiry, SessionStart hooks having a 600 s timeout) are from <https://code.claude.com/docs/en/cloud-environments> as read on 2026-10-05. The working directory of the setup script is not documented, hence the check above.

`bash .claude/cloud/setup.test.sh` checks the script's own logic (Claude no-op/best-effort, Codex failures, verified Go installation/cache and Playwright reuse/fallback) against stubbed tools, with no network.
