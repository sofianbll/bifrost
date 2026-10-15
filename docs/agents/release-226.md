# Bifrost 2.2.6 release preparation

Status checked on 2026-10-15. This is the agent preparation packet for [issue #14](https://github.com/sofianbll/bifrost/issues/14), not a release approval. The accepted target remains `transports/v2.2.6`; #11 stays open / needs-info. Production, personal Codex sessions, paid providers, tags, registry publication and writes to `maximhq/bifrost` remain outside the mandate.

## Prepared source and contribution order

| Layer | Fork branch | Base | Qualified head | Draft body |
| --- | --- | --- | --- | --- |
| A: Code Mode error result | `codex/mcp-errors-prep-226` | stock `8b4fce4f1709d66f9208d02f50552da522535f9e` | `f136c93fcf78f3db3df59974ae49cfc392412f51` | [A](pr-drafts/a-code-mode-errors.md) |
| B: first-boot key attachments | `codex/mcp-firstboot-prep-226` | A | `adb80e661560174025ff9f9c23b918cb377e14b9` | [B](pr-drafts/b-first-boot-grants.md) |
| C: MCP Apps relay | `codex/mcp-apps-relay-prep-226` | B | `a369739b6c5c6cd0921bce4fa8e0b9a6ae6b6034` | [C with inline architecture diagram](pr-drafts/c-native-mcp-apps.md) |

Open contributions in dependency order only after submission is authorized and the destination base is settled. These are existing preparation refs, not newly created upstream PRs. The integrated fork source is `19362d86b2a3d17b699283c98f67861cbc6dc46c` on `codex/mcp-apps-fork-226`; it includes the already merged PRs #16–#19 and differs from the clean contribution refs by fork-only material.

The official contribution guide [targets `dev`](../contributing/raising-a-pr.mdx), while this accepted work is frozen on 2.2.6. Read-only remote inspection on 2026-10-15 advertises upstream `dev` at `77c1259f851e6c8c6b53d7963cc8a49415473dd7` and `main` at `eb830c15f0f6a3a2e6fe6be839f25d88a78f7141`. Annotated tag object `1f775f6a1e3adf727e40ff619bdeb2a83b954ddc` peels to the expected stock commit `8b4fce4f1709d66f9208d02f50552da522535f9e`. These observations are time-bound. No rebase or compatibility claim against current `dev` is implied. Retargeting/rebasing needs an explicit scope decision and fresh CI on the resulting source; Bifrost 2.3 remains deferred.

## CI evidence already complete

| Requirement | Exact evidence | Scope / result |
| --- | --- | --- |
| Integrated build and mock regression | [38715303556](https://github.com/sofianbll/bifrost/actions/runs/38715303556) | integrated source `19362d86`; AMD64/ARM64 Go/UI/plugin/image probes, 675 MCP passed / 29 skipped / zero failures/errors per architecture; publish skipped |
| Independent A/B/C build | [38715391307](https://github.com/sofianbll/bifrost/actions/runs/38715391307) | each immutable candidate builds/tests independently on AMD64; harness pin `1dcbd14d5875a14dc780f00fc2edd22cfaee66a8` |
| Direct / Virtual MCP / aggregate and Code Mode | same final integrated and clean-C campaigns | 39 wire cases / 149 assertions; strict pinned-stock nine expected red sentinels / seven passing invariants; routes/grants are fixture-scoped, not arbitrary aggregate-source selection |
| Real App protocol and reference-host UI | same final integrated and clean-C campaigns | Apps 80/80; reference host 22/22; normalizer 10/10; immediate/delayed initialization |
| Checkpoint edit/save/read and labels | same final integrated and clean-C campaigns | seven elements, reciprocal bindings, fixed 800×600 camera; original and restored geometry; deliberate +24/-20 label offset rejected |
| Controlled progressive drawing | same four Excalidraw legs in both campaigns | `progressiveInput=true`; exactly one then two labels in a controlled replay after the original complete input, for official/corrected × direct/gateway; initial lifecycle order is not established by those runs |
| Existing synthetic store and persistence | [38715406042](https://github.com/sofianbll/bifrost/actions/runs/38715406042) | stock `8b4fce4f` / fork `19362d86`; `/mcp/persistent-http`; authorized VK/JWT before/after two same-store restarts; deliberate no-grant 403 and interruption cleanup detected |

Reference host: `modelcontextprotocol/ext-apps@82221c0c8ce7661efa6771c9d461511b1650495f`. Official App: `excalidraw/excalidraw-mcp@157aa23ceb1976008aadc89eb05e3444060f09d6`. Corrected App: `sofianbll/excalidraw-mcp@4c3700f0ef25a261177fecad03284a57bf7847be`. Official restoration fails alignment in both connection modes as expected; corrected restoration passes. A green Bifrost CI therefore does not mean the pristine official Excalidraw App is fixed or bundled for users.

Tested integrated image IDs, **not published registry digests**:

- AMD64 `sha256:9009af4e2d04a279e12b12c58b5033cfff673b23ebb56aa6cc3af3c63d5df3fe`.
- ARM64 `sha256:d4cad4f975afba2bda482d51939673df9dfa84059e360576c81357894abe1b52`.
- Display version `2.2.6+sofian.19362d86b2a3`.

## Existing CI entry points for a real source change

The checked-in workflows remain the execution mechanism; no duplicate pipeline or broader live test is needed. Successful campaigns above are not redispatched just to refresh a green result.

- `.github/workflows/fork-image.yml`: push-triggered on non-`main` branches; builds/tests AMD64 and ARM64. Ordinary branch pushes do not publish. It has no `workflow_dispatch`; do not invent a manual dispatch for it.
- `.github/workflows/upstream-stack-qualification.yml`: manual dispatch or workflow-file changes on its original qualification branch; candidate and harness pins are explicit. Use it when a contribution source/pin changes, retaining candidate source separately from wrapper SHA.
- `.github/workflows/persistent-mcp-restart.yml`: manual dispatch or its scoped changes; stock source pinned to 2.2.6, fork source is the checked-out workflow SHA. Its HTTP probe has no full UI test and does not reconstruct #11's unknown store.

After a relevant source change, record the exact pushed head and actual run head/candidate pins, verify the relevant complete pipeline, then archive sanitized reports with source/image/route/checkpoint identities. A changed clean candidate needs independent qualification, and changed fork runtime needs the integrated gates. Changed docs alone do not invalidate the already observed application evidence; a newly triggered build must be reported as a distinct run.

## New CI-only preparation: initial progressive lifecycle

A primary-source review found that the older progressive replay starts after the original complete input. The current preparation therefore moves the two partial updates into an opt-in hook in the temporary reference host, after App initialization and before its original complete-input call. The actual `AppBridge.sendToolInputPartial` API is used, each intermediate render is awaited, and the host's final-input/result path is preserved.

A passive observer in the App frame must record exactly `partial → partial → final input → result`; one-/two-label screenshots are taken before the next notification is allowed. Guard self-checks reject late partials, missing partials, result-before-input and duplicate finals. Original/adapted host-source SHA-256 values are retained in the bootstrap receipt and checked by the renderer. The SDK and source pins remain unchanged; only a small, explicitly reported hook is added to the disposable host build.

This new control must pass the integrated fork CI and independent clean-C qualification before it is reported as verified. The campaigns in the table above predate the new control. No live model or native Codex behavior can be inferred from it.

## Remaining criteria and next decisions

| Original #14 criterion | Current status | Action before it can pass |
| --- | --- | --- |
| Tagged/published image, Pulsar deployment | not authorized / not done | Owner approves exact source/tag/publication/deployment and a compatible-store backup/rollback procedure; do not treat local image IDs as registry digests |
| Several days of production use | not run | Requires separately authorized rollout; owner decides observation duration and acceptance |
| Native Codex direct/Virtual/aggregate render and checkpoint edit/save | not run for final source | Choose an isolated native-host environment/automation contract or explicitly revise acceptance; the personal session remains excluded |
| Same-Codex live progressive direct/Bifrost A/B | not run | Controlled reference-host replay is already green; it is not live model input or Codex host behavior |
| Historical #11 on the existing store | open / needs-info | A real sanitized failing trace is missing; do not substitute synthetic negative controls or first-boot failures |
| Live provider harness | not run | Owner chooses whether to retain it and, if retained, authorizes bounded credentials/provider/spend in isolation; synthetic CI does not certify it |
| Current upstream rebase and three upstream submissions | prepared, not submitted | Set destination base compatible with the frozen scope, qualify any changed source, and obtain explicit permission to write upstream |
| Corrected Excalidraw availability | tested immutable source only | Decide how users obtain the corrected App or wait for upstream adoption; building a source pin in CI does not distribute it |

No remaining original human/external criterion can be marked passed by another identical CI run. Keep #14 and parent #1 open; their release/native/live criteria are not silently waived by this preparation packet.

## Evidence retention and change boundary

The earlier approved 84- and 96-file sanitized QA batches are versioned on the fork. Final newer report/capture batches and the #11 trace audit stay local, with hashes/provenance under `docs/qa/bifrost-2.2.6/`. No additional QA batch is exported by this documentation update. The original checkout, old branches, contribution refs and previous evidence are preserved.

This preparation changes maintained Markdown and CI-only host/renderer controls. It does not change gateway behavior, permissions, credentials, schema, data, candidate/App/SDK pins, release settings or the package version. The independent workflow must pin the new test harness separately from its unchanged candidates. It is exempt from the wire-visible provider-harness requirement. No application test is run locally; native Codex and production remain untouched.
