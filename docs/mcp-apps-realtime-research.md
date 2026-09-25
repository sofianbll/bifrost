# MCP Apps: what makes Excalidraw draw progressively

Research checked on 2026-09-24 against the MCP specification, MCP Apps SDK, and Excalidraw source. This note separates documented behavior from what still needs a live test.

## The two streams

| Flow | What it carries | Relevant to progressive drawing? |
| --- | --- | --- |
| Model → MCP Apps host → iframe | Partial `create_view` arguments via `ui/notifications/tool-input-partial` | **Yes.** Excalidraw handles these in `ontoolinputpartial`, parses complete elements from the growing `elements` string, and redraws as the element count changes. |
| MCP server → MCP client | JSON-RPC responses and notifications over Streamable HTTP, optionally encoded as SSE | **Indirectly.** This stream can carry server messages during a tool call. It does not generate the model's partial tool arguments or the host-to-iframe notification. |

The host sends final arguments through `ui/notifications/tool-input`; Excalidraw renders the stable final state there. The MCP Apps SDK explicitly calls partial input a host-to-view preview and warns that partial JSON may be incomplete. [MCP Apps patterns](https://apps.extensions.modelcontextprotocol.io/api/documents/Patterns.html#lowering-perceived-latency), [App API](https://apps.extensions.modelcontextprotocol.io/api/classes/app.App.html), [Excalidraw rendering pipeline](https://github.com/excalidraw/excalidraw-mcp/blob/main/CLAUDE.md#rendering-pipeline)

Streamable HTTP already permits a `POST` response to be either `application/json` or `text/event-stream`; the client must accept both. Legacy HTTP+SSE is a separate, deprecated transport. Merely selecting SSE therefore cannot establish that a host forwards partial *model-generated input* to an MCP App. This is an architectural inference from the separate directions of the two protocols, not a substitute for testing the actual Bifrost settings. [MCP transport specification](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)

## What the official `basic-host` test proves

The [official testing guide](https://apps.extensions.modelcontextprotocol.io/api/documents/testing-mcp-apps.html) says to run `basic-host`, select a UI tool, enter JSON input, and click **Call Tool**. This tests discovery, `resources/read`, iframe rendering, and final tool input/result. Its [implementation](https://github.com/modelcontextprotocol/ext-apps/blob/main/examples/basic-host/src/implementation.ts) calls `sendToolInput({ arguments: input })` once after the iframe initializes; it does **not** call `sendToolInputPartial`. Thus a successful basic-host test alone does **not** prove live drawing while a model generates arguments. The host tries Streamable HTTP first and falls back to legacy SSE, so its console log can confirm which transport connected.

Basic-host's transport constructors do not set custom authorization headers in that implementation. For a VK-protected local Bifrost endpoint, use an authorized test route/adapter or a host that can send the VK; do not paste the key into an untrusted public URL. [Basic-host implementation](https://github.com/modelcontextprotocol/ext-apps/blob/main/examples/basic-host/src/implementation.ts)

## Minimal test sequence

1. **Server/UI baseline:** Run official `basic-host` against the Bifrost MCP endpoint as in the testing guide. Verify `create_view` is listed with UI metadata, its `ui://` resource loads with `text/html;profile=mcp-app`, the iframe renders, and tool result arrives. Record `[HOST] Connected via ...` in the browser console. This tests final rendering only.
2. **Widget partial-input baseline:** In Excalidraw's own development UI (`npm run dev:ui`, where dependencies are already installed), use its `streamElements` control. The [mock implementation](https://github.com/excalidraw/excalidraw-mcp/blob/main/src/dev-mock.ts) sends one partial element array every 120 ms by default, then final input. This isolates whether the Excalidraw widget can animate independent of Bifrost and the model. It is synthetic, not a live host test.
3. **Actual conversational host:** Connect Excalidraw directly and through Bifrost to the **same** MCP Apps-capable host/model, ask for the same multi-element diagram, and capture when the first shape appears relative to the final `tools/call`. Observe or instrument host-to-iframe `ui/notifications/tool-input-partial` events, plus final `tool-input` and `tool-result`. Compare both connections before attributing the difference to Bifrost.
4. **Bifrost setting A/B:** On the same host/model, compare Bifrost upstream Streamable HTTP vs legacy SSE if Excalidraw supports both; compare Code Mode off/on separately. Record tool discovery metadata and partial-event count, not just whether the final diagram appears. Excalidraw documents its HTTP server as Streamable HTTP by default, so an SSE-only endpoint may be unavailable. [Excalidraw run modes](https://github.com/excalidraw/excalidraw-mcp/blob/main/CLAUDE.md#running)

**Decision rule:** If direct and proxied connections both receive zero partial-input events, investigate host/model support. If direct receives partials and proxied does not, inspect the gateway's tool registration/host integration. If both receive partials but Bifrost draws only at the end, inspect argument shape, iframe timing, and Excalidraw's parser. Excalidraw itself recommends checking whether `ontoolinputpartial` fires and whether `elements` is under the expected argument path. [Excalidraw debugging notes](https://github.com/excalidraw/excalidraw-mcp/blob/main/CLAUDE.md#common-issues)

## Local gateway measurements

Tests on 2026-09-24 used the patched Bifrost binary and Excalidraw at `https://mcp.excalidraw.com/mcp`. The first A/B used isolated temporary Bifrost configurations.

| Variant | `tools/list` | `create_view` | `resources/read` |
| --- | --- | --- | --- |
| HTTP Streamable, `needs_session_stickiness=false` | 9 tools, UI metadata present | HTTP 200, structured `checkpointId` | HTTP 200, `text/html;profile=mcp-app` |
| HTTP Streamable, `needs_session_stickiness=true` | 9 tools, UI metadata present | HTTP 200, structured `checkpointId` | HTTP 200, `text/html;profile=mcp-app` |

The dashboard's **Maintain Persistent Connection** switch is `needs_session_stickiness`. Bifrost's source says it selects a shared reusable HTTP connection instead of connecting for each call; it does not control host-to-iframe partial input. [Configuration field](../core/schemas/mcp.go), [dashboard switch](../ui/app/workspace/mcp-registry/views/mcpClientSheet.tsx), [connection manager](../core/mcp/clientmanager.go)

With `Accept: text/event-stream`, the gateway's `GET /mcp/excalidraw` returned HTTP 200 `text/event-stream`, while a `POST tools/list` returned HTTP 200 `application/json`. Direct `GET https://mcp.excalidraw.com/mcp` returned HTTP 405. Bifrost's legacy SSE client pointed at that `/mcp` URL failed startup with HTTP 405; a direct `GET /sse` timed out after 8 seconds without headers, so `/sse` support remains unconfirmed. These observations show transport behavior, not widget partial-input delivery.

**Still open:** No same-host direct-versus-Bifrost A/B captured `ui/notifications/tool-input-partial`, so the cause of Codex showing the whole diagram at once is not established. The official `basic-host` cannot close this gap because it sends only complete tool input.

For a visual Codex trial, the local Bifrost on port 18766 was then switched to `needs_session_stickiness=true`. Its configured Codex URL and auth returned HTTP 200 for `initialize`, `tools/list` (9 tools), `create_view` (structured checkpoint), and `resources/read` (HTML). The switch initially produced `403 access_denied` after restart against the existing temporary SQLite database; a fresh temporary database restored access. The cause of that state-dependent 403 is unconfirmed. The user-facing visual comparison is pending.
