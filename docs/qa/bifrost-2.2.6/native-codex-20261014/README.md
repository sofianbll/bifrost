# Native Codex acceptance — 2026-10-14

This is the **native host verification**, separate from the automated CI
qualification in [ci-pr15-final](../ci-pr15-final/manifest.json).

The exact gateway commit, local binary version and digest, App commit and HTML
digest, route, and checkpoint are recorded in [manifest.json](manifest.json).
Only the isolated loopback gateway and synthetic scene were used.

## Observed

- The user opened the App in the Codex side panel and supplied a screenshot.
- The full Excalidraw editor and Open in Excalidraw control are visible.
- Codex, Bifrost and Excalidraw labels appear centered inside their boxes.
- The editor shows 100% zoom. This is an initial visual observation, not an
  automated bounds measurement or a before/after comparison.
- A read-only read_checkpoint call through /mcp/excalidraw returned HTTP 200.
  After the user edit, checkpoint a4de9e1013d44a30b3 contains seven full
  elements: three original boxes, three bound text elements and the added box.
  All reciprocal bindings are present; each saved label center has a zero
  delta from its container center. The original box geometry is unchanged.
- The App supplied a model-context update reporting the added rectangle. This
  is distinct from the read_checkpoint evidence proving its save.
- A fresh native create_view restored the saved checkpoint with camera
  (0, 0, 800, 600), creating checkpoint 2186fe8d0f52415193. Its saved data preserves
  the seven original elements exactly. The restored native view was observed blank; see the separate bootstrap diagnosis below.

[Initial capture](native-initial.png) retains only the App pane. The conversation,
question card, account controls and browser tabs were cropped out; drawing
pixels were not modified. Crop coordinates and hashes are in
[native-initial-receipt.json](native-initial-receipt.json).

## Remaining gate

Observe the fresh restored diagram in Codex, retaining the same panel dimensions
and 100% editor zoom, and compare the three labels and box framing against
[native-edited-before-restore.png](native-edited-before-restore.png). The full
editor uses scrollToContent on mount, so an explicit camera instruction alone
is not proof of identical fullscreen framing. Record any pan/zoom difference;
no native before/after comparison has passed yet.

The captured initial and edited views share the original box framing. The
post-edit capture and crop/hash receipt are retained separately. Saved state
and the native restore invocation are in [native-edit-save-read.json](native-edit-save-read.json)
and [native-restore-result.json](native-restore-result.json).

PR 15 remains draft. No source fix, merge, release or production change occurred.
The temporary local gateway and distinct Codex QA registration remain available
for completing this gate. The MCP Apps observation backend timeout is documented
separately in [debug/README.md](debug/README.md); its exact cause is not established.

## Blank restore and candidate fix

The restored native panel was blank. The actual saved elements rendered when
passed directly, and the App/SDK initialization replay reproduced missed eager
notifications. The fix and before/after evidence are in
[bootstrap/README.md](bootstrap/README.md). The original checkpoint is preserved.
The fixed App commit is 4c3700f0ef25a261177fecad03284a57bf7847be; its final native
render is still pending on the freshly named test registration. PR 15 stays draft.
