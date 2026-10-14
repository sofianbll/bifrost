# Code review and final checkpoint fix — 2026-10-13

The two changes were reviewed separately against fixed bases, with independent
Standards and Spec reviewers. One functional issue in the initial App fix was
reproduced and repaired before opening the PRs. Production was not accessed.

## Standards

**Excalidraw: zero documented violations and zero substantiated design smells.**
Sources: App README.md and CLAUDE.md. The shared helper retains the existing
interface and converter; no dependencies, storage or deployment settings change.
CLAUDE.md's historical claim that skeleton conversion is absent predates this
diff and conflicts with the actual base code. The initial publication limitation
(tests only available in Bifrost) is resolved by the App's new `npm test`, using
Node's test runner and its existing TypeScript dependency. It extracts the real
helper and instruments the converter; this unit check does not claim browser QA.

**Bifrost: zero documented violations and zero substantiated design smells.**
Sources: AGENTS.md, README.md, docs/agents/issue-tracker.md and domain.md. The
existing checkpoint test is strengthened without a new dependency or Go/config
change. Public artifacts contain only synthetic state and sanitized observations.
Provider-harness coverage is exempt because this fork diff changes tests and
documentation, without a gateway wire-visible behavior change.

## Spec

**Excalidraw: one resolved finding; zero remaining blockers.** The initial
`saved.get(el.id)` replacement discarded newly generated inverse arrow bindings
in mixed checkpoint/shorthand input. Reproduction: saved boxes plus a new arrow
with `start`/`end` references. The first fix left `startBinding`/`endBinding` on
the arrow but lost its entry in the boxes' `boundElements`. The repaired helper
retains new bindings without changing saved text/box geometry. The independent
reviewer reran both unit and real-library checks and confirmed the repair.

**Bifrost: zero missing or incorrect requested requirements.** Three bound
labels, actual SVG alignment before/after, constant framing, pointer edit and
exact save/read, deliberate displacement rejection, identities/routes/checkpoints
and separate native-Codex status are present. The additional mixed-arrow check
pins the regression found during review. Historical evidence is preserved.

## Final validation

| Check | Before | Reviewed fix |
| --- | --- | --- |
| App `npm test` on actual helper with converter spy | Official base 2/7; first fix 6/7 on arrow regression | 7/7 |
| Actual Excalidraw 0.18.0 browser checks | Initial fix 9/10, new arrow check red | 10/10 |
| Full actual App through unchanged Bifrost | Original restoration red, retained in parent evidence | 1/1, 9 seconds |
| Intentional rendered label displacement | Rejected | Rejected |
| App typecheck/build | Passed on prior fix | Passed on reviewed fix |

The reviewed App commit is `61bb966aa2ceee485aaa3badd91807c2edc6f236`. Its source and HTML hashes,
checks and evidence hashes are in [review.json](review.json). The function report
was recorded on the pre-commit working tree; its source hash matches that commit.
Bifrost remains the unchanged `d3318d6be866` binary identified by the full
[render report](roundtrip/render-report.json).

The real-library [red](review-arrow-red/report.json) and
[green](review-arrow-green/report.json) reports test the actual shared App helper.
The [roundtrip report](roundtrip/excalidraw-checkpoint-report.json) contains its
route, checkpoint IDs, geometry, exact readback and negative control. The
[before](roundtrip/excalidraw-checkpoint-before.png),
[restored](roundtrip/excalidraw-checkpoint-after.png) and
[deliberately shifted](roundtrip/excalidraw-checkpoint-offset.png) captures were
visually inspected. All have the same 1400×1050 crop and camera.

The [reviewed source patch](excalidraw-reviewed-source.patch) replaces the initial
source patch for reproducing the final fix; apply it to the official base using
`git apply --unidiff-zero`. The standalone App test is included in the reviewed
App commit. Use the parent report's local reproduction instructions for the
actual-browser runner; it now has ten checks and requires two saved boxes.

## Publication and qualification boundary

The App PR contains the source fix and standalone test. The Bifrost fork PR
retains automated checks and sanitized evidence against its 2.2.6 integration
branch. Keep the Bifrost PR in draft: its existing full CI still uses the public,
unpatched App, so the stronger visual gate can remain red until a corrected App
is adopted. No assertion is weakened and no gateway workaround is introduced.
Native Codex and the full four-App matrix remain unqualified. No image tag,
release, production config, authentication or production store was modified.
