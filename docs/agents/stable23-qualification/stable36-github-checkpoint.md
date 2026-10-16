## Stable candidate source and Cloud demonstration checkpoint

Product candidate **`e949886eb5212db2dc0ac7857e9934527ecb5044`** is published on `codex/stable23-integration`, with its **only parent** exact stable `9237ec06672406e437b3cb33fc793ffd0403b3ae`. The base branch `codex/stable23-base` points exactly to stable. Its tree `95c8016664ef8a7788e6ea6833cf7e7f9eda90ac` equals the tested worker `54630233a79dabdb681facb0de97d10227bafe83`; original TDD history remains published on `codex/stable23-36`, with no rewrite. [Complete stable product diff](https://github.com/sofianbll/bifrost/compare/9237ec06672406e437b3cb33fc793ffd0403b3ae...e949886eb5212db2dc0ac7857e9934527ecb5044).

Only the approved A/#28, B/#29 and C/#30 product/test/docs/changelog hunks were ported. No later-dev parent/history, workflow or fork-only driver entered this product diff. Stable keeps `github.com/maximhq/bifrost/transports` without `/v2`; the fixture language directive aligns to `go 1.27.0`, with no dependency bump. Overlapping handler/config/collection hunks were reconciled; C resource-hook tests were placed in an existing test file per AGENTS. Later-dev-only OpenAPI implementation was not imported. Existing stable authorization and source-authority rules remain the contract.

### Newly executed Cloud evidence

The wire/lifecycle candidate proof below executed clean worker **54630233**, whose tree is identical to the published integration candidate. It is not yet an exact-integration Actions/image/host qualification. Stock is clean9237; driver fixtures are clean968d934ed4489f3fa0bb86fdae9197406c05b5fb. Independent binary inspection confirms all14 linked Bifrost modules local `(devel)` and Go1.27.2. Native binary SHA256: stock `f58728f39a7d98f7a82b29563b40a9926ca7bc8a807e9ed0abc834bc3578c9fb`, worker `5f0dcf84a3ebf206cba91f97325fefbb179f89e2b746db8f581fdfc3ddd797c1`.

| Native matched scope | Stock failures / assertions | Candidate failures / assertions | Requests each | Skips each |
| --- | ---: | ---: | ---: | ---: |
| MCP Apps | 34 /148 | 0 /148 | 39 | 0 |
| Code Mode | 2 /10 | 0 /10 | 3 | 0 |

All13 named red sentinels and7 green controls were independently verified on stock. The original native launch omitted per-client callback hints needed for source-identifying resource HTML and produced one candidate assertion failure. Both sources were rerun with identical explicit hints matching the Actions host fixture setup; no product code or assertion changed. The exact stock failed-pair sets remain unchanged. The refined observation manifest is SHA256 **`33ee42eeeafc583a2db9ea9a51a839c0807b773dd891ae6201d51477c7d98369`**, superseding the prior manifest's provenance while retaining its exact expectations. Both attempts are retained.

The focused probe recorded24 requests /21 explicit assertions, all21 green on candidate (stock13 green/8 red). It is not standalone permission proof; the full Newman selection above contains the complete assertions. A separate disposable same-file firstboot/restart used the same binary and app store: candidate HTTP200 with exactly one `vk-firstboot-grants-fixture` both times; stock HTTP403/zero on firstboot, then HTTP200/exactly one on restart. No setup API seeded that tested grant.

Focused MCP, HTTP handler, config-store and Code Mode guardrail race selections pass. The broad unchanged-stock MCP suite records373 terminal passes, one failure `TestBuildTLSHTTPClientUsesGlobalProxy`, zero skips; the candidate broad run reproduced that unchanged test's failure and it passed isolated. Broader internalmcptests also encountered missing Node/remote fixture prerequisites; its output was truncated, so no exhaustive/global pass is claimed. Complete JSON inventory and inherited-check separation belong in #37.

The error/value and grant fixes retain executed behavioral red/green tests in the preserved worker history. Cost verification is being completed on the public HTTP handler, ordinary MCP catalog/execution and Code Mode paths before the draft PR opens; the earlier dispatcher benchmark is not full HTTP proof. Cloud reports, source mapping and commands will be committed separately with the fork-only driver and retained in the final Actions artifact.

**#36 publication is still in progress; #37 has not started implementation.** The draft PR, exact-source Actions, actual pinned reference host and independent reviews remain pending. No merge, upstream write, provider call/spend, production/Pulsar change, deployment, release publication or Mac execution occurred.

<details><summary>Complete 42-file source-diff mapping (source PR membership; overlapping hunks reconciled)</summary>

| Candidate path | Inspected source PRs |
| --- | --- |
| `core/bifrost.go` | #30 |
| `core/changelog.md` | #28, #30 |
| `core/internal/mcptests/codemode_security_test.go` | #28 |
| `core/mcp/clientmanager_test.go` | #30 |
| `core/mcp/codemode/starlark/executecode.go` | #28 |
| `core/mcp/codemode/starlark/toolerrormarker_test.go` | #28 |
| `core/mcp/exec.go` | #30 |
| `core/mcp/mcp.go` | #30 |
| `core/mcp/pluginpipeline.go` | #30 |
| `core/mcp/toolcall_recovery_test.go` | #30 |
| `core/mcp/toolmanager.go` | #30 |
| `core/mcp/toolmanager_test.go` | #30 |
| `core/mcp/toolshash_test.go` | #30 |
| `core/mcp/utils.go` | #30 |
| `core/mcp/utils_test.go` | #30 |
| `core/schemas/bifrost.go` | #30 |
| `core/schemas/chatcompletions.go` | #30 |
| `docs/mcp/code-mode.mdx` | #28 |
| `docs/mcp/overview.mdx` | #30 |
| `docs/mcp/virtual-mcps.mdx` | #29 |
| `examples/mcps/error-test-server/main.go` | #28 |
| `examples/mcps/error-test-server/main_test.go` | #28 |
| `examples/mcps/mcp-apps-test-server/go.mod` | #30 |
| `examples/mcps/mcp-apps-test-server/go.sum` | #30 |
| `examples/mcps/mcp-apps-test-server/main.go` | #30 |
| `examples/mcps/mcp-apps-test-server/main_test.go` | #30 |
| `framework/changelog.md` | #30 |
| `framework/configstore/rdb.go` | #30 |
| `framework/configstore/rdb_mcp_tools_test.go` | #30 |
| `framework/configstore/tables/mcp.go` | #30 |
| `tests/e2e/api/collections/provider-harness.json` | #28, #29, #30 |
| `tests/e2e/api/provider_config/mcp-apps-native-standalone/config.json` | #30 |
| `tests/integrations/python/config.json` | #28, #29 |
| `tests/mcpapps/fixture-app.ts` | #30 |
| `transports/bifrost-http/handlers/mcpserver.go` | #28, #30 |
| `transports/bifrost-http/handlers/mcpserver_auth_test.go` | #30 |
| `transports/bifrost-http/handlers/mcpserver_test.go` | #28, #30 |
| `transports/bifrost-http/lib/config.go` | #29 |
| `transports/bifrost-http/lib/config_test.go` | #29 |
| `transports/bifrost-http/server/server.go` | #30 |
| `transports/bifrost-http/server/server_test.go` | #30 |
| `transports/changelog.md` | #28, #29, #30 |

</details>
