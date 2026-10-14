# Excalidraw checkpoint text restoration — local fix, 2026-10-13

**Cause confirmed in the Excalidraw App; local fix verified.** Bifrost's gateway
implementation and production configuration are unchanged. The public App has
not been updated by this work.

## Cause and corrected surface

The official App's `convertRawElements` passes complete saved Excalidraw
elements through `convertToExcalidrawElements` again. That API creates elements
from shorthand. Its text constructor treats center/middle input coordinates as
an anchor and subtracts measured alignment offsets. Checkpoints already contain
top-left coordinates, dimensions, font and binding metadata, so the second
conversion shifts the text and can change its measured dimensions.

The shift occurs **before SVG export**, with the same font, in a minimal case
containing one rectangle and one bound text element. The extra rectangle and
other boxes are unnecessary. Keeping the font does not remove the shift.
This rules out Bifrost serialization, SVG camera positioning and font switching
as the cause of this reproduced defect.

Primary sources:

- [App normalization and its restoration callers](https://github.com/excalidraw/excalidraw-mcp/blob/157aa23ceb1976008aadc89eb05e3444060f09d6/src/mcp-app.tsx).
- [Excalidraw 0.18.0 skeleton conversion](https://github.com/excalidraw/excalidraw/blob/v0.18.0/packages/excalidraw/data/transform.ts).
- [Text constructor and alignment offsets](https://github.com/excalidraw/excalidraw/blob/v0.18.0/packages/excalidraw/element/newElement.ts).

The [eight-line patch](excalidraw-restore.patch) changes the shared App helper:
complete elements, identified by their persisted version and ID, keep their
stored geometry, font and metadata. New shorthand still converts normally;
mixed input keeps saved elements and their relative drawing order. The same
helper serves normal rendering and both final/partial checkpoint restoration.

App base: `157aa23ceb1976008aadc89eb05e3444060f09d6`, version 0.3.2.
Local fix commit: `4770d1aa02fb4407f8f4a520eb4db34d2ffe3e99` (not pushed).
The fixed source hash in [diagnosis.json](diagnosis.json) matches that commit.
No fork image, public App deployment, issue or PR was published.

## Before/after proof

The unpatched local build's HTML is **byte-identical** to the observed public
App: SHA-256 `441650e7bc844a43511b906281d11589e8d24a34a83d657c1d7a56f907c21a63`.
The fixed HTML is
`390ad96632060f0d61a40b173797e13ca0a8ab4c52a4d885cb6bfee2bca13e2c`.

Both App builds use the same unchanged Bifrost 2.2.6 binary, source
`d3318d6be866d3e34dde424cdcb4064b90843108`, SHA-256
`6305d22b64dd5ec24e7f55f54c0eea108b65ab1b4371dd54d5c4905b1a71625a`.
Each test has a fresh loopback gateway and synthetic key, a local STDIO App and
a private checkpoint directory. Route: `/mcp/excalidraw`. Official reference
host: ext-apps 2.0.3 / `82221c0c8ce7661efa6771c9d461511b1650495f`.

| Check | Before fix | After fix |
| --- | --- | --- |
| Shared normalizer regression checks | 4/9; five preservation checks fail | 9/9 |
| Full original render/edit/save/read/restore test | Fails restored alignment | Passes, exit 0, 10 seconds |
| Saved/read scene equality | Pass | Pass |
| Fixed camera, viewport and box bounds | Pass | Pass |
| Deliberate rendered text displacement | Rejected | Rejected |
| Restored label center error | −25 to −45 x; −12.84 y | +0.66 to +0.73 x; −0.34 y |

Geometry values are scene units. The allowed center error remains 8; no test
expectation, DOM transform or camera was adjusted to obtain the pass.

The [normalizer test](../../../../tests/mcpapps/qualify-excalidraw-restore.mjs)
compiles the **actual function from the App source** with TypeScript, then runs
it with Excalidraw 0.18.0 in a real browser. It checks complete state, the minimal
bound text, repeated restores, saved font choice, new shorthand labels, mixed
inputs, order, camera pseudo-elements and a deliberate displacement. This is an
isolated App-function test; the separate full MCP test verifies the real App
build, actual pointer edit and callbacks through Bifrost.

| Evidence | File |
| --- | --- |
| Root cause, target identities and checkpoints | [Diagnosis](diagnosis.json) |
| Function before/after | [Red report](source-red-final/report.json), [green report](source-green/report.json) |
| Full unpatched App | [Red roundtrip](local-unpatched-roundtrip/render-report.json), [bad restoration capture](local-unpatched-roundtrip/excalidraw-checkpoint-after.png) |
| Full patched App | [Green roundtrip](local-patched-roundtrip/render-report.json), [correct restoration capture](local-patched-roundtrip/excalidraw-checkpoint-after.png) |
| Negative control on patched App | [Displaced text capture](local-patched-roundtrip/excalidraw-checkpoint-offset.png) |
| Synthetic saved state, including persisted version fields | [Scene](saved-scene.json) |

Green source/save/read checkpoint: `50cccfd80726401e84`.
Green restored view: `2843083f50e04485a0`. These IDs belong to the private local
test store. The scene and reports remain usable after the QA services stop.

The first repeat against the public App stopped at a save callback without a
response before the test deadline. It did not reach restoration and is not
counted as a layout result. Using the byte-identical local App removes that
remote dependency: the unchanged original test is red before the fix and green
after it. The App correction passed on its first implementation, without a
repair cycle or weakened assertion. App typechecking and full builds pass.

## Reproduce without production

Clone the official App into an isolated QA checkout and check out the base
commit above. From that App checkout, apply `excalidraw-restore.patch`, install
the locked dependencies with `pnpm install --frozen-lockfile --ignore-scripts`
and build with `pnpm run build`.

From the Bifrost checkout, with its locked `tests/e2e` dependencies installed:

```bash
PLAYWRIGHT_CHANNEL=msedge node tests/mcpapps/qualify-excalidraw-restore.mjs \
  --app-source <qa-app>/src/mcp-app.tsx \
  --scene docs/qa/bifrost-2.2.6/restore-fix/saved-scene.json \
  --out <new-qa-report-directory>
```

Omit the channel setting when Playwright Chromium is installed. Applying the
patch is unnecessary to reproduce the red baseline. For the full MCP roundtrip,
use the existing `render-reference-host.mjs --checkpoint-only` against an
isolated gateway that starts the built App with `node dist/index.js --stdio`.
Give that process its own temporary checkpoint directory; do not reuse a real
store or production connector. Private manifests and keys are not retained here.

## Qualification boundary

This is **implemented and verified locally**, not deployed. No production
Bifrost registration, permission, configuration or service changed. The public
App still has the old behavior until its corrected build is adopted. Native
Codex and the full four-App CI matrix were not run; the available native
registrations target production. Existing historical QA evidence is preserved.
Only synthetic checkpoint state, sanitized results, screenshots and a source
patch are retained; full HTML, databases, credentials and private logs are excluded.
