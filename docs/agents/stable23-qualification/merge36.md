# Stable #36 integration merge

The approved #36 worker `codex/stable23-36` at `54630233a79dabdb681facb0de97d10227bafe83` was integrated into `codex/stable23-integration` with one squash commit. Worker history and worktree were preserved.

- Integration commit: `e949886eb5212db2dc0ac7857e9934527ecb5044`
- Subject: `[fix]: port approved MCP fixes and native Apps support`
- Parent: `9237ec06672406e437b3cb33fc793ffd0403b3ae` (exact stable base)
- Integration tree: `95c8016664ef8a7788e6ea6833cf7e7f9eda90ac`
- Worker final tree: `95c8016664ef8a7788e6ea6833cf7e7f9eda90ac` (identical)
- Commit includes `Signed-off-by: Sofian <sofianbll.x@gmail.com>`.
- Both tracked worktrees were clean after commit. `git show --check` passed. No workflow or fork-driver paths are in the change. No build or test was run as part of this bounded merger task.

The commit contains 42 files (3,610 insertions, 129 deletions), limited to the approved A/B/C source, focused tests/fixtures, docs and changelogs:

```text
core/bifrost.go
core/changelog.md
core/internal/mcptests/codemode_security_test.go
core/mcp/clientmanager_test.go
core/mcp/codemode/starlark/executecode.go
core/mcp/codemode/starlark/toolerrormarker_test.go
core/mcp/exec.go
core/mcp/mcp.go
core/mcp/pluginpipeline.go
core/mcp/toolcall_recovery_test.go
core/mcp/toolmanager.go
core/mcp/toolmanager_test.go
core/mcp/toolshash_test.go
core/mcp/utils.go
core/mcp/utils_test.go
core/schemas/bifrost.go
core/schemas/chatcompletions.go
docs/mcp/code-mode.mdx
docs/mcp/overview.mdx
docs/mcp/virtual-mcps.mdx
examples/mcps/error-test-server/main.go
examples/mcps/error-test-server/main_test.go
examples/mcps/mcp-apps-test-server/go.mod
examples/mcps/mcp-apps-test-server/go.sum
examples/mcps/mcp-apps-test-server/main.go
examples/mcps/mcp-apps-test-server/main_test.go
framework/changelog.md
framework/configstore/rdb.go
framework/configstore/rdb_mcp_tools_test.go
framework/configstore/tables/mcp.go
tests/e2e/api/collections/provider-harness.json
tests/e2e/api/provider_config/mcp-apps-native-standalone/config.json
tests/integrations/python/config.json
tests/mcpapps/fixture-app.ts
transports/bifrost-http/handlers/mcpserver.go
transports/bifrost-http/handlers/mcpserver_auth_test.go
transports/bifrost-http/handlers/mcpserver_test.go
transports/bifrost-http/lib/config.go
transports/bifrost-http/lib/config_test.go
transports/bifrost-http/server/server.go
transports/bifrost-http/server/server_test.go
transports/changelog.md
```
