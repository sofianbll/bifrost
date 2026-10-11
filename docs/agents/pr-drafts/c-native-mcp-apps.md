## Summary

Relay MCP Apps natively through direct, Virtual MCP and aggregate routes: preserve UI tool definitions, serve authorized HTML resources, route callbacks without ambiguous source selection, and forward the full native tool result. Preserve App metadata through config-store reload. A directly called App stays available when its client uses Code Mode; scripts return text and do not open an App view.

```text
host -> admitted tool -> upstream App
     <- native result / namespaced UI resource
iframe callback -> unambiguous admitted source
```

## Evidence

Based on `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`). [Integration CI 38181001842](https://github.com/sofianbll/bifrost/actions/runs/38181001842) qualifies commit `e1ca7961489c07d348fe19c95fc26a423dffb64a`; the three contribution branches also compile independently with identical stock UI assets.

- **Before:** metadata persistence regressions lose `_meta.ui` with flat Chat tool rows. The stock 2.2.6 fixture demonstrates eight named missing-feature assertions while six existing-behavior/positive fixture invariants already pass.
- **After:** schema/SQLite, live-map lookup, core, handler and server checks pass. All 37 exact Apps wire requests pass 143 assertions; the stock comparator validates the eight red sentinels and six green invariants. This is a whole-stack wire comparison, not isolated attribution of every stock failure to this relay.
- The integrated four-real-App qualification passes 80/80 controls. The pinned official reference host passes 18/18 browser cases in 48 seconds: 16 renders, four simultaneous connections and actual checkpoint edit/save/read from a fresh view with three elements. Screenshots were inspected; the restored seed label is partly clipped, so full viewport framing/native Codex remain owner release checks.
- The second Code Mode error pair passes six assertions; both architecture MCP suites pass 675 tests / 29 skipped / zero failures. Fork QA, image/cloud tooling and this external qualification runner are excluded from the contribution branch. The existing fork architecture document supplies the reviewer diagram.

## Merge Danger

**Door:** two-way with a compatible store snapshot. **Blast Radius:** MCP discovery, results, resources, callbacks and cached tool metadata. The new reader accepts old flat rows, but stock binaries cannot read the new wrapped tool-cache format. Preserve a store backup before production qualification; a downgrade to stock requires restoring or rediscovering the old cache. No production rollout is part of this PR.
