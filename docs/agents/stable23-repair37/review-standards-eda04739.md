Standards review — base `9237ec06672406e437b3cb33fc793ffd0403b3ae`, HEAD `eda04739a7bf77a9be50fc958acda0f455517bba`

No documented-standard breaches found. The final diff updates the affected core, framework, and transport changelogs, adds tests and benchmarks, documents public APIs, and keeps the new refresh lock off request readers. The lock assessment accounts for one fixed 8-byte mutex per handler and expected refresh serialization; the regression exercises public `tools/list` and callback behavior through the approved injected manager seam. The harness boundary is documented as deferred qualification work, not represented as a pass.

Actionable test reliability finding: [transports/bifrost-http/handlers/mcpserver_test.go:1142](../stable23-integration/transports/bifrost-http/handlers/mcpserver_test.go:1142) uses a 100 ms no-capture window to infer that the second refresh is blocked. If its goroutine is not scheduled before the timer expires, the test releases the first refresh before the second reaches `SyncMCPServer`; the pre-fix implementation can then pass. The regression does not deterministically establish the overlapping attempt.

Baseline smell (judgment call): possible Duplicated Code in [core/mcp/mcp.go:357](../stable23-integration/core/mcp/mcp.go:357) and [core/mcp/mcp.go:377](../stable23-integration/core/mcp/mcp.go:377). Both loops traverse per-client tools and apply the same tool-name/resource-URI predicate to locate the owning client. A small lookup helper could share that shape while still revalidating after plugin hooks.

No other baseline smells identified. The mutex has constant per-handler retained memory, is released with the handler, and does not add a lock to HTTP request reads; refreshes queue under contention by design.
