#!/usr/bin/env bash
# Fork-only: prepares dependencies for MCP tests in Claude or Codex cloud.
# Claude mode remains a best-effort SessionStart hook; --codex is for the Codex Cloud
# install script and returns a failure status when dependencies are incomplete.
# See docs/agents/cloud-sessions.md.
mode=claude
if [ "${1:-}" = --codex ]; then
  mode=codex
elif [ "$#" -gt 0 ]; then
  echo "Usage: $0 [--codex]" >&2
  exit 2
fi

if [ "$mode" = claude ] && [ "${CLAUDE_CODE_REMOTE:-}" != true ]; then exit 0; fi
if ! cd "$(dirname "$0")/../.."; then
  [ "$mode" = codex ] && exit 1 || exit 0
fi

if [ "$mode" = codex ]; then
  log=${TMPDIR:-/tmp}/cloud-setup-codex.log
else
  log=${TMPDIR:-/tmp}/cloud-setup.log
fi
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

  # The agent phase is a separate shell. Codex users will add this bin directory
  # to PATH on the test command (documented and self-checked there), not via export here.
  if [ "$mode" = codex ]; then
    if command -v gotestsum >/dev/null 2>&1; then
      echo "+ gotestsum available on PATH: $(command -v gotestsum)"
    else
      go_bin=${GOBIN:-$(go env GOBIN)}
      [ -n "$go_bin" ] || go_bin="$(go env GOPATH)/bin"
      step test -x "$go_bin/gotestsum"
    fi
  fi

  # CLAUDE_ENV_FILE exists only in the SessionStart hook and persists PATH there.
  if [ "$mode" = claude ] && [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    echo "export PATH=\"${GOPATH:-$HOME/go}/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
  fi
} > "$log" 2>&1

platform=$(uname -s 2>/dev/null || echo unknown)
if [ "$failed" = 0 ]; then
  if [ "$mode" = codex ]; then
    echo "cloud-setup (codex): ok on $platform in ${SECONDS}s (log: $log)"
  else
    echo "cloud-setup: ok in ${SECONDS}s (log: $log)"
  fi
else
  if [ "$mode" = codex ]; then
    echo "cloud-setup (codex): $failed step(s) FAILED on $platform in ${SECONDS}s, see $log"
  else
    echo "cloud-setup: $failed step(s) FAILED in ${SECONDS}s, see $log"
  fi
fi
[ "$mode" = codex ] && exit "$failed"
exit 0
