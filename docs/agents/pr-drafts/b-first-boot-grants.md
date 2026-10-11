## Summary

On an empty store, Virtual MCP reconciliation currently runs before config-declared virtual keys exist, so their attachments are lost. Reconcile after governance hydration, while preserving upstream's deferred governance resets and auth-load error handling.

```text
MCP clients -> governance keys -> Virtual MCP attachments
```

## Evidence

Based on `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`). [Integration CI 38181001842](https://github.com/sofianbll/bifrost/actions/runs/38181001842) qualifies commit `e1ca7961489c07d348fe19c95fc26a423dffb64a`; the three contribution branches also compile independently with identical stock UI assets.

- **Before:** `TestLoadConfig_FirstBootVirtualMCPVirtualKeyAssignment` is red on unchanged stock 2.2.6: expected `vk-1`, actual no attachment.
- **After:** the same test passes after the reconciliation move. First-boot/stickiness pass `-race -count=20` (43.079 seconds); upstream deferred governance resets and auth-load error handling remain intact.
- The shared fixture passes 37 MCP Apps wire requests / 143 assertions, including Virtual MCPs booted from an empty store. Both architecture MCP suites pass 675 tests / 29 skipped / zero failures. The historical persistent-connection HTTP/OAuth 403 remains a separate unresolved diagnosis.

## Merge Danger

**Door:** two-way for the code. **Blast Radius:** startup reconciliation. Existing source-of-truth rules and schemas are unchanged; the fix makes configured attachments effective on the first boot.
