## Summary

Relay MCP Apps natively through direct, Virtual MCP and aggregate routes: preserve UI tool definitions, serve authorized HTML resources, route callbacks without ambiguous source selection, and forward the full native tool result. Preserve App metadata through config-store reload. A directly called App stays available when its client uses Code Mode; scripts return text and do not open an App view.

```text
host -> admitted tool -> upstream App
     <- native result / namespaced UI resource
iframe callback -> unambiguous admitted source
```

## Evidence

- Schema and SQL regressions fail when gateway-only metadata is stored as flat Chat tools, then pass with the metadata envelope. Reload through `AddClient` is covered.
- Focused core, config-store, handler and server checks pass. The fixture's MCP result decoder rejects an empty content type, then passes with valid text content.
- The shared offline fixture contributes 37 MCP-only wire cases. Fork CI also contains pinned real-server and reference-host gates; their actual run and the final Codex check remain required before opening this PR.
- See the existing fork architecture document for the reviewer diagram; fork QA files, CI and cloud tooling are excluded from this branch.

## Merge Danger

**Door:** two-way with a compatible store snapshot. **Blast Radius:** MCP discovery, results, resources, callbacks and cached tool metadata. The new reader accepts old flat rows, but stock binaries cannot read the new wrapped tool-cache format. Preserve a store backup before production qualification; a downgrade to stock requires restoring or rediscovering the old cache. No production rollout is part of this PR.
