#!/usr/bin/env bash
# Fork-only: lets a Claude Code cloud session run the MCP Go suites and `make test-mcp` untouched.
# Runs as the environment's setup script (result is cached) and as a SessionStart hook (fallback when
# the cache is cold, and the only place CLAUDE_ENV_FILE exists). No-op outside the cloud.
# See docs/agents/cloud-sessions.md. Never exits non-zero: a failing setup script would block the session.
[ "${CLAUDE_CODE_REMOTE:-}" = true ] || exit 0
cd "$(dirname "$0")/../.." || exit 0

log=${TMPDIR:-/tmp}/cloud-setup.log
failed=0
step() { echo "+ $*"; "$@" || { echo "FAILED: $*"; failed=$((failed + 1)); }; }

{
  # The module go.mod files pin Go 1.27.x. Go >= 1.21 fetches it through proxy.golang.org.
  export GOTOOLCHAIN=auto
  step go version
  # Warm only what the two suites load. Plain `go mod download` fetches core's whole graph (every provider SDK)
  # and overruns the five-minute setup cache limit. `make test-mcp` runs with GOWORK=off, so mirror that too.
  pushd core >/dev/null
  step go list -deps -test ./mcp/...
  step env GOWORK=off go list -deps -test ./internal/mcptests/...
  popd >/dev/null
  step make install-gotestsum setup-mcp-tests

  # Opt-in: the browser CDN is not on the Trusted network list, see the doc.
  if [ "${BIFROST_CLOUD_PLAYWRIGHT:-}" = 1 ]; then
    step make install-playwright
    pushd tests/e2e >/dev/null
    step node -e "require('@playwright/test').chromium.launch().then(b => b.close())"
    popd >/dev/null
  fi

  # make test-mcp calls gotestsum directly; `go install` drops it in GOPATH/bin, which the VM may not have on PATH.
  [ -z "${CLAUDE_ENV_FILE:-}" ] || echo "export PATH=\"${GOPATH:-$HOME/go}/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
} > "$log" 2>&1

if [ "$failed" = 0 ]; then
  echo "cloud-setup: ok in ${SECONDS}s (log: $log)"
else
  echo "cloud-setup: $failed step(s) FAILED in ${SECONDS}s, see $log"
fi
exit 0
