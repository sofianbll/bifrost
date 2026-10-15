## Summary

When `executeToolCode` fails, the gateway turns its flagged Chat tool message into a successful text result. Forward that failure as MCP `isError` while keeping remote protocol-marked tool errors available to the script as values. Native Go exceptions retain upstream runtime-failure behavior.

```text
remote MCP error result -> script value
native Go exception / script failure -> MCP isError
```

Affected packages: `core` and HTTP transport, including their changelogs. First contribution in the A → B → C stack, based on `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`). Candidate A: `f136c93fcf78f3db3df59974ae49cfc392412f51`.

## Evidence

- **Before:** pinned stock 2.2.6 loses outer `isError` on the script-failure wire assertion; remote nested validation errors already remain script values.
- **After:** the [independent A/B/C campaign](https://github.com/sofianbll/bifrost/actions/runs/38715391307) builds A from its own immutable checkout on AMD64: Go/UI build, package/race checks, dynamic plugin and exact built-UI probes pass. Its MCP suite reports 675 passed, 29 skipped, zero failures/errors. Harness source is separately pinned at `1dcbd14d5875a14dc780f00fc2edd22cfaee66a8`.
- Whole-stack wire evidence from C in that same campaign executes both error requests / six assertions. The strict stock comparator confirms the corrected script-failure behavior and the already-green nested-error invariant. This wire result belongs to C, not an isolated A HTTP run.
- The [integrated fork campaign](https://github.com/sofianbll/bifrost/actions/runs/38715303556) independently qualifies `19362d86b2a3d17b699283c98f67861cbc6dc46c` on AMD64/ARM64. New invocation-time and pre-hook Code Mode authorization remain intact. No live-provider or native Codex check is claimed.

## Merge Danger

**Door:** two-way. **Blast Radius:** Code Mode error reporting. No storage or public configuration schema change. Remote MCP error values and native Go exceptions intentionally retain distinct behavior. The prepared source is qualified on the frozen 2.2.6 base; submitting it against a newer upstream target requires a separate compatibility check.
