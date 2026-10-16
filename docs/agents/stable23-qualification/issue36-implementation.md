# Issue #36 stable implementation and Cloud evidence

## Source and scope

The implementation branch is `/tmp/stable23-36`, `codex/stable23-36`. It is based on unchanged stable `9237ec06672406e437b3cb33fc793ffd0403b3ae`; its current implementation commit is `54630233a79dabdb681facb0de97d10227bafe83` (`[fix]: add native MCP Apps gateway support`, signed off). The tree is clean. The approved A/B/C source mapping and excluded later-dev-only behavior are detailed in `source-audit.md`; only accepted semantic hunks were ported from the preserved complete PR patches.

- **A / PR #28:** `core/mcp/codemode/starlark/executecode.go` now treats only MCP `CallToolResult.IsError` as a nested protocol error, preserving textual `Error:` values as values. `transports/bifrost-http/handlers/mcpserver.go` carries Code Mode's outer `IsError` to `tools/call`. The deterministic error fixture, focused security/boundary tests, and Code Mode docs are included.
- **B / PR #29:** `transports/bifrost-http/lib/config.go` now reconciles file-declared Virtual MCP grants after governance virtual keys load, retaining source-authority and pruning behavior. The first-boot regression test, Python integration config, and Virtual MCP docs are included.
- **C / PR #30:** Stable core/schema MCP App metadata is bounded, hashed, preserved across refresh and config-store reload, and kept out of model catalogs while remaining available to native MCP. Core hooks provide source-qualified app resource access. The stable HTTP MCP handler negotiates the extension and returns native tool/resource results with existing auth/filter controls; the stable server seam, synthetic MCP Apps fixture, contract collection, docs, and changelogs are included.

The candidate keeps stable unsuffixed Go imports (no `/v2`), stable APIs and auth/filter hooks, and the existing local-module workspace. The new fixture module says `go 1.27.0`; no existing dependencies or module requirements were changed. All local Bifrost workspace modules were present in the candidate build metadata. Detailed stable source constraints and intentionally omitted dev-only OpenAPI behavior remain in `source-audit.md` and `orchestrator.md`.

## Baseline characterization

Before product edits, unchanged stable was built from `9237ec06672406e437b3cb33fc793ffd0403b3ae` with Go 1.27.2 and canonical full local `go.work` using:

```sh
GOWORK="$PWD/go.work" go build -trimpath -o /tmp/stable23-36-stock-gateway-fullws ./transports/bifrost-http
```

Stock binary SHA-256: `f58728f39a7d98f7a82b29563b40a9926ca7bc8a807e9ed0abc834bc3578c9fb`. Build info and source/module verification are in `stock-fullworkspace-go-version-m.txt`, `stock-canonical-native-binaries.sha256`, and `stable-stock-characterization.md`.

Native, credential-free probes used the pinned immutable driver `968d934ed4489f3fa0bb86fdae9197406c05b5fb`; providers were `{}`, catalogs were local files, and the gateway received only the recorded allowlisted environment. Actual stable wire failures included missing negotiated MCP Apps capability, app catalog/resource relay, aggregate callback, Code Mode prefix-value/outer-marker behavior, and first-boot grant attachment. The same probes retained ordinary/no-extension, legacy/modern metadata, second-source, nested validation-value, and restart controls. Reports and responses: `stock-canonical-native-probes/native-characterization.json`, `stock-canonical-native-probes/firstboot-restart.json`, `stock-canonical-native-probes.stdout.log`, and `stable-stock-characterization.md`.

The accepted pinned Newman comparison uses the same driver `968d934ed4489f3fa0bb86fdae9197406c05b5fb`, collection source, and explicit callback-prefix fixture hints on both builds. Stable made 39 Apps requests / 148 assertions with 34 failures and zero skips, plus 3 Code Mode requests / 10 assertions with 2 failures and zero skips. Candidate `54630233` made the same 39/148 and 3/10 with zero failures and zero skips. Stock pair classification was independently rechecked: the 13 approved red sentinels fail and 7 controls pass. Raw matched reports are in `cloud-stock-matrix-source-hints/` and `cloud-candidate-matrix-source-hints-54630233/`; the exact shared contract URI input is `stable-stock-contract-source-hints.json` (SHA-256 `33ee42eeeafc583a2db9ea9a51a839c0807b773dd891ae6201d51477c7d98369`). Both observations record `{}` providers and their allowlisted environment. The first candidate orchestration attempt had one aggregate second-resource assertion fail because its locally generated fixture HTML omitted the third callback-prefix hint used by the Actions host config. Root corrected the native orchestration only, did not change product/driver/collection, and reran both sources with identical hints; the rerun above is the accepted result.

## Red/green and focused checks

Behavioral reds were captured before their corresponding fixes: Code Mode nested `Error:` classification (`test-a-core-red.log`), outer MCP `isError` loss (`test-a-handler-red.log`), and first-boot grant absent (`test-b-config-red-fast.log`). Corresponding green logs are `test-a-core-green.log`, `test-a-handler-green.log`, and `test-b-config-green.log`. The stock native wire probes above separately establish the actual C protocol reds. No unit-suite result is being represented as a red unless its test was executed and failed.

C-focused green logs include metadata conversion (`test-c-metadata-green.log`), app-only catalog/native result (`test-c-catalog-native-green.log`), cache persistence (`test-c-cache-store-green.log`), full refresh/reload lifecycle (`test-c-cache-lifecycle.log`), resource hook/denial behavior (`test-c-resource-hooks.log`), handler Apps/resource/auth/filter cases (`test-c-handler-apps.log`), stale native-result recovery (`test-c-native-recovery.log`), HTTP server seam (`test-c-server-seams.log`), and fixture compilation/tests (`test-c-mcp-app-fixture.log`). Fixture test used `GOWORK=off go test . -count=1` from `examples/mcps/mcp-apps-test-server`.

The final nonempty race selections passed:

```sh
go test -race ./core/mcp -run '^(TestConvertMCPToolToBifrostSchema_PreservesMCPAppDefinition|TestConvertMCPToolToBifrostSchemaBoundsRetainedMCPAppMetadata|TestReadMCPAppResourceUsesMCPHooksAndHidesDenials|TestExecuteMCPAppToolPreservesNativeFieldsAndDropsStaleHookOutput|TestGetAvailableToolsKeepsAppOnlyToolsOutOfModelCatalog|TestGetAvailableToolsForMCPIncludesAppsFromCodeModeClients)$' -count=1
```

```sh
go test -race ./transports/bifrost-http/handlers -run '^(TestInitializeNegotiatesMCPAppsExtensionOnlyWhenRequested|TestMCPApps.*|TestCallToolPreservesNativeMCPAppResultAndUsesEffectiveFallback|TestCallToolRefusesHiddenToolWithoutDisclosingItsName|TestToolsListPreservesAndQualifiesMCPAppMetadata|TestMCPAppsPreservesSupportedOutputSchema|TestResourcesListAndReadAreSourceQualified|TestCallToolPreservesCodeModeErrorMarker)$' -count=1
```

```sh
go test -race ./framework/configstore -run '^TestMCPAppToolMetadataSurvivesRefreshAndManagerReload$' -count=1
go test -race ./core/internal/mcptests -run '^TestExecuteToolCode_GuardrailedResultNeverReachesModel$' -count=1
```

These runs are recorded in `test36-core-mcp-focused-race.log`, `test36-handler-focused-race.log`, `test36-configstore-race.log`, and `test36-codemode-guardrail-race.log`. The exact matched names were nonempty. The handler suite covers extension negotiation, resource qualification, hidden-tool denial, output schema, native result/fallback, and Code Mode marker; the Code Mode guardrail proves blocked results do not reach the model.

A broader command was attempted once:

```sh
go test ./core/mcp ./core/schemas ./core/internal/mcptests -count=1
```

`core/schemas` passed. `core/mcp` hit the unchanged `TestBuildTLSHTTPClientUsesGlobalProxy` failure under package load (proxy saw no request). That exact test passed isolated on the candidate with `go test ./core/mcp -run '^TestBuildTLSHTTPClientUsesGlobalProxy$' -count=1` (`test36-proxy-candidate-isolated.log`); the proxy test and builder are not changed in this port. `core/internal/mcptests` took 240 seconds and failed unrelated Cloud prerequisites such as absent `test-tools-server/dist/index.js` and `remote-test-server` binaries and unstable stdio fixtures. The command output was truncated by the execution tool; `test36-core-mcp-full.log` preserves the core-package failure. Root then ran the same ordinary broad MCP test on unchanged stable `9237ec06672406e437b3cb33fc793ffd0403b3ae`: 373 test/subtest terminal passes, zero skips, and the same one failure in `TestBuildTLSHTTPClientUsesGlobalProxy` after 50.173 seconds. Full stock JSON output is `stable-stock-mcp-broad.jsonl`. This independently reproduces the order-sensitive/global-proxy failure on stock; the candidate isolated proxy test passed. This is not a full-suite qualification. Issue #37 owns the source-pinned Actions matrix and host run.

## Candidate binary, native probes, benchmark

Candidate build command:

```sh
GOWORK="$PWD/go.work" go build -trimpath -o /tmp/stable23-36-candidate-gateway-54630233 ./transports/bifrost-http
```

Candidate binary SHA-256: `5f0dcf84a3ebf206cba91f97325fefbb179f89e2b746db8f581fdfc3ddd797c1`. Go metadata reports Go 1.27.2, candidate VCS revision `54630233a79dabdb681facb0de97d10227bafe83`, `vcs.modified=false`, and all included local Bifrost modules as `(devel)`. Metadata and build log are `candidate-fullworkspace-go-version-m.txt` and `candidate-build.log`.

The direct native candidate characterization made 24 HTTP requests with 21 explicit named assertions, all 21 passing. Named assertions cover negotiated/no-extension controls, direct legacy/modern/Code Mode discovery, App callbacks, and Code Mode nested prefix, outer error, and validation controls. Resource and limited-key requests were recorded but did not all have explicit checks; this probe is not standalone permission proof, and the full Newman selection supplies its own assertions. First-boot and restart each used the same candidate binary and app store; both found exactly `vk-firstboot-grants-fixture` and returned HTTP 200 from the file-declared callback. Reports are `candidate-native-probes-54630233/native-characterization.json`, `firstboot-restart.json`, and `candidate-native-probes-54630233.stdout.log`. Providers remained `{}` and the native allowlist is recorded in the reports.

The earlier `BenchmarkMCPToolCallResponse` sample uses `h.server().HandleMessage` and bypasses public `handleMCPServer` admission, alias resolution, and catalog work. It is a dispatch/serialization microbenchmark only, not public request-path cost. Its identical body hash is `1e5e3da80e5930f18bf0d8b4527f19c28f641eb427072df2272bef0de4e21f62` (1,149 bytes), baseline test-only commit `6c568eba6`; baseline was 7.98–8.76 μs/op, 1,737–1,745 B/op, 44 allocs/op and candidate was 8.31–10.61 μs/op, 1,702–1,709 B/op, 42 allocs/op (`bench-handler-stock.log`, `bench-handler-final.log`). It is retained only as supplementary dispatch cost. The stock-only core benchmark in `bench-core-stock.log` is not used for a candidate comparison.

### Matched cost verification for changed MCP paths

The baseline is a new sparse worktree at `/tmp/stable23-36-cost-baseline`, branch `codex/stable23-36-cost-baseline`. Its product base is exact stable `9237ec06672406e437b3cb33fc793ffd0403b3ae`; its final benchmark-only commit is `b711d286da51bd7adba5cf82f47070a80bf3be27`, preceded by test-only `a56ade7c2` (the authorized pre-A Code Mode benchmark source). The only new code in the cost-baseline commits is test code in existing test files; no product source, dependency, or public API changes. Candidate product source remains the unchanged clean `54630233a79dabdb681facb0de97d10227bafe83` tree.

Both sides used Go 1.27.2 and equivalent canonical `go.work` files with all 16 Bifrost modules resolving to their corresponding local worktree directories. Each command also sourced `/workspace/spec21-intake/cloud-env.sh`. The full toolchain/workspace record is `cost-baseline-go-version.txt`; `cost-benchmark-source-manifest.json` records matching SHA-256 hashes and byte lengths for all four benchmark functions and the relevant helper bodies. Exact baseline and candidate commands were:

| Changed path measured | Stable time (range; median) | Candidate time (range; median) | Stable bytes / allocs | Candidate bytes / allocs |
|---|---:|---:|---:|---:|
| Public HTTP `handleMCPServer` tools/call, no App metadata | 12.58–14.46 μs (13.00 μs) | 10.87–14.82 μs (12.24 μs) | 2,437–2,454 B; 52 | 2,416–2,435 B; 50 |
| Core `GetAvailableTools`, no App metadata | 2.78–3.44 μs (3.11 μs) | 2.31–3.06 μs (2.59 μs) | 2,048 B; 9 | 2,048 B; 9 |
| Ordinary `ExecuteChatTool` over local streamable HTTP MCP fixture | 219.83–298.23 μs (223.26 μs) | 150.29–195.98 μs (182.57 μs) | 14,379–14,459 B; 208 | 14,354–14,479 B; 208 |
| Code Mode nested success | 78.40–97.84 μs (91.47 μs) | 76.96–85.98 μs (80.21 μs) | 17,732–17,972 B; 275–276 | 17,656–18,059 B; 275 |
| Code Mode nested protocol error value | 91.95–123.93 μs (103.10 μs) | 78.20–100.37 μs (89.70 μs) | 17,296–17,946 B; 273 | 18,374–18,744 B; 287 |

Every benchmark ran five samples per reported subcase and passed. Timing ranges are workload snapshots, not speed claims. The ordinary public handler, catalog, and MCP execution paths show no allocation increase; the Code Mode protocol-error value path uses 14 more allocations/op and a median ~771 additional bytes than unchanged stable. Its timing samples overlap. This cost is confined to the corrected marked-error path, which now preserves a nested protocol error as a Starlark value; it is explicitly reported rather than generalized as zero cost.

```sh
# Stable product base plus test-only cost baseline
GOWORK=/tmp/stable23-36-cost-baseline/go.work go test ./transports/bifrost-http/handlers -run '^$' -bench '^BenchmarkMCPHTTPNoApp$' -benchmem -count=5
GOWORK=/tmp/stable23-36-cost-baseline/go.work go test ./core/mcp -run '^$' -bench '^BenchmarkGetAvailableToolsNoApps$' -benchmem -count=5
GOWORK=/tmp/stable23-36-cost-baseline/go.work go test ./core/mcp -run '^$' -bench '^BenchmarkExecuteChatToolNoApps$' -benchmem -count=5
GOWORK=/tmp/stable23-36-cost-baseline/go.work go test ./core/mcp/codemode/starlark -run '^$' -bench '^BenchmarkCodeModeNestedToolResponse$' -benchmem -count=5

# Candidate 54630233
GOWORK=/tmp/stable23-36/go.work go test ./transports/bifrost-http/handlers -run '^$' -bench '^BenchmarkMCPHTTPNoApp$' -benchmem -count=5
GOWORK=/tmp/stable23-36/go.work go test ./core/mcp -run '^$' -bench '^BenchmarkGetAvailableToolsNoApps$' -benchmem -count=5
GOWORK=/tmp/stable23-36/go.work go test ./core/mcp -run '^$' -bench '^BenchmarkExecuteChatToolNoApps$' -benchmem -count=5
GOWORK=/tmp/stable23-36/go.work go test ./core/mcp/codemode/starlark -run '^$' -bench '^BenchmarkCodeModeNestedToolResponse$' -benchmem -count=5
```

The baseline log set is `cost-baseline-public-http.log`, `cost-baseline-catalog.log`, `cost-baseline-ordinary-call.log`, and `cost-baseline-codemode.log`. The candidate equivalents are `cost-candidate-public-http.log`, `cost-candidate-catalog.log`, `cost-candidate-ordinary-call.log`, and `cost-candidate-codemode.log`. The source/helper hash manifest is `cost-benchmark-source-manifest.json`.

## Remaining qualification boundary

Cloud evidence here does not claim Docker image parity, strict full Actions checks, the entire repository test suite, or actual host rendering. Issue #37 owns those checks. At initial worker completion, integration tip `codex/stable23-integration` was still `9237ec06672406e437b3cb33fc793ffd0403b3ae`, an ancestor of the worker; no newer integration commit was present to merge. The designated merger subsequently created integration candidate `e949886eb5212db2dc0ac7857e9934527ecb5044`, whose only parent is9237 and whose tree95c8016664ef8a7788e6ea6833cf7e7f9eda90ac equals the tested worker546 tree. Fork draft PR38 targets the stable-pinned base; exact integration-source Actions qualification remains #37. The final matched Newman rerun is complete as recorded above.
