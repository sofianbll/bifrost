# Issue #37 Standards repair 1 — native MCP Apps request-path benchmarks

## Finding and change

The initial Standards review of e949 found that new MCP Apps `tools/call` and `resources/read` request paths had no benchmark in an existing test file. The adjacent Spec review found zero product findings. This repair adds only benchmarks and their preflight assertions; product/runtime source, public APIs, dependencies, harness/workflows, and changelogs are unchanged.

The repair commit is `e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05` (`[test]: benchmark native MCP Apps request paths`), based directly on integration `e949886eb5212db2dc0ac7857e9934527ecb5044`. The repair worker branch was clean at the recorded commit. The commit changes only `transports/bifrost-http/handlers/mcpserver_test.go` and `core/mcp/toolcall_recovery_test.go`.

- `BenchmarkMCPHTTPAppToolCall` drives `tools/call` through `MCPServerHandler.handleMCPServer`. Its untimed preflight checks the public tool name, input schema, source-qualified App resource URI, App visibility metadata, and native `content`, `structuredContent`, and `_meta`. The native payload differs from the compatibility fallback, so this does not silently benchmark fallback behavior.
- `BenchmarkMCPHTTPAppResourceRead` drives `resources/read` through the same public handler. Its preflight verifies `resources/list` advertises the qualified URI and MIME type and that the read returns the expected qualified URI, HTML MIME type, and fixture bytes.
- `BenchmarkMCPAppCoreNativePaths` covers both `MCPManager.ExecuteMCPAppTool` and `MCPManager.ReadMCPAppResource` against a real local Streamable HTTP MCP server, with setup/connection outside the timed loops and the existing pass-through `nativeResultMutationPipeline` installed. Preflight checks native structured content/meta and the resource URI/MIME/body.

Each fixture is bounded to one client, one App tool, one resource, a fixed small native response, and fixed HTML. The handler fixture manager is an in-process seam; the core fixture uses loopback HTTP. No provider or external MCP server is used. The handler’s admission hook is the existing no-admitter path; the core benchmark uses the standard no-op pre/post pipeline. Existing handler/core tests retain auth/visibility/denial and hook-redaction controls.

## Why stable has no successful-operation numeric baseline

The unchanged stable stock gateway is commit `9237ec06672406e437b3cb33fc793ffd0403b3ae`, binary SHA-256 `f58728f39a7d98f7a82b29563b40a9926ca7bc8a807e9ed0abc834bc3578c9fb`. Its native capture is in `stock-canonical-native-probes/native-characterization.json`. On stock, `direct.resources-list` and `direct.resources-read` each returned HTTP 200 with JSON-RPC error `-32601`, `resources not supported`; the negotiated UI initialize assertion failed and the Apps tool was absent from the catalog. The matched stock Newman run recorded 39 Apps requests / 148 assertions with 34 failures and zero skips, plus 3 Code Mode requests / 10 assertions with 2 failures and zero skips, as documented in `issue36-implementation.md`.

Consequently stock cannot execute the successful native App operation measured here. Measuring stock’s unsupported-method/error latency would compare different semantics and is not presented as a baseline or speedup. The existing matched stable/candidate ordinary public-handler and core costs remain in `issue36-implementation.md`; they do not stand in for Apps-path measurements.

## Toolchain, source, and commands

The Cloud worker toolchain profile used Go `go1.27.2` and a canonical full workspace with directive 1.27.2; all 16 Bifrost modules resolved to local directories in the exact candidate checkout. The module/source identity output is retained in `issue37-go-modules-e33b67ab6.txt`. No `GITHUB_ACTIONS` value was set. The final source HEAD and clean status were checked before the authoritative measurements.

Exact authoritative commands:

```sh
GOWORK="$PWD/go.work" go test ./transports/bifrost-http/handlers -run '^$' -bench '^BenchmarkMCPHTTPApp(ToolCall|ResourceRead)$' -benchmem -count=5
GOWORK="$PWD/go.work" go test ./core/mcp -run '^$' -bench '^BenchmarkMCPAppCoreNativePaths$' -benchmem -count=5
```

The complete logs pinned to commit `e33b67ab6` are `issue37-handler-final-e33b67ab6.log` and `issue37-core-final-e33b67ab6.log`. Every benchmark preflight passed and each command reported `PASS`.

| Successful native candidate path | Five `ns/op` samples | Median `ns/op` | `B/op` range (median) | `allocs/op` |
|---|---|---:|---:|---:|
| Public handler `tools/call` | 132,301; 62,199; 63,851; 55,764; 70,513 | 63,851 | 6,317–6,434 (6,339) | 122 |
| Public handler `resources/read` | 22,011; 16,042; 12,125; 11,023; 11,797 | 12,125 | 2,014–2,027 (2,024) | 33 |
| Core `ExecuteMCPAppTool` | 230,970; 248,163; 229,117; 224,873; 222,176 | 229,117 | 20,336–20,839 (20,515) | 294 |
| Core `ReadMCPAppResource` | 191,395; 200,933; 196,419; 189,394; 241,982 | 196,419 | 17,463–17,588 (17,529) | 246 |

Timing samples varied, especially the public-handler first sample; these are measured workload snapshots, not a before/after performance claim. Bytes and allocations were consistent across each operation’s samples. Core timings include local loopback MCP HTTP, connection execution, and no-op pipeline costs; public handler timings use a synthetic in-process tool manager and direct fasthttp handler invocation, not TCP listener/network cost.

The source block hashes at this commit are:

- Both public handler benchmark functions: 7,742 bytes, SHA-256 `c969aed50992cee14b0b31245486090085141d34e878dc35083458bda76ce138`.
- Existing handler fixture manager helper used by them: 1,893 bytes, SHA-256 `e1ac37c6f02c2765c5356515508495ba07f4c0636cbc14c7c9165581371ce58d`.
- Core benchmark function: 4,097 bytes, SHA-256 `62d2674001749f6b23669d3211f28e9cff9f62dc3a90dc72c88250b3594fc6fd`.

## Focused verification

The authoritative benchmark runs above exercise both new benchmark preflights. Relevant existing behavior tests also passed:

```sh
GOWORK="$PWD/go.work" go test ./transports/bifrost-http/handlers -run '^(TestInitializeNegotiatesMCPAppsExtensionOnlyWhenRequested|TestMCPApps.*|TestCallToolPreservesNativeMCPAppResultAndUsesEffectiveFallback|TestCallToolRefusesHiddenToolWithoutDisclosingItsName|TestToolsListPreservesAndQualifiesMCPAppMetadata|TestMCPAppsPreservesSupportedOutputSchema|TestResourcesListAndReadAreSourceQualified)$' -count=1
GOWORK="$PWD/go.work" go test ./core/mcp -run '^(TestExecuteMCPAppToolPreservesNativeFieldsAndDropsStaleHookOutput|TestReadMCPAppResourceUsesMCPHooksAndHidesDenials)$' -count=1
```

Both passed. The matching handler cases cover extension negotiation, source-qualified callback selection/ambiguity/admission denial, native result preservation/fallback, hidden-tool denial, tool schema/metadata, output schema, and resource qualification/read. The core cases cover native-result hook mutation and resource hook/redaction/denial. Logs are `issue37-handler-focused-e33b67ab6.log` and `issue37-core-focused-e33b67ab6.log`. `git diff --check` passed; final tree is clean.

This Cloud repair addresses the Standards benchmark-coverage finding only. Docker/Actions strict matrix and host qualification remain with issue #37’s qualification gates.
