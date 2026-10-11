# Cloud coding sessions

Fork-only: this belongs to the fork plumbing (branch d of the stack in #4) and never goes into an upstream PR.

The repository supports both Claude Code remote sessions and Codex Cloud for preparing the MCP Go tests. The same `.claude/cloud/setup.sh` script keeps Claude's existing best-effort behavior and adds an explicit `--codex` mode for Codex Cloud. It does nothing for local Claude invocations; Codex mode is selected only by passing the argument. No provider credentials or secrets are needed.

## Codex Cloud

Codex Cloud does not use the Claude SessionStart hook. When preparing the Codex Cloud environment, use this setup command in its install/setup phase:

```bash
bash .claude/cloud/setup.sh --codex
```

The script prepares the pinned Go toolchain and MCP test dependencies, reports any failed step with a nonzero exit status, and prints the platform and elapsed time. Detailed output goes to `$TMPDIR/cloud-setup-codex.log`. It accepts `gotestsum` already available on `PATH`; otherwise it checks the configured `go env GOBIN`, falling back to `go env GOPATH` plus `/bin`.

The current Codex Cloud guide describes an Install script and a published environment snapshot; if the setup flow instead opens a setup terminal, run the same command there. Resolve the test executable again in the agent phase instead of relying on the setup process to preserve its shell environment. Run tests in one shell block with the Go binary directory added explicitly:

```bash
export GOTOOLCHAIN=auto
if ! command -v gotestsum >/dev/null 2>&1; then
  go_bin="$(go env GOBIN)"
  if [ -z "$go_bin" ]; then go_bin="$(go env GOPATH)/bin"; fi
  export PATH="$go_bin:$PATH"
fi
command -v gotestsum
go version
(cd core && go test ./mcp/...)
make test-mcp
```

This lets the Makefile's `install-gotestsum` check and the later direct `gotestsum` invocation use the same executable, without assuming or changing a system-wide PATH. Codex Cloud documents Install script setup and isolated task workspaces in its [Cloud environments guide](https://learn.chatgpt.com/docs/environments/cloud-environments); add this script in the environment configuration, review the setup report, then publish or republish the environment before starting a new task. Package-manager network access covers the Go proxy/sums and npm registries needed here; this workflow needs no provider APIs.

Verify in the first Codex Cloud task with the command block above. Record the `cloud-setup (codex)` platform/timing line and the test results when assessing setup overhead. This repository change does not configure or publish the user's Codex Cloud environment, and the script has not been run in an actual cloud task.

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

Not needed for fork issue #13, which runs in GitHub Actions. The browser CDN is not on the Trusted list, so it is off by default. To turn it on:

1. Network access: **Custom**, include the defaults, and add `cdn.playwright.dev` and `playwright.download.prss.microsoft.com`. These hosts are from Playwright's own download logic, not from the cloud docs; if the install fails, the log names the host it could not reach.
2. Environment variable: `BIFROST_CLOUD_PLAYWRIGHT=1`.

The script then runs `make install-playwright` (`npm ci` plus `playwright install --with-deps chromium`) and launches Chromium headless once as a check. The setup script runs as root, which `--with-deps` (apt) needs. Expect this to push the setup script towards the five-minute cache limit.

## Verify in the first cloud session

Not verifiable from a local machine, so check these in a real session:

```bash
head -3 "${TMPDIR:-/tmp}/cloud-setup.log"      # was the repo the cwd when the setup script ran?
go version && command -v gotestsum gcc          # toolchain 1.27.x, gotestsum on PATH, gcc (make test-mcp uses -race)
(cd core && go test ./mcp/...)                  # expected green
make test-mcp                                   # see the known failures below
```

**Historical failures on the preserved branch.** The October 5 CI and local run on `feature/mcp-apps-native` recorded: 7 `TestCodeMode_STDIO_ErrorTestServer_*` tests in `core/internal/mcptests/codemode_stdio_test.go` still assert the pre-`50e9bd199` behavior (a failing nested tool must not raise an execution error). A local run of 698 tests (29 skipped) had only those 7 failures. The 2.2.4 stack preserves upstream Chat tool-error values, as chosen by the owner on October 11. All tests must pass on the new stack; the seven historical failures are not accepted failures for it. See [the recovery ledger](mcp-apps-recovery.md) for current verification gates.

**Start-up overhead** (a criterion of #3). The `cloud-setup: ok in Ns` line times the script only, not VM provisioning. Compare two sessions: the first in a fresh environment (cold: the setup script runs, then the hook) and a second started after the environment cache exists (warm: the hook only). Compare each session's start-to-first-prompt time at claude.ai/code, and post both numbers and the `cloud-setup` lines as a comment on #3. If the setup script exceeds roughly five minutes, the environment is not cached; drop the slowest step from it and let the hook do that step.

With the Playwright opt-in on, also run `(cd tests/e2e && node -e "require('@playwright/test').chromium.launch().then(b => b.close())")`; it exits 0 when headless Chromium launches.

If `go version` fails because `GOTOOLCHAIN` is `local`, add `GOTOOLCHAIN=auto` as an environment variable. If the setup script does not find the repo, the hook still does the same work on every session; fix the path in the setup script field.

The environment facts above (Trusted domains incl. the Ubuntu apt mirrors, the setup script running as root, the roughly five-minute cache limit and seven-day expiry, SessionStart hooks having a 600 s timeout) are from <https://code.claude.com/docs/en/cloud-environments> as read on 2026-10-05. The working directory of the setup script is not documented, hence the check above.

`bash .claude/cloud/setup.test.sh` checks the script's own logic (no-op outside the cloud, step order, failures never abort, Playwright opt-in) against stubbed tools, with no network.
