# Native Codex acceptance — 2026-10-14

**Passed on the corrected App.** The [final native verification](final/README.md) records the loaded build digest, native captures and independent pixel comparison. It is distinct from [automated CI qualification](../ci-pr15-bootstrap/manifest.json). The [manifest](manifest.json) retains exact gateway/App identities, route, checkpoints and the earlier failure.

## Real editor save and restore

The user opened the original scene `a4de9e1013d44a30b3` in Codex and added a rectangle. The App model-context update described the edit; a separate read_checkpoint call proved its save. Seven full elements were persisted: the three original boxes, their reciprocal bound texts and the user rectangle. Saved text centers have zero deltas from their containers, and the original box geometry did not change.

The [initial capture](native-initial.png) and [edited capture](native-edited-before-restore.png) were cropped to the App pane without changing drawing pixels. Their receipts record source hashes and crop coordinates. These captures document the real edit, not the final constant-frame comparison.

## Blank-view failure and correction

The first fresh native restore, `2186fe8d0f52415193`, preserved the saved elements but displayed a [blank panel](bootstrap/native-restored-white.png). The [initialization diagnosis](bootstrap/README.md) reproduced eager notification loss in the actual App/SDK. The pre-connect handler fix is at Excalidraw commit `4c3700f0ef25a261177fecad03284a57bf7847be`. Earlier native observation timeouts are retained as historical diagnostics in [debug/README.md](debug/README.md).

## Final native evidence

After reloading the test connection, the final restore `1a94a805c1a44209bf` displayed four rectangles and three centered texts. The loaded module matches the corrected build. The direct same-scene control `b3c6eb56c5244b9782` and restored view have identical native pixels at 766×923 and 100% zoom. The control was captured after the restored view; the [report](final/native-pixel-comparison.json) explicitly records this order and the full editor's automatic recentering.

The [native restored capture](final/restored-native.jpg), [control](final/direct-native-control.jpg), [capture receipt](final/capture-receipt.json) and [checkpoint readback](final/checkpoint-data-check.json) establish the native gate. The rendered deliberate-offset rejection remains separately automated. No production service, production configuration, user checkpoint, merge, release or public App deployment was changed.
