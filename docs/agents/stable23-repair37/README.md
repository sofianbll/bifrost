# Issue 37 — bounded product repair, focused proof only

**Implemented, focused-tested and independently reviewed; NOT fully qualified. PR38 remains draft; issue37 incomplete.** This repairs a confirmed controlled lost update, not a proven cause of historical startup run38938846259. No full qualification/host/browser/startup campaign, provider call, production change, upstream write, merge or deployment occurred.

## Sources and changes

- Stable/base: `9237ec06672406e437b3cb33fc793ffd0403b3ae`.
- New exact candidate: `b38fadec8d78ec64ffbefb53f650e6ef149d948e`, parent `eda04739a7bf77a9be50fc958acda0f455517bba`, tree `43af221324ecb85d1c4b61a56c99dd6751fc293d`.
- Product correction commit `eda04739a7bf77a9be50fc958acda0f455517bba`, parent original candidate `e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05`. Final commit tightens regression scheduling only.
- Qualification driver remains `b838f2e5c3d3cf2340b7fee6c0cfcf2ec1ad0efb`; diagnostic orchestration remains `2474aa7bc08c602d792dcc02b902ef5c3241f83a`. Neither was merged into the candidate.
- Repair delta from e33: 3 files, +189/-1: `transports/bifrost-http/handlers/mcpserver.go`, its existing `mcpserver_test.go`, and `transports/changelog.md`. Full stable-base final diff: 42 files, +4046/-130, four commits; no later-dev integration merge.
- [PR38](https://github.com/sofianbll/bifrost/pull/38), [approved diagnostic and proposed diff](https://github.com/sofianbll/bifrost/issues/37#issuecomment-6223683514), [new bounded authority checkpoint](https://github.com/sofianbll/bifrost/issues/37#issuecomment-6224110231).

A per-handler `sync.Mutex` serializes **catalog capture, build and atomic publication** in `SyncMCPServer`. Locking only publication would leave stale captures able to win. Request readers continue loading their immutable snapshot without taking the refresh mutex. No grant, auth, admission, hook, cache or lifecycle policy changed.

## Red before green

The initial expected red was retained before applying the mutex: serial/empty/removal controls passed, overlapping capture failed, and public HTTP `tools/list` lacked `modern-read_state`.

The **final tightened regression** was also run against the frozen original product function using a Go overlay of **only** `mcpserver.go` from e33. It again fails for overlapping capture and missing modern tool (exit1), with serial controls passing. Original pins and product files were not edited by that counterfactual. Overlay mapping and source checksum are retained. This is a controlled product regression, not a natural startup reproduction.

On clean final b38, both new tests pass with the race detector as part of the handler suite. The test holds the first capture, checks public HTTP readers still serve the previous snapshot, acknowledges the second worker at the call site, checks bounded capture exclusion, releases and joins both workers, requires all three source-qualified names, and calls the modern callback alias from the same snapshot. Serial controls cover addition, removal/narrowing, empty catalogs and removed aliases. Existing denial/auth/source-isolation tests remain effective.

**Scheduling limit:** the 100 ms negative window is a bounded scheduling assertion, not proof of the runtime's internal mutex state. The worker-start handshake resolves the independent review concern that the worker had not reached its call site. No arbitrary startup sleep or weakened catalog assertion was introduced.

## Focused race proof

Go `go1.27.2 linux/amd64`, `CGO_ENABLED=1`, all16 Bifrost workspace modules resolved to owned local candidate sources. These are focused Cloud source tests, not an image or complete stable compatibility campaign.

| Suite | top-level tests | including subtests | failures | skips | shell exit |
|---|---:|---:|---:|---:|---:|
| handler | 26 | 76 | 0 | 0 | 0 |
| server | 7 | 7 | 0 | 0 | 0 |
| core MCP | 7 | 10 | 0 | 0 | 0 |
| total | 40 | 93 | 0 | 0 | 0 |

Coverage includes refresh ordering and removals; App metadata/schema and qualified resources; callback uniqueness, ambiguity and admitted-source isolation; auth/JWT/anonymous and denied admission; effective native post-hook/error behavior; resource hooks and hidden denials; metadata cache bounds and unchanged/changed discovery callbacks; server persistence and key-cache reload controls. `sanitized-race-results.json` retains every test outcome and the raw Cloud log hashes; complete raw logs remain in the existing Cloud task.

The first default combined race attempt failed **at linking**, `No space left on device`, not a test assertion. That failure remains in `initial-default-race-link-failure.json`. Recovered within the same VM using a task-owned writable cache view, sequential package builds (`-p 1`) and `-ldflags=-w`, which omits DWARF debug sections only; race instrumentation remains enabled. No old worktree, historical evidence, original f3/fc cache backing or platform was removed/replaced. Only obsolete cache artifacts created during this phase were byte-preserved in hash-verified compressed archives. This build-flag/resource adaptation is explicitly part of the focused proof, not the future exact-image qualification configuration.

Commands below ran only in the existing Cloud workspace, after sourcing `/workspace/spec21-intake/cloud-env.sh`; temporary cache paths belong to this task:

```bash
export GOCACHE=/tmp/stable23-repair37-cache-view
export GOTMPDIR=/tmp/stable23-repair37-go
export TMPDIR=/tmp/stable23-repair37-go
# cwd /workspace/stable23-integration/transports
go test -race -p 1 -ldflags=-w -json -count=1 -timeout=4m ./bifrost-http/handlers -run '^(Test(SyncMCPServer|MCPApps|MCPAuth|CallTool|ToolsList|ResourcesList|Initialize|ConvertToolFunction|Authenticate|Admit|GetVK))'
go test -race -p 1 -ldflags=-w -json -count=1 -timeout=4m ./bifrost-http/server -run '^(Test(SyncMCPServer|PersistMCPClientTools|ReloadVirtualKey|BifrostHTTPServerImplementsMCPApps))'
# cwd /workspace/stable23-integration/core
go test -race -p 1 -ldflags=-w -json -count=1 -timeout=4m ./mcp -run '^(TestExecuteMCPAppToolPreservesNativeFieldsAndDropsStaleHookOutput|TestReadMCPAppResourceUsesMCPHooksAndHidesDenials|TestSetClientTools_UnchangedTools_DoesNotFireCallback|TestSetClientTools_MetadataOnlyChangeRefreshesCatalog|TestConvertMCPToolToBifrostSchema_PreservesMCPAppDefinition|TestConvertMCPToolToBifrostSchemaBoundsRetainedMCPAppMetadata|TestComputeToolsHash_IncludesCachedMCPAppsMetadata)$'
# cwd /workspace/stable23-integration/transports; expected exit1
GOMAXPROCS=4 go test -p 1 -ldflags=-w -overlay=/workspace/stable23-repair37/parent-product-overlay.json ./bifrost-http/handlers -run '^TestSyncMCPServer(SerialCatalogChanges|OverlappingRefreshKeepsLatestCatalog)$' -count=1 -v
```

The overlay file maps the candidate's absolute `mcpserver.go` path to `git show e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05:transports/bifrost-http/handlers/mcpserver.go`. No test-only hook or generic framework was added. Existing frozen driver's strict source-specific wire catalog checks remain required for future qualification; no redundant nondeterministic Postman case or driver edit was added to force internal scheduling. This follows the explicitly approved controlled-manager/public HTTP seam, with the concurrency-test boundary documented in the lock assessment and reviewed independently.

## Lock ordering and measured cost

All repository callers were inspected. Mutations return from config/client locks before sync; tools-change callbacks are fired outside manager.mu and launch sync asynchronously. Refresh lock order is handler mutex -> in-memory manager read locks; capture/build performs no upstream dial, DB write, plugin tool execution or reentrant sync. Fresh server registration does not execute handlers. No current inversion/reentrant caller found. Custom managers must likewise avoid reentrant sync from catalog enumeration.

The new retained memory is one 8-byte mutex per long-lived linux/amd64 handler, freed with that handler. No new per-request/key state or worker is created. Existing concurrent refresh callers queue; the fix does not batch/coalesce their work.

Matched before/after use identical final benchmark functions, same Go/toolchain/link flags, `GOMAXPROCS=4`, `-p 1 -ldflags=-w -benchmem -benchtime=200ms -count=3`. Before overlays only the old product file; after is clean b38. Three samples each, on shared Cloud hardware:

| Operation | before ns/op range | after ns/op range | before/after allocs/op |
|---|---:|---:|---:|
| ordinary public HTTP | 12065–12841 | 9905–11176 | 50 / 50 |
| serial refresh, 3 tools | 16611–17720 | 16688–19892 | 95 / 95 |
| parallel refresh, 4 workers, one handler | 8604–10117 | 20512–21784 | 95 / 95 |

No speedup claim. Refresh throughput under contention decreases intentionally because full refresh work is now serialized. These synthetic three-tool measurements are not a large-catalog or whole-service performance guarantee. Raw samples are retained in both matched bench logs and JSON. The benchmark command is the same on both sides except the old-product overlay:

```bash
GOMAXPROCS=4 go test -p 1 -ldflags=-w ./bifrost-http/handlers -run '^$' -bench '^(BenchmarkSyncMCPServerCatalog|BenchmarkMCPHTTPNoApp)$' -benchmem -benchtime=200ms -count=3
```

## Independent final reviews

Both GPT-6 Luna agents reviewed `git diff 9237ec06672406e437b3cb33fc793ffd0403b3ae...b38fadec8d78ec64ffbefb53f650e6ef149d948e`, the complete final stable diff, independently in Cloud.

- Standards: **0 actionable findings, 1 nonblocking judgment-call smell**. The initial scheduling finding was fixed in the second and final round. The retained possible duplicated resource lookup surrounds plugin hooks; revalidation is required and extracting a helper is unnecessary for this mutex repair. Full report retained verbatim.
- Spec: **0 findings**, worst none. Full report retained verbatim. No code-review result is treated as runtime qualification.

This new authorization used two bounded rounds; the prior two issue37 cycles remain exhausted and were not reset. No additional product repair is authorized here.

## Remaining unverified

No new full Actions qualification status was posted or copied. Old exact-candidate failure remains tied to e33. New b38 has **no stable compatibility verdict**. No current stock/candidate wire/Newman, Code Mode wire, same-store lifecycle/cache/reset campaign, actual pinned reference host, native Codex rendering or live-model streaming proof was run. Historical run38938846259's modern-empty cause remains unestablished; the prior 14native/3Actions non-reproductions are not rewritten as evidence for this new SHA.

PR38 must remain draft and #37 open/incomplete. Full qualification requires a separate future owner request. No Mac fallback, provider calls/secrets/spend, production/Pulsar/auth-policy/source-authority changes, upstream write, migration, merge, tag/image release or deployment.
