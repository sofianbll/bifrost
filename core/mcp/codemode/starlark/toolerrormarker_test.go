//go:build !tinygo && !wasm

package starlark

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	codemcp "github.com/maximhq/bifrost/core/mcp"
	"github.com/maximhq/bifrost/core/schemas"
)

// TestCreateToolResponseMessageMarksError pins the CodeMode copy of the tool
// response builder. CodeMode reports its own failures -- unknown server, unknown
// tool, ambiguous filename, failed sandbox execution -- as ordinary result text,
// so without the marker the model reads a lookup failure as a successful answer.
func TestCreateToolResponseMessageMarksError(t *testing.T) {
	id := "call_1"
	name := "readToolFile"
	toolCall := schemas.ChatAssistantMessageToolCall{
		ID: &id,
		Function: schemas.ChatAssistantMessageToolCallFunction{
			Name:      &name,
			Arguments: `{"path":"servers/missing.pyi"}`,
		},
	}

	failed := createToolResponseMessage(toolCall, "No server found matching 'missing'", true)
	if failed.ChatToolMessage == nil {
		t.Fatal("expected ChatToolMessage to be attached")
	}
	if failed.ChatToolMessage.IsError == nil || !*failed.ChatToolMessage.IsError {
		t.Fatal("a CodeMode failure must set IsError true")
	}

	ok := createToolResponseMessage(toolCall, "def read_file(path: str) -> str", false)
	if ok.ChatToolMessage == nil {
		t.Fatal("expected ChatToolMessage to be attached")
	}
	if ok.ChatToolMessage.IsError != nil {
		t.Fatalf("a successful CodeMode result must leave IsError nil, got %v", *ok.ChatToolMessage.IsError)
	}
}

// TestHandleExecuteToolCodeMarksSandboxErrors covers the caller that decides what to
// hand createToolResponseMessage. A sandbox run that raises produces a non-nil
// result.Errors, and that is a tool failure: without the marker the provider receives
// the traceback as an ordinary tool result and the model reads it as a real answer.
func TestHandleExecuteToolCodeMarksSandboxErrors(t *testing.T) {
	mode := NewStarlarkCodeMode(&codemcp.CodeModeConfig{
		BindingLevel:         schemas.CodeModeBindingLevelTool,
		ToolExecutionTimeout: time.Second,
	}, nil)
	mode.clientManager = &testClientManager{}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	id := "call_err"
	name := "executeToolCode"
	toolCall := schemas.ChatAssistantMessageToolCall{
		ID: &id,
		Function: schemas.ChatAssistantMessageToolCallFunction{
			Name: &name,
			// Raises at runtime inside the sandbox: undefined name.
			Arguments: `{"code":"result = definitely_not_defined + 1"}`,
		},
	}

	msg, err := mode.handleExecuteToolCode(ctx, toolCall)
	if err != nil {
		t.Fatalf("handleExecuteToolCode returned a transport error: %v", err)
	}
	if msg == nil || msg.ChatToolMessage == nil {
		t.Fatal("expected a tool response message")
	}
	if msg.ChatToolMessage.IsError == nil || !*msg.ChatToolMessage.IsError {
		t.Fatalf("a failed sandbox execution must set IsError true, got %v content=%v",
			msg.ChatToolMessage.IsError, msg.Content)
	}
}

func TestHandleExecuteToolCodeReturnsIsErrorPrefixedToolResultAsValue(t *testing.T) {
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	mcpServer := server.NewMCPServer("error-test", "1.0.0")
	mcpServer.AddTool(mcp.NewTool("fail"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("Error: expected tool failure"), nil
	})
	conn, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient returned error: %v", err)
	}
	if err := conn.Start(ctx); err != nil {
		t.Fatalf("client start failed: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		Capabilities:    mcp.ClientCapabilities{},
	}}); err != nil {
		t.Fatalf("client initialize failed: %v", err)
	}
	manager := &testClientManager{
		clients: map[string]*schemas.MCPClientState{
			"testServer": {Name: "testServer", ExecutionConfig: &schemas.MCPClientConfig{IsCodeModeClient: true, ToolsToExecute: schemas.WhiteList{"*"}}},
		},
		tools: map[string][]schemas.ChatTool{
			"testServer": {{Function: &schemas.ChatToolFunction{Name: "fail"}}},
		},
		conn: conn,
	}
	mode := NewStarlarkCodeMode(&codemcp.CodeModeConfig{
		BindingLevel:         schemas.CodeModeBindingLevelServer,
		ToolExecutionTimeout: time.Second,
	}, nil)
	mode.clientManager = manager
	toolCall := schemas.ChatAssistantMessageToolCall{
		Function: schemas.ChatAssistantMessageToolCallFunction{
			Name:      schemas.Ptr("executeToolCode"),
			Arguments: `{"code":"result = testServer.fail()"}`,
		},
	}
	msg, err := mode.handleExecuteToolCode(ctx, toolCall)
	if err != nil {
		t.Fatalf("handleExecuteToolCode returned error: %v", err)
	}
	if msg == nil || msg.Content == nil || msg.Content.ContentStr == nil {
		t.Fatal("expected an executeToolCode result")
	}
	if msg.ChatToolMessage == nil || msg.ChatToolMessage.IsError != nil && *msg.ChatToolMessage.IsError {
		t.Fatalf("an MCP error result should be available as a value, got IsError=%v", msg.ChatToolMessage.IsError)
	}
	if !strings.Contains(*msg.Content.ContentStr, "Execution completed successfully") || !strings.Contains(*msg.Content.ContentStr, "Error: expected tool failure") {
		t.Fatalf("expected successful script output containing the nested error value, got %q", *msg.Content.ContentStr)
	}
}
