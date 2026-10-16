# Exact b38 stable qualification preflight

Issue #37 full qualification was explicitly authorized by the owner on 2026-10-16 in the existing private Codex Cloud task. Product repair is complete and frozen. This campaign may change fork-only orchestration, not product source. PR #38 remains draft until every mandatory gate and exact-source review passes. No merge, upstream write, provider call, production access, release, image publication, or Mac execution is authorized.

## Frozen independent sources

- Stable stock and PR base: `9237ec06672406e437b3cb33fc793ffd0403b3ae` (`transports/v2.3.0`, `codex/stable23-base`).
- Candidate: `b38fadec8d78ec64ffbefb53f650e6ef149d948e`.
- Candidate parent: `eda04739a7bf77a9be50fc958acda0f455517bba`.
- Candidate tree: `43af221324ecb85d1c4b61a56c99dd6751fc293d`.
- Original qualification driver: `b838f2e5c3d3cf2340b7fee6c0cfcf2ec1ad0efb`.
- This driver descends directly from that original driver on `codex/stable23-qualify-b38`. Its exact SHA is recorded independently as `DRIVER_SOURCE` by the workflow; it is never merged into the product.
- Actual host: ext-apps2.0.3 at `82221c0c8ce7661efa6771c9d461511b1650495f`.

The clean existing private Cloud runtime orchestrates this campaign. Its constrained disk is not used for image builds or the qualification runtime; the existing fork's GitHub Actions infrastructure runs the complete frozen-source campaign. All previous worktrees, cache backing, and failed evidence are preserved.

## Minimal driver adaptation, cycle 1 of at most 2

The workflow pins the approved candidate/tree/parent, triggers only the new driver branch, and publishes status against the candidate SHA. The focused handler inventory additionally requires both new catalog synchronization regression/control tests. Existing source ancestry, local-module provenance, race detection, nonempty inventories, explicit skip accounting, paired strict stock/candidate assertions, same-store lifecycle controls, and actual host gates remain required.

The original candidate readiness implementation remains unchanged: no sleep, retry, route substitution, or assertion relaxation. After a failure only, bounded read-only observations retain timestamped client discovery state and exact route tool names/counts/errors before container disposal. They never replace the failing observation or produce a passing verdict. A permanently empty catalog self-control confirms empty observations remain empty and preserve the original failure context.

The review provenance adapter carries the independently authored stable-to-b38 reviews verbatim from [immutable repair evidence](https://github.com/sofianbll/bifrost/tree/99b3338c5f58da22e017f342415601e13ebd640e/docs/agents/stable23-repair37). Standards has **0 actionable findings and 1 accepted nonblocking lookup observation**, Spec has **0 findings**. The verifier retains total counts, requires internally consistent split counts, rejects actionable findings, and verifies the verbatim review checksum. This is not a new product review or a claim of zero total Standards observations.

The single Actions artifact includes only this campaign's runtime/Go evidence and current review/preflight documents. A per-file SHA-256 manifest complements the immutable ZIP digest. Old Cloud reports remain preserved in their original history and evidence branches, rather than being presented as current qualification.

## Preflight controls

- `bash .github/workflows/scripts/test-mcp-apps-fixture.sh --self-check`
- `python3 tests/mcpapps/qualify-go-tests.py --self-check`
- `python3 tests/mcpapps/verify-stable-review-provenance.py --self-check`
- Existing workspace/image provenance self-checks; inspection of unchanged candidate-status targeting.
- `actionlint .github/workflows/stable-23-qualification.yml`
- `git diff --check` and inspection of the fork-only delta against the original driver.

No qualification result is claimed by this preflight document. Counts, failures, skips, source/module/build identities, actual host results, and final verdict must come from the new Actions artifact.

## Historical limits retained

The [old e33 full qualification failure](https://github.com/sofianbll/bifrost/actions/runs/38938846259) remains source-specific. Diagnostic orchestration `2474aa7bc08c602d792dcc02b902ef5c3241f83a` and data-only repair evidence `99b3338c5f58da22e017f342415601e13ebd640e` are not merged into product or driver. The mutex fixes a demonstrated overlapping-publication defect; its role in the historical startup failure remains unestablished. Controlled replay cannot establish native Codex rendering or live-model streaming. Inherited repository checks are separate from the approved compatibility contract.
