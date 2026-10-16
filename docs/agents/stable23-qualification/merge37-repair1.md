# Stable #37 repair 1 integration merge

Fast-forwarded `codex/stable23-integration` to the exact worker commit whose final benchmarks were recorded. No new commit was created; worker commit and history are preserved.

- Integration HEAD: `e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05`
- Parent: `e949886eb5212db2dc0ac7857e9934527ecb5044`
- Stable ancestry: `9237ec06672406e437b3cb33fc793ffd0403b3ae` → `e949886eb5212db2dc0ac7857e9934527ecb5044` → `e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05`
- Tree: `702a0479d7f5dd6dbf5df0fa4c0b68fc74850dc0`, identical to worker tree.
- Commit subject: `[test]: benchmark native MCP Apps request paths`; signed off by OpenAI Codex.
- Integration and worker tracked worktrees are clean. `git show --check` passed.

Only these two existing test files changed (247 insertions):

```text
core/mcp/toolcall_recovery_test.go
transports/bifrost-http/handlers/mcpserver_test.go
```

The repair adds public-handler benchmarks and preflights for native MCP Apps `tools/call` and `resources/read`, plus core `MCPManager` native path benchmarks using a bounded loopback fixture. No product/runtime/API/dependency, workflow, harness, or changelog changes are included.

The final benchmark commands passed all five samples and preflights on the exact HEAD. Full method, sample data, limitations, and retained logs are documented in `issue37-product-repair1.md`, `issue37-handler-final-e33b67ab6.log`, and `issue37-core-final-e33b67ab6.log` in this evidence directory. No additional build or test was run as part of the merger task.
