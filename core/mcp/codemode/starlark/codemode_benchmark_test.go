//go:build !tinygo && !wasm

package starlark

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	codemcp "github.com/maximhq/bifrost/core/mcp"
	"github.com/maximhq/bifrost/core/schemas"
)

// BenchmarkCodeModeNestedToolResponse exercises executeToolCode through its
// real MCP client boundary for the normal result and protocol error value paths.
func BenchmarkCodeModeNestedToolResponse(b *testing.B) {
	for _, tc := range []struct {
		name    string
		text    string
		isError bool
	}{
		{name: "success", text: "42"},
		{name: "protocol_error_value", text: "Error: remote rejected", isError: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkCodeModeNestedToolResponse(b, tc.text, tc.isError)
		})
	}
}

func benchmarkCodeModeNestedToolResponse(b *testing.B, text string, isError bool) {
	remote := mcpserver.NewMCPServer("remote", "test", mcpserver.WithToolCapabilities(true))
	remote.AddTool(mcpgo.NewTool("lookup"), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return &mcpgo.CallToolResult{IsError: isError, Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: text},
		}}, nil
	})
	conn, err := client.NewInProcessClient(remote)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	if err := conn.Start(context.Background()); err != nil {
		b.Fatal(err)
	}
	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "benchmark", Version: "1"}
	if _, err := conn.Initialize(context.Background(), initReq); err != nil {
		b.Fatal(err)
	}
	state := &schemas.MCPClientState{
		Name: "server",
		ExecutionConfig: &schemas.MCPClientConfig{
			Name: "server", IsCodeModeClient: true, ToolsToExecute: schemas.WhiteList{"*"},
		},
	}
	manager := &protocolErrorClientManager{
		testClientManager: &testClientManager{
			clients: map[string]*schemas.MCPClientState{"server": state},
			tools:   map[string][]schemas.ChatTool{"server": {{Function: &schemas.ChatToolFunction{Name: "lookup"}}}},
		},
		conn: conn,
	}
	mode := NewStarlarkCodeMode(&codemcp.CodeModeConfig{
		BindingLevel: schemas.CodeModeBindingLevelTool, ToolExecutionTimeout: time.Second,
	}, nil)
	mode.clientManager = manager
	call := schemas.ChatAssistantMessageToolCall{
		ID: schemas.Ptr("bench"),
		Function: schemas.ChatAssistantMessageToolCallFunction{
			Name: schemas.Ptr("executeToolCode"), Arguments: `{"code":"result = server.lookup()"}`,
		},
	}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		msg, err := mode.handleExecuteToolCode(ctx, call)
		if err != nil || msg == nil || msg.Content == nil || msg.Content.ContentStr == nil {
			b.Fatalf("nested protocol error benchmark failed: msg=%+v err=%v", msg, err)
		}
	}
}
