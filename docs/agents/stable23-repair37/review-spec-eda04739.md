# Spec review — stable23-integration

Review axis: Spec. Exact base: `9237ec06672406e437b3cb33fc793ffd0403b3ae`. Exact HEAD: `eda04739a7bf77a9be50fc958acda0f455517bba`. The base and HEAD refs resolve; `git diff 9237ec06672406e437b3cb33fc793ffd0403b3ae...HEAD` is nonempty (42 files; commits `e949886eb` and `e33b67ab6` plus the bounded repair `eda04739a`). The new repair is limited to `mcpserver.go`, its existing handler test file, and the transport changelog.

Findings: 0.

The issue-37 override asks for a “per-handler mutex around complete SyncMCPServer capture/build/store.” The candidate holds `syncMu` over manager capture, fresh server/snapshot construction, and atomic publication. Requests continue to load the immutable published server and do not take this lock. I found no changed authorization, admission, visibility, hook, source-isolation, cache, or lifecycle behavior in the repair. The inspected callers release manager/config mutation locks before sync; capture/build does not execute tool handlers or reenter sync, consistent with the recorded lock-order assessment.

The regression drives the existing injected-manager seam, holds the first refresh in capture, checks the old public HTTP `tools/list` snapshot, updates to a catalog with a modern tool, and checks that a second capture does not enter until release. After both refreshes join, it checks the complete public tool list and invokes the modern callback alias. Serial coverage checks addition, narrowing/removal, and empty catalogs. No issue-37 wire controls are represented as newly qualified here; the mandate explicitly withholds full qualification, and the recorded historical startup cause remains unestablished.

Worst issue within Spec: none. This is a code/spec review result, not a qualification pass.
