package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const resourceURI = "ui://mcp-apps-test-server/view.html"
const maxSyntheticStateBytes = 256

func main() {
	instance := "fixture"
	metadataVariant := "dual"
	if len(os.Args) > 1 {
		instance = os.Args[1]
	}
	if len(os.Args) > 2 {
		metadataVariant = os.Args[2]
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
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI: resourceURI, MIMEType: "text/html;profile=mcp-app",
			Text: "<!doctype html><html><head><meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'self'\"></head><body><main id=\"fixture\">" + instance + "</main></body></html>",
			Meta: map[string]any{"ui": map[string]any{"csp": map[string]any{"resourceDomains": []string{}, "connectDomains": []string{}}}},
		}}, nil
	})
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
