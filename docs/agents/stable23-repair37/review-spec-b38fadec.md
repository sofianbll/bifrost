# Spec review — stable23-integration

Review axis: Spec. Exact base: `9237ec06672406e437b3cb33fc793ffd0403b3ae`. Exact HEAD: `b38fadec8d78ec64ffbefb53f650e6ef149d948e`. Both refs resolve. The three-dot diff is nonempty: 42 files, 4,046 insertions and 130 deletions. Commits after base: `e949886eb`, `e33b67ab6`, `eda04739a`, `b38fadec8`.

Findings: 0.

The narrow issue-37 override requires a “per-handler mutex around complete SyncMCPServer capture/build/store.” The product repair still holds that mutex across manager capture, server/snapshot build, and atomic publication; request readers continue serving an immutable atomic snapshot without taking the refresh lock. The only delta since the prior review is a `started` channel acknowledgement in the overlapping-refresh test before its 100 ms negative capture window. The first refresh remains barrier-held while the second is started; the test then releases and joins both workers and checks the final public tools list and callback alias. Serial addition, narrowing/removal, and empty-catalog controls remain.

The acknowledgement improves scheduling evidence but cannot prove the worker entered `SyncMCPServer` before the window; the timer remains a bounded scheduler control, not proof of internal lock state. No product or callback behavior changed in this commit, and I found no spec finding from that limitation. Full qualification remains pending; this code review is not a qualification pass. No tests were run for this review.

Worst issue within Spec: none.
