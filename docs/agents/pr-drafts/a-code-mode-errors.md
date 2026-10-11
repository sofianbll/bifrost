## Summary

When `executeToolCode` fails, the MCP gateway currently turns its flagged Chat tool message into a successful text result. Forward that failure as MCP `isError` while keeping remote protocol-marked tool errors available to the script as values. Native Go handler exceptions retain the upstream runtime-failure behavior.

```text
remote MCP error result -> script value
native Go exception / script failure -> MCP isError
```

## Evidence

- Before: the handler regression loses `isError`; the legacy fork also fails seven STDIO error scenarios.
- After: focused handler and Starlark checks pass. Both architecture reports in CI run 38172752771 contain 699 total cases, 670 passed, 29 skipped, zero failures and zero errors; the seven legacy failures and native-handler regression pass without changing upstream expectations.
- Both offline error-test-server wire cases pass in CI run 38172752771 (2 requests, 6 assertions, no assertion failure). Full release verification is tracked in the recovery ledger.

## Merge Danger

**Door:** two-way. **Blast Radius:** Code Mode error reporting. No storage or public configuration schema change. Remote MCP error values and native Go exceptions intentionally retain distinct behavior.
