## Summary

When `executeToolCode` fails, the MCP gateway currently turns its flagged Chat tool message into a successful text result. Forward that failure as MCP `isError` while keeping remote protocol-marked tool errors available to the script as values. Native Go handler exceptions retain the upstream runtime-failure behavior.

```text
remote MCP error result -> script value
native Go exception / script failure -> MCP isError
```

## Evidence

- Before: the handler regression loses `isError`; the legacy fork also fails seven STDIO error scenarios.
- After: focused handler and Starlark checks pass; the first full 2.2.4 CI passes those seven scenarios. Its remaining native-handler regression is corrected without changing upstream expectations; the rerun must confirm it.
- Two offline error-test-server harness cases are included. Full verification is tracked in the recovery ledger; do not mark this ready before the current CI finishes.

## Merge Danger

**Door:** two-way. **Blast Radius:** Code Mode error reporting. No storage or public configuration schema change. Remote MCP error values and native Go exceptions intentionally retain distinct behavior.
