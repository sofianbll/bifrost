# Native observation diagnosis — 2026-10-14

This reproduces the **native panel observation timeout**, not an Excalidraw
alignment defect. The synthetic QA gateway and App remain isolated from
production. No application implementation or deployment configuration changed.

## Feedback loop and minimization

Use the supported `cua_repl` MCP Apps backend and bind the observed native view.
Load the function in [native-title-probe.js](native-title-probe.js), then run:

```javascript
await probeNativeTitle();
```

Already executed: the minimized `document.title` request was red twice in
2,022 and 2,015 ms, with `CDP operation exceeded its deadline before command
dispatch`. Removing the additional body-presence check preserves the same error.
The loop checks actual native DOM access; it does not inspect hidden application
state, change the page or stand in for the native visual acceptance gate.

## Ranked probes

The predictions were reported before testing. [Full sanitized results](probe-results.json):

| Prediction | Observed result |
| --- | --- |
| A fresh tab reference fixes a stale binding | Still red in 2,027 ms |
| The same read works in the in-app browser | Green in 31 ms on an official documentation page |
| An eight-second timeout fixes delayed preparation | Still red before dispatch in 3,086 ms |
| Snapshot/locator reads offer an alternative | Combined call timed out at 10 seconds; no partial result retained |

The in-app browser blocked a loopback control page with `ERR_BLOCKED_BY_CLIENT`.
No alternate hostname or mechanism was used to bypass that refusal. A public
official documentation page served as the control instead.

## Finding and remaining uncertainty

Confirmed: the native MCP Apps observation path cannot dispatch even a title
read, while the in-app-browser path can. This precedes evaluation of the drawing
or its label coordinates. The failure is localized to that automation boundary;
its exact internal cause is **not established**.

There is no supported repair or regression-test seam for that backend in this
Bifrost/Excalidraw checkout. Browser-use implementation internals were not read
or modified, and no unrelated computer-control mechanism was substituted.
Changing label conversion or weakening its CI assertions would not address the
observed dispatch failure.

The user subsequently provided a screenshot of the native Codex panel. It shows
the full Excalidraw editing toolbar, Open in Excalidraw and 100% zoom. All three
initial labels appear centered. The cropped, sanitized capture is retained as
[native-initial.png](../native-initial.png). This rules out a blank or unrendered
App as the explanation for the observation timeout; it does not establish the
backend's internal cause.

A read-only read_checkpoint call through the isolated gateway returned HTTP 200
and the unchanged initial shorthand scene (camera plus three labeled boxes).
The user then added a rectangle, and the App supplied its model-context update.
A subsequent read_checkpoint call verified all seven full elements, reciprocal
text bindings and zero center deltas. A fresh native restore created checkpoint
2186fe8d0f52415193, preserving the seven elements exactly plus the original camera.
Its native rendering and matching fullscreen framing are still pending. PR 15
remains draft; the observation timeout is not a demonstrated App rendering error.

The throwaway reproduction is retained in this clearly marked debug directory.
No debug logging was added to either application's source. No native repair is
claimed, and no production instrumentation, merge or release was performed.
