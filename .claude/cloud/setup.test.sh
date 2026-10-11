#!/usr/bin/env bash
# Runnable check for setup.sh, no network: `bash .claude/cloud/setup.test.sh`.
# The toolchain is stubbed so only setup.sh's own logic is under test.
set -u
here=$(cd "$(dirname "$0")" && pwd)
t=$(mktemp -d); trap 'rm -rf "$t"' EXIT
for c in go make node; do
  printf '#!/bin/sh\necho "%s $* @ $(basename "$PWD")" >> "$CALLS"\ncase "$*" in *"${FAIL_ON:-%%never%%}"*) exit 1;; esac\n' "$c" > "$t/$c"
  chmod +x "$t/$c"
done

# run [ENV=val ...]: runs setup.sh in a clean env; leaves $t/calls, $t/out; echoes its exit code.
run() { : > "$t/calls"; env -i PATH="$t:/usr/bin:/bin" HOME="$t" TMPDIR="$t" CALLS="$t/calls" "$@" bash "$here/setup.sh" > "$t/out" 2>&1; echo $?; }
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

[ "$fail" = 0 ] && echo "setup.test.sh: ok"
exit "$fail"
