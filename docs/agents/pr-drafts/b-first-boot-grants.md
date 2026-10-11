## Summary

On an empty store, Virtual MCP reconciliation currently runs before config-declared virtual keys exist, so their attachments are lost. Reconcile after governance hydration, while preserving Bifrost 2.2.4's deferred governance resets.

```text
MCP clients -> governance keys -> Virtual MCP attachments
```

## Evidence

Ported to `transports/v2.2.6` (`8b4fce4f1709d66f9208d02f50552da522535f9e`). Focused local checks and independent HTTP builds pass; real 2.2.6 CI qualification is pending. The CI results listed below are historical 2.2.4 evidence.

- Before: `TestLoadConfig_FirstBootVirtualMCPVirtualKeyAssignment` is red on stock 2.2.4: expected `vk-1`, actual no attachment.
- After: the same test passes, including `-race -count=20` (18.033 seconds). Focused checks pass again after stack integration.
- Wire verification passes in the integration branch fixture gate of CI run 38172752771 (37 MCP Apps requests, 143 assertions, no assertion failure), including Virtual MCPs created from an empty store. The old persistent-connection HTTP 403 remains a separate unresolved diagnosis.

## Merge Danger

**Door:** two-way for the code. **Blast Radius:** startup reconciliation. Existing source-of-truth rules and schemas are unchanged; the fix makes configured attachments effective on the first boot.
