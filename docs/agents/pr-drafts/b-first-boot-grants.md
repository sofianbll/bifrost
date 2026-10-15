## Summary

On an empty store, Virtual MCP reconciliation runs before config-declared virtual keys exist, so their attachments are lost. Reconcile after governance hydration while preserving upstream's deferred governance resets and auth-load error handling.

```text
MCP clients -> governance keys -> Virtual MCP attachments
```

Affected package: HTTP transport, with its changelog. Second contribution in the A → B → C stack. Base A: `f136c93fcf78f3db3df59974ae49cfc392412f51`; candidate B: `adb80e661560174025ff9f9c23b918cb377e14b9`. The common upstream base is `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`).

## Evidence

- **Before:** `TestLoadConfig_FirstBootVirtualMCPVirtualKeyAssignment` fails on unchanged stock 2.2.6: expected `vk-1`, actual no attachment.
- **After:** the same regression passes after the reconciliation move. The retained first-boot/stickiness race run passes with `-count=20`; deferred governance resets and auth-load errors remain covered.
- The [independent A/B/C campaign](https://github.com/sofianbll/bifrost/actions/runs/38715391307) builds B from its own immutable checkout on AMD64: Go/UI build, package/race checks, dynamic plugin and built-UI probes pass. Its MCP suite reports 675 passed, 29 skipped, zero failures/errors. Harness source is separately pinned at `1dcbd14d5875a14dc780f00fc2edd22cfaee66a8`.
- C's whole-stack wire run in that campaign passes 39 requests / 149 assertions, including Virtual MCPs created from an empty store. B's independent build is separate from C's HTTP/App qualification.
- The historical existing-store persistent-connection 403 in [fork issue #11](https://github.com/sofianbll/bifrost/issues/11) remains open / needs-info. This first-boot correction does not establish its cause.

## Merge Danger

**Door:** two-way for the code. **Blast Radius:** startup reconciliation. Existing source-of-truth rules and schemas are unchanged; configured attachments become effective on the first boot. The source is qualified on 2.2.6; a newer upstream target needs fresh compatibility evidence. No production store or setting was changed.
