# Cloud coding sessions

Fork-only: this belongs to the fork plumbing (branch d of the stack in #4) and never goes into an upstream PR.

A Claude Code cloud session on `sofianbll/bifrost` can run the core MCP Go tests and `make test-mcp` with no manual setup. One script, `.claude/cloud/setup.sh`, does the work. It does nothing unless `CLAUDE_CODE_REMOTE=true`, so local sessions are untouched, and it never exits non-zero. It runs from two places:

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
