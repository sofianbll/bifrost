## Summary

When `executeToolCode` fails, the MCP gateway currently turns its flagged Chat tool message into a successful text result. Forward that failure as MCP `isError` while keeping remote protocol-marked tool errors available to the script as values. Native Go handler exceptions retain the upstream runtime-failure behavior.

```text
remote MCP error result -> script value
native Go exception / script failure -> MCP isError
```

## Evidence

Based on `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`). [Integration CI 38181001842](https://github.com/sofianbll/bifrost/actions/runs/38181001842) qualifies commit `e1ca7961489c07d348fe19c95fc26a423dffb64a`; the three contribution branches also compile independently with identical stock UI assets.

- **Before:** the named stock 2.2.6 wire assertion for script failure loses outer `isError`. Remote nested validation errors already remain script values upstream.
- **After:** both exact error-server requests pass all six assertions. The comparator confirms the script-failure correction while preserving the existing green nested-error behavior.
- Focused handler, Starlark and STDIO checks pass. Both architecture MCP suites pass 675 tests / 29 skipped / zero failures. New upstream invocation-time and pre-hook Code Mode authorization remain intact.

## Merge Danger

**Door:** two-way. **Blast Radius:** Code Mode error reporting. No storage or public configuration schema change. Remote MCP error values and native Go exceptions intentionally retain distinct behavior.
