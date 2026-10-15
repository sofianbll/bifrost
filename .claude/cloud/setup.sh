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

GO_REQUIRED_VERSION=1.27.2
GO_CACHE_ROOT=${XDG_CACHE_HOME:-$HOME/.cache}
GO_INSTALL_DIR="$GO_CACHE_ROOT/bifrost-go/$GO_REQUIRED_VERSION"
BOOTSTRAPPED_GOROOT=

go_version_at_least_auto() {
  local version_output go_major go_minor
  version_output=$("$1" version 2>/dev/null) || return 1
  if [[ "$version_output" =~ ^go\ version\ go([0-9]+)\.([0-9]+)(\.([0-9]+))?([[:space:]]|$) ]]; then
    go_major=${BASH_REMATCH[1]}
    go_minor=${BASH_REMATCH[2]}
    [ "$go_major" -gt 1 ] || { [ "$go_major" -eq 1 ] && [ "$go_minor" -ge 21 ]; }
  else
    return 1
  fi
}

go_is_pinned() {
  local version_output
  version_output=$("$1" version 2>/dev/null) || return 1
  [[ "$version_output" == "go version go$GO_REQUIRED_VERSION "* ]]
}

download_pinned_go() {
  local system machine go_os go_arch archive checksum parent staging
  system=$(uname -s 2>/dev/null) || system=unknown
  machine=$(uname -m 2>/dev/null) || machine=unknown
  case "$system:$machine" in
    Linux:x86_64|Linux:amd64) go_os=linux; go_arch=amd64 ;;
    Linux:aarch64|Linux:arm64) go_os=linux; go_arch=arm64 ;;
    Darwin:x86_64|Darwin:amd64) go_os=darwin; go_arch=amd64 ;;
    Darwin:arm64|Darwin:aarch64) go_os=darwin; go_arch=arm64 ;;
    *) echo "Unsupported Go bootstrap platform: $system/$machine"; return 1 ;;
  esac
  case "$go_os-$go_arch" in
    linux-amd64) checksum=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5 ;;
    linux-arm64) checksum=94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8 ;;
    darwin-amd64) checksum=587b59182488b23aa6e5fc25110405a3e0e5b38ed2f5b2f46ed13c32aee356fe ;;
    darwin-arm64) checksum=76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5 ;;
  esac
  archive="go$GO_REQUIRED_VERSION.$go_os-$go_arch.tar.gz"
  parent="$GO_CACHE_ROOT/bifrost-go"
  mkdir -p "$parent" || { echo "Could not create isolated Go cache"; return 1; }
  if [ -e "$GO_INSTALL_DIR" ] || [ -L "$GO_INSTALL_DIR" ]; then
    if [ -x "$GO_INSTALL_DIR/bin/go" ] && go_is_pinned "$GO_INSTALL_DIR/bin/go"; then
      export GOROOT="$GO_INSTALL_DIR"
      BOOTSTRAPPED_GOROOT="$GO_INSTALL_DIR"
      PATH="$GO_INSTALL_DIR/bin:$PATH"
      export PATH
      return 0
    fi
    echo "Existing isolated Go $GO_REQUIRED_VERSION installation is invalid; remove $GO_INSTALL_DIR to recover"
    return 1
  fi
  staging=$(mktemp -d "$parent/.go$GO_REQUIRED_VERSION.XXXXXX") || { echo "Could not create Go staging directory"; return 1; }
  if ! curl --fail --location --silent --show-error --connect-timeout 20 --max-time 120 \
      --retry 2 --retry-max-time 120 \
      --output "$staging/$archive" "https://dl.google.com/go/$archive"; then
    echo "Go $GO_REQUIRED_VERSION download failed"
    rm -rf "$staging"
    return 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    if ! printf '%s  %s\n' "$checksum" "$staging/$archive" | sha256sum -c - >/dev/null 2>&1; then
      echo "Go $GO_REQUIRED_VERSION checksum verification failed"
      rm -rf "$staging"
      return 1
    fi
  elif command -v shasum >/dev/null 2>&1; then
    if ! printf '%s  %s\n' "$checksum" "$staging/$archive" | shasum -a 256 -c - >/dev/null 2>&1; then
      echo "Go $GO_REQUIRED_VERSION checksum verification failed"
      rm -rf "$staging"
      return 1
    fi
  else
    echo "No SHA-256 utility is available to verify Go $GO_REQUIRED_VERSION"
    rm -rf "$staging"
    return 1
  fi
  if ! tar -xzf "$staging/$archive" -C "$staging" ||
     [ ! -x "$staging/go/bin/go" ] || ! go_is_pinned "$staging/go/bin/go"; then
    echo "Go $GO_REQUIRED_VERSION archive did not contain the pinned compiler"
    rm -rf "$staging"
    return 1
  fi
  if ! mv "$staging/go" "$GO_INSTALL_DIR"; then
    echo "Could not install Go $GO_REQUIRED_VERSION into the isolated cache"
    rm -rf "$staging"
    return 1
  fi
  rm -rf "$staging"
  export GOROOT="$GO_INSTALL_DIR"
  BOOTSTRAPPED_GOROOT="$GO_INSTALL_DIR"
  PATH="$GO_INSTALL_DIR/bin:$PATH"
  export PATH
  echo "Installed verified Go $GO_REQUIRED_VERSION at $GO_INSTALL_DIR"
}

ensure_go() {
  local go_path
  go_path=$(command -v go 2>/dev/null || true)
  if [ -n "$go_path" ] && go_version_at_least_auto "$go_path"; then
    echo "Using Go toolchain on PATH: $go_path"
    return 0
  fi
  if [ -n "$go_path" ]; then
    echo "Go on PATH is missing, invalid, or too old for automatic toolchain selection: $go_path"
  else
    echo "Go compiler is missing from PATH"
  fi
  download_pinned_go
}

{
  # Go 1.21+ supports GOTOOLCHAIN=auto. Install the pinned compiler only when
  # PATH has no real compiler capable of resolving the module's Go 1.27 pin.
  go_ready=0
  if ensure_go; then
    go_ready=1
    export GOTOOLCHAIN=auto
    step go version
    # Warm only what the two suites load. Plain `go mod download` fetches core's whole graph (every provider SDK)
    # and overruns the five-minute setup cache limit. `make test-mcp` runs with GOWORK=off, so mirror that too.
    pushd core >/dev/null
    step go list -deps -test ./mcp/...
    step env GOWORK=off go list -deps -test ./internal/mcptests/...
    popd >/dev/null
    step make install-gotestsum setup-mcp-tests
  else
    echo "FAILED: no verified Go compiler is available; skipped Go-dependent setup"
    failed=$((failed + 1))
  fi

  # Opt-in: the browser CDN is not on the Trusted network list, see the doc.
  if [ "${BIFROST_CLOUD_PLAYWRIGHT:-}" = 1 ]; then
    system_chromium=$(command -v chromium 2>/dev/null || command -v chromium-browser 2>/dev/null || true)
    if [ -n "$system_chromium" ]; then
      pushd tests/e2e >/dev/null
      step npm ci
      step env PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="$system_chromium" node -e \
        "require('@playwright/test').chromium.launch({executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}).then(b => b.close())"
      popd >/dev/null
    else
      step make install-playwright
      pushd tests/e2e >/dev/null
      step node -e "require('@playwright/test').chromium.launch().then(b => b.close())"
      popd >/dev/null
    fi
  fi

  # The agent phase is a separate shell. Codex users will add this bin directory
  # to PATH on the test command (documented and self-checked there), not via export here.
  if [ "$mode" = codex ] && [ "$go_ready" = 1 ]; then
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
    if [ "$go_ready" = 1 ]; then
      if [ -n "$BOOTSTRAPPED_GOROOT" ]; then
        echo "export GOROOT=\"$BOOTSTRAPPED_GOROOT\"" >> "$CLAUDE_ENV_FILE"
        echo "export PATH=\"$BOOTSTRAPPED_GOROOT/bin:${GOPATH:-$HOME/go}/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
      else
        echo "export PATH=\"${GOPATH:-$HOME/go}/bin:\$PATH\"" >> "$CLAUDE_ENV_FILE"
      fi
    fi
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
