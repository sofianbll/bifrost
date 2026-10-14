# Existing-store persistent HTTP MCP restart comparison

Scope: fork issue [#11](https://github.com/sofianbll/bifrost/issues/11). This is an automated synthetic HTTP/OAuth test in GitHub Actions, with no personal Codex App session, production connection/store, live OAuth account or paid provider. Bifrost 2.3 is out of scope.

The same command builds a disposable existing SQLite store, obtains a Bifrost-issued OAuth JWT through registration, PKCE authorization, virtual-key consent and token exchange, then exercises `initialize`, `tools/list` and an exact echo through `/mcp/persistent-http`. It enables `needs_session_stickiness` through the public admin API and repeats those authorized requests after two restarts of the same binary and store. The original config file still specifies `false`: the persisted update must hydrate as `true`. The HTTP server behind the gateway uses a synthetic static Bearer credential and real MCP sessions; live upstream-provider OAuth is not exercised.

```bash
python3 .github/workflows/scripts/test-persistent-mcp-restart.py --binary "$RUNNER_TEMP/bifrost-restart" --variant "$VARIANT" --commit "$SOURCE_COMMIT" --output "$RUNNER_TEMP/persistent-restart/$VARIANT.json"
```

The command refuses application execution outside GitHub Actions. The workflow compiles the pinned stock source and the fork independently, with transport version `v2.2.6` and a dummy embedded HTML file. It tests backend HTTP/OAuth behavior; it does not qualify a built UI or a published image.

## Recorded runs

| Run | Source | Outcome |
| --- | --- | --- |
| [38629558244](https://github.com/sofianbll/bifrost/actions/runs/38629558244) | stock `8b4fce4f1709d66f9208d02f50552da522535f9e`; harness/fork `d7416ed5b3b13a2f961c9b6e60d83e8a6c56c65e` | Both probes failed before authorization: fixture readiness filtered `id`, whereas the public MCP configuration uses `client_id`. This is a harness defect, not the historical 403. |
| [38630801599](https://github.com/sofianbll/bifrost/actions/runs/38630801599) | same stock; fork `f4527a9ab72210952c03e37ba30e200c5cea5bb4` | Both pass: six real echoes each, VK and OAuth JWT authorized before/after two restarts, same SQLite file, original config unchanged, both deliberate 403 controls rejected. Initial-boot SIGTERM cleanup passes. |
| [38632070370](https://github.com/sofianbll/bifrost/actions/runs/38632070370) | same stock; fork `a6f3075a954e716ae801f03d89e02c3634136604` | Both probes and all four cleanup controls pass. The two pristine cleanup defects go red; corrected controls explicitly interrupt and remove the live child/private files. The driver also removes every intentional legacy residue. |

The negative authorization control uses another active, valid synthetic virtual key with no client grant. The actual HTTP response must be exactly `403 access_denied`, and the same authorized-request assertion must reject it. This deliberate denial proves the check is sensitive; it does not reproduce the historical incident.

The independent cleanup control uses a real gateway child and disposable SQLite store. It compares the pristine scripts from `d7416ed5b` (initial-boot SIGTERM) and `f4527a9ab` (SIGTERM during normal cleanup) against the corrected script. The latter signal is injected deterministically at the external `subprocess` boundary: stop the real child when termination begins, then signal the parent at its first `wait`. It changes no Bifrost code or HTTP assertion. The driver detects and then kills/reaps old leftover children and removes its private temporary files. A corrected probe must explicitly report `interrupted`, exit 143 and leave no live child or private files.

## Repair-cycle ledger

1. **Cycle 1:** `d7416ed5b` → `f4527a9ab`. Fix the harness's `client_id` contract; extract concrete configuration, gateway-lifecycle and MCP assertion responsibilities; install controlled SIGTERM/SIGINT handling. Run 38630801599 passes the HTTP/OAuth scenario on stock and fork, plus initial-boot interruption. Review finds a remaining first-signal edge during normal gateway `wait`.
2. **Cycle 2:** `f4527a9ab` → `a6f3075a9`. A nested `finally` kills and reaps a still-live child before the interrupted wait propagates. Run 38632070370 passes the same HTTP scenario and calibrates the exact earlier script defects: `d7416ed5b` leaves a child/private directory after initial-boot SIGTERM; `f4527a9ab` reports interruption but leaves a stopped live child after a signal during normal cleanup. Both corresponding corrected cases leave no live child/private directory. The external-process driver removes the deliberate legacy residues on success and failure paths. Two repair cycles were used; no product fix or historical 403 cause was inferred.

The final HTTP probe takes 9.661 seconds on the fork and its exact stock duration is retained in `final/stock.json`. CI job startup, dependency compilation and the independent cleanup controls are separate from that duration. The final tested source is `a6f3075a954e716ae801f03d89e02c3634136604`; these proof documents are versioned in a subsequent docs commit and do not imply a test result at that docs SHA.

## Interpretation and evidence boundary

Both stock 2.2.6 and the fork preserve authorization in the tested synthetic scenario. No historical red reproduction or cause is established, and no production fix is attributed. Issue #11 must remain open if the final synthetic run passes.

Missing evidence is a sanitized trace of an actually failing historical request: exact route/method and `403` body, credential kind (VK header or Bifrost JWT), affected upstream auth type and connection state, stable redacted client/grant identifiers, persistent-flag transition, tested version and store/migration history, and correlated boot/admission/upstream failure timestamps. The fixture creates an existing store during the test; it cannot reconstruct unknown older store contents or a real provider's OAuth session lifecycle.

There are no UI captures because no UI is exercised. `codex_verification: not_run` and automated validation are recorded separately in every probe report. Private process logs, config, credentials, tokens and database files are destroyed inside the runner and never uploaded. Only JSON status/proof summaries are retained here.
