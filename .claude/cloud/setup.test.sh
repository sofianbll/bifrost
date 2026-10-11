#!/usr/bin/env bash
# Runnable check for setup.sh, no network: `bash .claude/cloud/setup.test.sh`.
# The toolchain is stubbed so only setup.sh's own logic is under test.
set -u
here=$(cd "$(dirname "$0")" && pwd)
t=$(mktemp -d); trap 'rm -rf "$t"' EXIT
mkdir -p "$t/home"
for c in go make node npm; do
  printf '#!/bin/sh\necho "%s $* @ $(basename "$PWD")" >> "$CALLS"\ncase "$*" in *"${FAIL_ON:-%%never%%}"*) exit 1;; esac\n' "$c" > "$t/$c"
  if [ "$c" = go ]; then
    cat >> "$t/$c" <<'STUB'
case "$*" in
  version) echo "${STUB_GO_VERSION:-go version go1.27.0 linux/amd64}"; exit 0 ;;
  "env GOBIN") printf '%s\n' "${GOBIN:-${GOENV_GOBIN:-}}"; exit 0 ;;
  "env GOPATH") printf '%s\n' "${GOPATH:-$HOME/go}"; exit 0 ;;
esac
STUB
  fi
  if [ "$c" = make ]; then
    cat >> "$t/$c" <<'STUB'
case "$*" in
  *install-gotestsum*) bin="${GOBIN:-${GOENV_GOBIN:-${GOPATH:-$HOME/go}/bin}}"; mkdir -p "$bin"; : > "$bin/gotestsum"; chmod +x "$bin/gotestsum" ;;
esac
STUB
  fi
  chmod +x "$t/$c"
done
# Isolated tools let tests simulate missing Go without using the host's /usr/bin/go.
mkdir -p "$t/tools"
for c in dirname basename mkdir mktemp rm mv chmod env cat; do ln -s "$(command -v "$c")" "$t/tools/$c"; done
cat > "$t/tools/test" <<'STUB'
#!/bin/sh
[ "$1" = -x ] && [ -x "$2" ]
STUB
ln -s "$t/make" "$t/tools/make"
ln -s "$t/node" "$t/tools/node"
ln -s "$t/npm" "$t/tools/npm"
cat > "$t/tools/uname" <<'STUB'
#!/bin/sh
case "$1" in
  -s) echo "${TEST_UNAME_OS:-Linux}" ;;
  -m) echo "${TEST_UNAME_ARCH:-x86_64}" ;;
  *) exit 1 ;;
esac
STUB
cat > "$t/tools/curl" <<'STUB'
#!/bin/sh
echo "curl $*" >> "$CALLS"
[ "${FAIL_GO_DOWNLOAD:-0}" = 0 ] || exit 1
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ] || [ "$1" = --output ]; then shift; : > "$1"; exit 0; fi
  shift
done
exit 1
STUB
cat > "$t/tools/sha256sum" <<'STUB'
#!/bin/sh
echo "sha256sum $*" >> "$CALLS"
[ "$1" = -c ] || exit 2
IFS= read -r line
case "$line" in "${EXPECTED_GO_SHA256}  "*)
  [ "${BAD_GO_CHECKSUM:-0}" = 0 ] || exit 1
  echo "${line#*  }: OK"
  ;;
  *) exit 1 ;;
esac
STUB
cat > "$t/tools/tar" <<'STUB'
#!/bin/sh
echo "tar $*" >> "$CALLS"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -C ]; then shift; dest=$1; fi
  shift
done
mkdir -p "$dest/go/bin"
cat > "$dest/go/bin/go" <<'GO'
#!/bin/sh
echo "go $* @ $(basename "$PWD")" >> "$CALLS"
case "$*" in
  version) echo 'go version go1.27.0 linux/amd64'; exit 0 ;;
  "env GOBIN") printf '%s\n' "${GOBIN:-}"; exit 0 ;;
  "env GOPATH") printf '%s\n' "${GOPATH:-$HOME/go}"; exit 0 ;;
esac
exit 0
GO
chmod +x "$dest/go/bin/go"
STUB
chmod +x "$t/tools/test" "$t/tools/uname" "$t/tools/curl" "$t/tools/sha256sum" "$t/tools/tar"

# run [ENV=val ...]: runs setup.sh in a clean env; leaves $t/calls, $t/out; echoes its exit code.
run() { : > "$t/calls"; env -i PATH="$t/tools:$t" HOME="$t/home" TMPDIR="$t" CALLS="$t/calls" "$@" "$(command -v bash)" "$here/setup.sh" > "$t/out" 2>&1; echo $?; }
run_codex() {
  : > "$t/calls"
  local run_path="$t/tools:$t" assignment stub_go_version= bad_checksum=0 fail_download=0 test_uname_os=Linux test_uname_arch=x86_64 fail_on= goenv_gobin= claude_env_file= playwright= xdg_cache_home=
  for assignment in "$@"; do
    case "$assignment" in
      RUN_PATH=*) run_path=${assignment#RUN_PATH=} ;;
      STUB_GO_VERSION=*) stub_go_version=${assignment#STUB_GO_VERSION=} ;;
      BAD_GO_CHECKSUM=*) bad_checksum=${assignment#BAD_GO_CHECKSUM=} ;;
      FAIL_GO_DOWNLOAD=*) fail_download=${assignment#FAIL_GO_DOWNLOAD=} ;;
      TEST_UNAME_OS=*) test_uname_os=${assignment#TEST_UNAME_OS=} ;;
      TEST_UNAME_ARCH=*) test_uname_arch=${assignment#TEST_UNAME_ARCH=} ;;
      FAIL_ON=*) fail_on=${assignment#FAIL_ON=} ;;
      GOENV_GOBIN=*) goenv_gobin=${assignment#GOENV_GOBIN=} ;;
      CLAUDE_ENV_FILE=*) claude_env_file=${assignment#CLAUDE_ENV_FILE=} ;;
      BIFROST_CLOUD_PLAYWRIGHT=*) playwright=${assignment#BIFROST_CLOUD_PLAYWRIGHT=} ;;
      XDG_CACHE_HOME=*) xdg_cache_home=${assignment#XDG_CACHE_HOME=} ;;
    esac
  done
  env -i PATH="$run_path" HOME="$t/home" XDG_CACHE_HOME="$xdg_cache_home" TMPDIR="$t" CALLS="$t/calls" \
    EXPECTED_GO_SHA256=675c26c449cbb18fc24b74650de1eabbae6e16f64326fd85a283fb3b58280685 \
    BAD_GO_CHECKSUM="$bad_checksum" FAIL_GO_DOWNLOAD="$fail_download" STUB_GO_VERSION="$stub_go_version" TEST_UNAME_OS="$test_uname_os" TEST_UNAME_ARCH="$test_uname_arch" \
    FAIL_ON="$fail_on" GOENV_GOBIN="$goenv_gobin" CLAUDE_ENV_FILE="$claude_env_file" BIFROST_CLOUD_PLAYWRIGHT="$playwright" \
    "$(command -v bash)" "$here/setup.sh" --codex > "$t/out" 2>&1
  echo $?
}
fail=0
check() { if ! eval "$2"; then echo "FAIL: $1"; fail=1; fi; }

rc=$(run)
check "no-op outside the cloud"        '[ "$rc" = 0 ] && [ ! -s "$t/calls" ] && [ ! -s "$t/out" ]'

rc=$(run CLAUDE_CODE_REMOTE=true)
check "cloud: exits 0"                 '[ "$rc" = 0 ]'
check "cloud: resolves go toolchain"   'grep -q "^go version" "$t/calls"'
check "cloud: warms core/mcp modules"  'grep -q "^go list -deps -test ./mcp/... @ core$" "$t/calls"'
check "cloud: warms mcptests modules"  'grep -q "^go list -deps -test ./internal/mcptests/... @ core$" "$t/calls"'
check "cloud: never go mod download"   '! grep -q "mod download" "$t/calls"'
check "cloud: installs MCP test deps"  'grep -q "^make install-gotestsum setup-mcp-tests" "$t/calls"'
check "cloud: playwright is opt-in"    '! grep -q playwright "$t/calls"'
check "cloud: reports success"         'grep -q "cloud-setup: ok" "$t/out"'

rc=$(run CLAUDE_CODE_REMOTE=true CLAUDE_ENV_FILE="$t/env")
check "hook: puts gotestsum on PATH"   '[ "$rc" = 0 ] && grep -q "^export PATH=.*/go/bin" "$t/env"'

rc=$(run CLAUDE_CODE_REMOTE=true FAIL_ON=setup-mcp-tests)
check "step failure never aborts"      '[ "$rc" = 0 ] && grep -q FAILED "$t/out"'

rc=$(run CLAUDE_CODE_REMOTE=true BIFROST_CLOUD_PLAYWRIGHT=1)
check "playwright: installs chromium"  'grep -q "^make install-playwright" "$t/calls"'
check "playwright: launches chromium"  'grep -q "^node -e .*chromium.launch().* @ e2e$" "$t/calls"'

printf '#!/bin/sh\n' > "$t/chromium"; chmod +x "$t/chromium"
rc=$(run CLAUDE_CODE_REMOTE=true BIFROST_CLOUD_PLAYWRIGHT=1)
check "playwright: reuses system chromium after npm ci" 'grep -q "^npm ci @ e2e$" "$t/calls" && ! grep -q "^make install-playwright" "$t/calls"'
check "playwright: passes system browser path explicitly" 'grep -q "^node -e .*executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH" "$t/calls" && grep -q "^+ env PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=.*/chromium node -e .*executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH" "$t/cloud-setup.log"'

rc=$(run_codex)
check "Codex: runs without Claude remote variables" '[ "$rc" = 0 ] && grep -q "^go version" "$t/calls"'
check "Codex: valid Go on PATH does not reinstall" '! grep -q "^curl " "$t/calls"'
check "Codex: verifies gotestsum is installed for the agent phase" 'grep -q "^+ test -x .*/go/bin/gotestsum$" "$t/cloud-setup-codex.log"'
check "Codex: reports platform and elapsed time" 'grep -q "cloud-setup (codex): ok on .* in .*s" "$t/out"'

rc=$(run_codex GOENV_GOBIN="$t/goenv-bin")
check "Codex: resolves configured go env GOBIN" '[ "$rc" = 0 ] && grep -q "^+ test -x .*/goenv-bin/gotestsum$" "$t/cloud-setup-codex.log"'

rc=$(run_codex CLAUDE_ENV_FILE="$t/codex-env")
check "Codex: does not use Claude environment hooks" '[ "$rc" = 0 ] && [ ! -e "$t/codex-env" ]'

printf '#!/bin/sh\n' > "$t/gotestsum"; chmod +x "$t/gotestsum"
rc=$(run_codex)
check "Codex: accepts gotestsum already on standard PATH" '[ "$rc" = 0 ] && grep -q "^+ gotestsum available on PATH: .*/gotestsum$" "$t/cloud-setup-codex.log"'
rm -f "$t/gotestsum"

rc=$(run_codex FAIL_ON=setup-mcp-tests)
check "Codex: setup failure is nonzero" '[ "$rc" != 0 ] && grep -q "cloud-setup (codex): .* FAILED on .* in .*s" "$t/out"'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools")
check "Codex: missing Go installs the pinned verified toolchain" '[ "$rc" = 0 ] && grep -q "curl .*go1.27.0.linux-amd64.tar.gz" "$t/calls" && grep -q "sha256sum -c -" "$t/calls" && grep -q "go list -deps -test ./mcp/... @ core" "$t/calls"'
check "Codex: Go archive download has bounded connect, transfer and retry times" 'grep -q -- "--connect-timeout 20 --max-time 120 --retry 2 --retry-max-time 120" "$t/calls"'
check "Codex: installed toolchain stays under isolated cache" '[ -x "$t/home/.cache/bifrost-go/1.27.0/bin/go" ]'

rc=$(run_codex RUN_PATH="$t/tools")
check "Codex: cached Go toolchain is reused without another download" '[ "$rc" = 0 ] && ! grep -q "^curl " "$t/calls" && grep -q "go list -deps -test ./mcp/... @ core" "$t/calls"'

rm -rf "$t/home/.cache" "$t/custom-cache"
rc=$(run_codex RUN_PATH="$t/tools" XDG_CACHE_HOME="$t/custom-cache")
check "Codex: honors XDG_CACHE_HOME for isolated Go install" '[ "$rc" = 0 ] && [ -x "$t/custom-cache/bifrost-go/1.27.0/bin/go" ] && [ ! -e "$t/home/.cache/bifrost-go/1.27.0/bin/go" ]'

rc=$(run_codex RUN_PATH="$t/tools" XDG_CACHE_HOME="$t/custom-cache")
check "Codex: reuses Go from custom XDG cache" '[ "$rc" = 0 ] && ! grep -q "^curl " "$t/calls" && grep -q "go list -deps -test ./mcp/... @ core" "$t/calls"'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools:$t" STUB_GO_VERSION='not the Go compiler')
check "Codex: wrong go program is replaced before Go-dependent steps" '[ "$rc" = 0 ] && grep -q "curl .*go1.27.0.linux-amd64.tar.gz" "$t/calls" && grep -q "go list -deps -test ./mcp/... @ core" "$t/calls"'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools:$t" STUB_GO_VERSION='go version go1.20.14 linux/amd64')
check "Codex: Go too old for automatic toolchains is replaced" '[ "$rc" = 0 ] && grep -q "curl .*go1.27.0.linux-amd64.tar.gz" "$t/calls" && grep -q "go list -deps -test ./mcp/... @ core" "$t/calls"'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools:$t" STUB_GO_VERSION='not the Go compiler' BAD_GO_CHECKSUM=1)
check "Codex: checksum failure is nonzero and never compiles with bad Go" '[ "$rc" != 0 ] && grep -q "checksum" "$t/cloud-setup-codex.log" && ! grep -q "go list -deps" "$t/calls" && [ ! -x "$t/home/.cache/bifrost-go/1.27.0/bin/go" ]'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools" FAIL_GO_DOWNLOAD=1)
check "Codex: download failure is nonzero and skips Go-dependent steps" '[ "$rc" != 0 ] && grep -q "download failed" "$t/cloud-setup-codex.log" && ! grep -q "go list -deps" "$t/calls"'

rm -rf "$t/home/.cache"
rc=$(run_codex RUN_PATH="$t/tools" TEST_UNAME_OS='Plan9' TEST_UNAME_ARCH='i386')
check "Codex: unsupported platform fails closed" '[ "$rc" != 0 ] && ! grep -q "curl " "$t/calls" && ! grep -q "go list -deps" "$t/calls"'

rc=$(run_codex BIFROST_CLOUD_PLAYWRIGHT=1 FAIL_ON=chromium.launch)
check "Codex: system Chromium launch failure fails setup" '[ "$rc" != 0 ] && grep -q "FAILED: env PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=.* node -e" "$t/cloud-setup-codex.log"'

[ "$fail" = 0 ] && echo "setup.test.sh: ok"
exit "$fail"
