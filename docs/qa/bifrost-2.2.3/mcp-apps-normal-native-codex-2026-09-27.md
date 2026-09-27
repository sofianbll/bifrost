# Normal-mode native Codex MCP Apps check — 2026-09-27

**Updated result: native tool invocation and visible native App rendering verified on all three connectors. In-place editing and UI save remain unverified.**

## Scope and provenance

- Conversation: `01a0e2e1-5251-7a40-855e-904ea13b2927`.
- Checkout observed: branch `feature/mcp-apps-native`, HEAD `c3f711b49`; existing unrelated changes were preserved.
- Authorized setup supplied with the task: local adapter at `127.0.0.1:18080`, candidate at `127.0.0.1:18083`, two Excalidraw sources, `is_code_mode_client=false`, no gateway LLM providers. Runtime binary identity and configuration were not independently re-read in this check.
- Supplied routing: `qa-normal-direct` `/qa/direct` → `/mcp/excalidraw`; `qa-normal-virtual` `/qa/virtual` → `/mcp/codexmcp`; `qa-normal-combined` `/qa/combined` → `/mcp`. Private VK values were neither read nor recorded.
- Calls used the session's registered native MCP tools through `functions.exec`. This is host tool orchestration, not Bifrost `executeToolCode`. No HTTP substitute, reference host, remote connector, or LLM provider was called.

## Exposed tools and calls

The current session tool registry exposed nine matching tools:

- Direct: `excalidraw_read_me`, `excalidraw_create_view`.
- Virtual: `excalidraw_read_me`, `second_read_me`, `second_create_view`.
- Combined: `excalidraw_read_me`, `excalidraw_create_view`, `second_read_me`, `second_create_view`.

The registry did not expose `qa-normal-virtual`'s `excalidraw_create_view`. The Virtual test therefore used the exposed `second` source. This inventory describes the agent-visible registry, not a raw `tools/list` capture or proof of the reason for that asymmetry.

Each row ran its matching `read_me` first and received the format reference, then called `create_view` once with a camera and one labeled rectangle (400 × 110 at x=100, y=170; font size 24).

| Connector | Native tool prefix | Rectangle label | Returned `structuredContent.checkpointId` |
|---|---|---|---|
| Direct | `mcp__qa_normal_direct__excalidraw_` | Codex natif — direct | `0fa3cc67dc204c8487` |
| Virtual MCP | `mcp__qa_normal_virtual__second_` | Codex natif — Virtual MCP | `78d7a51dc4e14cd69e` |
| Combined VK | `mcp__qa_normal_combined__excalidraw_` | Codex natif — VK combinée | `1d2e33d087ed442db3` |

All three `create_view` results contained exactly the top-level keys `content` and `structuredContent`. Each contained one text block saying `Diagram displayed!` with its checkpoint ID and editing instructions. `structuredContent` contained the matching `checkpointId`. No exception or error result was returned; `isError` was absent, not explicitly false.

## UI evidence and limits

| Evidence level | Direct | Virtual MCP | Combined VK |
|---|---|---|---|
| Native tool call completed | Verified | Verified | Verified |
| Structured checkpoint payload received | Verified | Verified | Verified |
| UI metadata / HTML resource received by host | Not established | Not established | Not established |
| App visibly rendered in this conversation | Verified by user screenshot | Verified by user screenshot | Verified by user screenshot |
| Editing / saving observed | Not observed | Not observed | Not observed |

The agent-visible results had no `_meta` field, resource link, embedded HTML, or image. The registry provided tool names, descriptions and argument schemas, but did not expose raw tool-definition UI metadata such as `_meta.ui.resourceUri`. Absence from these visible surfaces does not prove that the gateway omitted metadata or that the host did not receive a UI resource. A checkpoint response alone does not establish rendering or saved user edits.

Native UI inspection was attempted with `cua.getApp("Codex")`. Computer Use rejected access: `Computer Use is not allowed to use the app 'com.openai.codex' for safety reasons.` No screenshot or accessibility state of this conversation was obtained by that automated attempt. A user-supplied screenshot subsequently established rendering, as recorded below. The restriction was not bypassed. The voice-only screen-context tool was inapplicable in this text conversation. No basic-host image was used as native evidence.

## Outcome

The three connectors are callable in this fresh Codex conversation and preserve structured checkpoint results in normal mode. The user subsequently confirmed rendering and supplied a screenshot showing all three native Apps. In-place editing and UI save remain separate acceptance gaps; this does not establish full UI compatibility.

Only this report was added. No configuration, service, production instance, or unrelated repository file was changed; no other chat was created or contacted. No source change was made, so compilation and regression suites were not run. Verification consisted of native MCP calls, result-field inspection, the attempted UI observation, and report readback.

## User confirmation and follow-up conversation review

The user supplied [this unmodified screenshot](mcp-apps-normal-native-user-confirmation-2026-09-27.png) and confirmed that it works. It visibly shows the Direct, Virtual and Combined native integration cards with their corresponding blue, green and purple labeled rectangles. Native rendering is now established for these three normal-mode cases. The screenshot does not establish UI editing, save, or partial-input rendering.

Reading the same live conversation revealed a separate unmet request:

1. The agent created a water-cycle diagram with one `create_view` call containing the whole ordered element array. Its claim of progressive appearance was not supported by a captured timeline or partial-input events.
2. Asked to move the sun to the center, it called `create_view` again, with `restoreCheckpoint` (`9b01d09cdd454ff9a7`), deletion of `sun,sun-label`, and replacement elements. The user reported that this opened another view rather than changing the already open one.
3. Asked explicitly to modify that exact open view, the agent acknowledged the limitation and made no further call. The earlier success wording overstated fulfillment of the in-place request.

The upstream `read_me`, fetched again through the local direct route, describes `restoreCheckpoint` as loading saved diagram state and appending elements in a `create_view` call. This establishes content reuse, not reuse of the existing host view. Existing protocol observations also show `read_checkpoint` and `save_checkpoint` with `_meta.ui.visibility: ["app"]`: these callbacks exist for the App, although they are absent from the model-visible tool registry. Their absence from that registry is not evidence that Bifrost lost them. Whether an existing Codex view can receive and display a later model-requested edit requires separate upstream/host investigation; no cause or fix is asserted here.

No diagram was recreated or changed during this review. The local test services/connectors were retained because the referenced conversation is still being used for the user's editing investigation. Their cleanup remains outstanding.
