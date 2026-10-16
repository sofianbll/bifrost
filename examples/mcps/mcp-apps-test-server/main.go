package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const resourceURI = "ui://mcp-apps-test-server/view.html"
const maxSyntheticStateBytes = 256

// The pinned reference-host bootstrap builds the SDK entry in
// tests/mcpapps/fixture-app.ts with the official ext-apps 2.0.3 package, then
// serves it from its existing static directory as /mcp-app-fixture.js. Keeping
// that generated bundle outside the product tree avoids checking in a vendored
// SDK while still exercising the real App handshake and App.callServerTool.
const fixtureViewHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src http://localhost:8080; style-src 'unsafe-inline'; connect-src 'none'; img-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Bifrost MCP Apps fixture</title>
  </head>
  <body>
    <main data-testid="mcp-app-fixture" data-status="loading" data-notification-order="[]" data-save-tool="SAVE_STATE_TOOL" data-read-tool="READ_STATE_TOOL">
      <h1>MCP Apps fixture</h1>
      <output data-testid="fixture-status" aria-live="polite">Waiting for host</output>
      <pre data-testid="tool-partial-input">No partial input received</pre>
      <pre data-testid="tool-input">No input received</pre>
      <pre data-testid="tool-result">No result received</pre>
      <label>Saved state <input data-testid="state-input" value="first-state"></label>
      <button data-testid="save-state" type="button">Save state</button>
      <button data-testid="read-state" type="button">Read state</button>
      <button data-testid="change-state" type="button">Change state</button>
      <button data-testid="restore-state" type="button">Restore state</button>
      <button data-testid="corrupt-state" type="button">Corrupt state (negative control)</button>
      <pre data-testid="callback-result">No callback yet</pre>
    </main>
    <script type="module" src="http://localhost:8080/mcp-app-fixture.js"></script>
  </body>
</html>`

func main() {
	instance := "fixture"
	metadataVariant := "dual"
	callbackPrefix := ""
	if len(os.Args) > 1 {
		instance = os.Args[1]
	}
	if len(os.Args) > 2 {
		metadataVariant = os.Args[2]
	}
	if len(os.Args) > 3 {
		callbackPrefix = os.Args[3]
	}
	if callbackPrefix != "" && strings.Trim(callbackPrefix, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
		fmt.Fprintln(os.Stderr, "fixture callback prefix must contain only letters, digits, underscores, and hyphens")
		os.Exit(2)
	}
	callbackName := func(name string) string {
		if callbackPrefix == "" {
			return name
		}
		return callbackPrefix + "-" + name
	}
	s := server.NewMCPServer("mcp-apps-test-server-"+instance, "1.0.0", server.WithToolCapabilities(true), server.WithResourceCapabilities(true, false))
	tool := mcp.NewTool("open_view",
		mcp.WithDescription("Open the fixture MCP App"),
		mcp.WithString("message", mcp.Description("Message to return")),
	)
	toolMeta, err := appToolMetadata(metadataVariant)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	tool.Meta = &mcp.Meta{AdditionalFields: toolMeta}
	s.AddTool(tool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		message, _ := req.RequireString("message")
		return result(instance, message), nil
	})
	savedState := ""
	var savedStateMu sync.RWMutex
	for _, name := range []string{"save_state", "read_state"} {
		callback := name
		options := []mcp.ToolOption{mcp.WithDescription("MCP App callback")}
		if callback == "save_state" {
			options = append(options, mcp.WithString("state", mcp.Description("Synthetic state to retain for this fixture process; limited to 256 bytes")))
		}
		t := mcp.NewTool(callback, options...)
		t.Meta = &mcp.Meta{AdditionalFields: map[string]any{"ui": map[string]any{"visibility": []string{"app"}}}}
		s.AddTool(t, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if callback == "save_state" {
				if value, exists := req.GetArguments()["state"]; exists {
					state, ok := value.(string)
					if !ok {
						return mcp.NewToolResultError("state must be a string"), nil
					}
					if len(state) > maxSyntheticStateBytes {
						return mcp.NewToolResultError("state exceeds the 256-byte fixture limit"), nil
					}
					savedStateMu.Lock()
					savedState = state
					savedStateMu.Unlock()
					return stateResult(instance, callback, state), nil
				}
			}
			if callback == "read_state" {
				savedStateMu.RLock()
				state := savedState
				savedStateMu.RUnlock()
				return stateResult(instance, callback, state), nil
			}
			return mcp.NewToolResultText(instance + ":" + callback), nil
		})
	}
	s.AddTool(mcp.NewTool("structured_result", mcp.WithDescription("Return structured fixture data")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return result(instance, "structured"), nil
	})
	s.AddTool(mcp.NewTool("fail_tool", mcp.WithDescription("Return a tool error")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("fixture tool failure: " + instance), nil
	})
	s.AddResource(mcp.NewResource(resourceURI, "Fixture App", mcp.WithMIMEType("text/html;profile=mcp-app")), func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return fixtureResourceContents(callbackName("save_state"), callbackName("read_state")), nil
	})
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fixtureResourceContents(saveStateTool, readStateTool string) []mcp.ResourceContents {
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI: resourceURI, MIMEType: "text/html;profile=mcp-app",
		Text: strings.NewReplacer(
			"SAVE_STATE_TOOL", saveStateTool,
			"READ_STATE_TOOL", readStateTool,
		).Replace(fixtureViewHTML),
		Meta: map[string]any{"ui": map[string]any{"csp": map[string]any{
			// The pinned reference host serves the separately generated App SDK
			// bundle from its static origin. No other external resources or
			// network connections are permitted by this fixture.
			"resourceDomains": []string{"http://localhost:8080"},
			"connectDomains":  []string{},
		}}},
	}}
}

func stateResult(instance, callback, state string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(instance + ":" + callback)},
		StructuredContent: map[string]any{
			"instance": instance,
			"state":    state,
		},
	}
}

func appToolMetadata(variant string) (map[string]any, error) {
	metadata := map[string]any{}
	if variant == "modern" || variant == "dual" {
		metadata["ui"] = map[string]any{"resourceUri": resourceURI}
	}
	if variant == "legacy" || variant == "dual" {
		metadata["ui/resourceUri"] = resourceURI
	}
	if len(metadata) == 0 {
		return nil, fmt.Errorf("unknown App metadata variant %q (want modern, legacy, or dual)", variant)
	}
	return metadata, nil
}

func result(instance, message string) *mcp.CallToolResult {
	payload := map[string]any{"instance": instance, "message": message}
	raw, _ := json.Marshal(payload)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{mcp.NewTextContent(string(raw))},
		StructuredContent: payload,
		Meta:              &mcp.Meta{AdditionalFields: map[string]any{"fixture": instance}},
	}
}
