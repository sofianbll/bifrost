# Bifrost 2.2.6 checkpoint alignment — 2026-10-13

This page records the original detection campaign. The subsequent
[root-cause diagnosis and local App fix](restore-fix/README.md) is now verified:
the same full checkpoint test passes against the corrected Excalidraw App.
The public App and production Bifrost remain unchanged.

The deployment retrospective's points 1 and 2 are implemented: the existing
checkpoint test checks bound-label geometry, and its sanitized evidence is
stored here. **The control works; the restored App fails alignment.** This is a
local qualification result, not a production or native Codex pass.

## Tested target and preserved work

- Gateway commit: `d3318d6be866d3e34dde424cdcb4064b90843108`, the qualified
  `codex/mcp-apps-fork-226` starting point, based on Bifrost 2.2.6.
- Native binary: `v2.2.6-checkpoint-qa.d3318d6be866`; SHA-256
  `6305d22b64dd5ec24e7f55f54c0eea108b65ab1b4371dd54d5c4905b1a71625a`.
  Go build information confirms that revision and `vcs.modified=false`;
  Go 1.27.2, Darwin/ARM64. No container image was tested.
- Route: `/mcp/excalidraw` on a fresh authenticated loopback gateway, one
  synthetic key, no providers and a new SQLite store. The management UI was a
  disposable embed placeholder and was not tested.
- Official reference host: `modelcontextprotocol/ext-apps` 2.0.3,
  commit `82221c0c8ce7661efa6771c9d461511b1650495f`.
- Browser: Playwright 1.57.0, installed Edge `155.0.4283.45`.
- Excalidraw: the existing test's public MCP endpoint. The observed resource
  HTML hash is recorded; this endpoint and its ESM dependencies are not pinned.

Work is on `codex/checkpoint-label-alignment-226` in a separate managed worktree.
The original `feature/mcp-apps-native` checkout at
`71174d96ae4763e1167f9acdb636df20e704bb41`, its local commit, `.agents/` and
untracked context note were preserved. Only test code, CI evidence metadata and
documentation changed. No gateway implementation, production service, store,
configuration or connector registration changed.

## Automated result

[`renderCheckpoint`](../../../tests/mcpapps/render-reference-host.mjs) now uses
three colored boxes with actual bound labels. It measures rendered SVG text and
container bounds independently of stored text coordinates. Center error and
containment allow 8 scene units for font bearings and hand-drawn borders.
The test also checks `containerId`/`boundElements`, successful callbacks, an
actual new rectangle from a pointer edit and exact saved/read scene equality.

The camera is `x=0, y=0, 800×600` in both views. Their SVG viewBox is
`-50 -70 800 600` and viewport `1400×1050`. Every measured box bound is
identical before/after; the framing assertion allows at most 2 scene units.
No DOM camera or text correction was applied to the restored result.

| Label | Before center error (x, y) | After restoration (x, y) | Result |
| --- | --- | --- | --- |
| Codex | +0.66, −0.34 | −25.45, −12.84 | Fail |
| Bifrost | +0.66, −0.34 | −27.12, −12.84 | Fail |
| Excalidraw | +0.73, −0.34 | −44.80, −12.84 | Fail |

Values are scene units. Saved and read payloads share SHA-256
`cd31dcf81d3731b3d523b8ed17291b542a3087e9ce4fa757ffa50c795f443079`.
Persistence passes while rendering/restoration fails; this does not identify
which App, host or gateway stage causes the defect.

The negative control translates only the **rendered** Bifrost text by `+24, −20`.
The same assertion rejects its measured error of `+24.66, −20.34`, with unchanged
camera/boxes. The transform is removed, and initial alignment passes again
before the edit/save. No checkpoint is changed by the negative control.

Source/save/read checkpoint: `c400b57141a74fcda0`.
Fresh restoration view: `cf44d1b953044d839a`. These public App IDs identify the
observed sessions; their future availability is not guaranteed.

| Evidence | File |
| --- | --- |
| Initial aligned scene | [Before capture](excalidraw-checkpoint-before.png) |
| Deliberate displacement | [Negative-control capture](excalidraw-checkpoint-offset.png) |
| Failed restoration | [After capture](excalidraw-checkpoint-after.png) |
| Geometry, checks and checkpoints | [Checkpoint report](excalidraw-checkpoint-report.json) |
| Target, renderer hash and failing exit | [Render report](render-report.json), [manifest](manifest.json) |
| Sanitized synthetic scene | [Saved scene](excalidraw-checkpoint-scene.json) |
| Redacted callback/host events | [Console report](excalidraw-checkpoint.console.json) |
| Passing script checks | [Self-checks](self-checks.json) |
| Legacy false visual pass | [Baseline report](baseline/report.json), [restored capture](baseline/excalidraw-checkpoint-after.png) |

The legacy checkpoint function passed despite its visibly displaced/clipped
label. Its temporary harness was narrowed to that function and installed browser;
its assertions were unchanged. The first enhanced attempt stopped at a nested
frame initialization race. One repair added the visible-SVG wait; the next run
reached the actual alignment failure. Capture checks then exposed one-pixel
locator rounding; a fixed document crop produces three complete `1400×1050` PNGs.
Two bounded screenshot repairs were needed (fixed crop, then full-document capture).
The final run still detects the alignment failure. No App correction
or weaker expectation was introduced.

The final renderer exits **1** for the observed regression. Both script
self-checks, JS/Python syntax and `git diff --check` pass. CI runs this checkpoint
by default, so shifted restored labels now fail its render gate. The CI manifest
also retains image ID and available OCI commit/version/digest; missing identity
fields are omitted rather than invented.

## Reproduce the focused control

Use the existing private qualification manifest for an **isolated local or CI
gateway**, generated by `tests/mcpapps/ci.py`. It contains credentials and stays
outside this evidence directory. This campaign used a native build rather than
the Docker CI launcher. Prepare the pinned host and locked test dependencies:

```bash
node tests/mcpapps/bootstrap-reference-host.mjs --directory <empty-qa-host-directory>
npm ci --prefix tests/e2e --ignore-scripts
node tests/mcpapps/render-reference-host.mjs --self-check
python3 tests/mcpapps/ci.py --self-check
PLAYWRIGHT_CHANNEL=msedge node tests/mcpapps/render-reference-host.mjs \
  --manifest <private-local-qualification-manifest.json> \
  --host-dir <qa-host-directory> \
  --out <new-qa-output-directory> --checkpoint-only --timeout-ms 30000
```

Omit `PLAYWRIGHT_CHANNEL` when Playwright Chromium is installed. Without
`--checkpoint-only`, the existing App/route matrix and connection check remain
enabled. A completed round trip with `afterAlignment=false` is a detected defect,
not a setup failure. Full App matrix, Docker/image CI and provider harness were
not run for this test-only change.

## Native Codex gate and evidence hygiene

[The separate native record](native-codex.json) is **not_run**. The available
Codex Bifrost registrations target production, which this task excludes.
Reading their local URLs did not invoke those endpoints. No native MCP App
capture or pass is claimed; viewing these screenshots in Codex does not change
their classification as automated reference-host evidence.

The native gate needs an authorized local/QA registration: create this synthetic
scene, observe its labels, perform an edit/save and restore in a fresh App view
with the same camera. Record runtime identity, route, checkpoint IDs and App-only
captures separately. Preserve existing user checkpoints; a browser pane is not
native MCP App verification.

These files contain only synthetic content and sanitized results. Credentials,
full App HTML, gateway logs, SQLite stores and backups are excluded. Saved scene
fields exclude files, links and unrelated metadata. All retained PNGs were
visually inspected and contain only the QA diagram.
