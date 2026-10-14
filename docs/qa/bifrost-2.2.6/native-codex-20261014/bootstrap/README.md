# Blank restored native view — initialization diagnosis

The user supplied a screenshot of a blank native restored pane. The saved edit
and three reciprocal text bindings were intact. A direct native create_view
containing those same saved elements rendered four rectangles and three texts.
A tagged temporary read trace showed that the first blank view did not even call
read_checkpoint. A repeated view did read seven elements and mount the editor.

The actual App/SDK HTML was replayed through a minimal loopback initialization
host. With eager notifications, the old App has no SVG and never calls
read_checkpoint; delaying those notifications by 100 ms makes the same input
render. The source registered the handlers only after useApp completed connect
and the React core mounted. The fix installs temporary handlers in onAppCreated,
keeps only the latest input and result, then replays them through existing core
handlers. The dev core caller remains compatible.

The standalone regression also fails on the byte-verified pristine 61bb966
source and passes after the fix. Eight Node checks, typechecking, the complete
build and both initialization timings pass. The automated reproducer is now
[qualify-excalidraw-initialization.mjs](../../../../../tests/mcpapps/qualify-excalidraw-initialization.mjs).
It exercises the actual built App and SDK with a fixture callback, independently
of native Codex qualification. It cannot replace the native acceptance gate.

[diagnosis.json](diagnosis.json) records commits, HTML digest, gateway version,
route, checkpoints, ranked probes, scope and limits. The original native event
timing was not captured, so the report distinguishes the reproduced App defect
from its final confirmation in Codex. [The blank capture](native-restored-white.png)
retains only the App pane; chat, account controls, desktop and menu bar were
removed without modifying drawing pixels. Its receipt records crop and hashes.

A second diagnostic HTML was served by the gateway but Codex kept the first
trace script. The final QA reloaded the test connection and verified the loaded script digest
in the native panel. All diagnostic markers are removed from the committed source
and rebuilt HTML. No production service, merge, release or user checkpoint was
changed. The [final native verification](../final/README.md) now records the corrected build and passing same-scene framing comparison.
