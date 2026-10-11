package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const resourceURI = "ui://mcp-apps-test-server/view.html"

func main() {
	instance := "fixture"
	if len(os.Args) > 1 {
		instance = os.Args[1]
	}
	s := server.NewMCPServer("mcp-apps-test-server-"+instance, "1.0.0", server.WithToolCapabilities(true), server.WithResourceCapabilities(true, false))
	tool := mcp.NewTool("open_view",
		mcp.WithDescription("Open the fixture MCP App"),
		mcp.WithString("message", mcp.Description("Message to return")),
	)
	tool.Meta = &mcp.Meta{AdditionalFields: map[string]any{
		"ui":             map[string]any{"resourceUri": resourceURI},
		"ui/resourceUri": resourceURI,
	}}
	s.AddTool(tool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		message, _ := req.RequireString("message")
		return result(instance, message), nil
	})
	for _, name := range []string{"save_state", "read_state"} {
		callback := name
		t := mcp.NewTool(callback, mcp.WithDescription("MCP App callback"))
		t.Meta = &mcp.Meta{AdditionalFields: map[string]any{"ui": map[string]any{"visibility": []string{"app"}}}}
		s.AddTool(t, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

func result(instance, message string) *mcp.CallToolResult {
	payload := map[string]any{"instance": instance, "message": message}
	raw, _ := json.Marshal(payload)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{mcp.NewTextContent(string(raw))},
		StructuredContent: payload,
		Meta:              &mcp.Meta{AdditionalFields: map[string]any{"fixture": instance}},
	}
}
