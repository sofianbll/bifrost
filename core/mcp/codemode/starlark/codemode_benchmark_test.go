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

// BenchmarkCodeModeNestedProtocolError exercises executeToolCode through its
// real MCP client boundary with a protocol-marked error value.
func BenchmarkCodeModeNestedProtocolError(b *testing.B) {
	remote := mcpserver.NewMCPServer("remote", "test", mcpserver.WithToolCapabilities(true))
	remote.AddTool(mcpgo.NewTool("lookup"), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return &mcpgo.CallToolResult{IsError: true, Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: "Error: remote rejected"},
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
			tools: map[string][]schemas.ChatTool{"server": {{Function: &schemas.ChatToolFunction{Name: "lookup"}}}},
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
