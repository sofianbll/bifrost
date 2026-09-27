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

func TestHandleExecuteToolCodePreservesNestedErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		upstreamError bool
		emptyContent  bool
		postHookError *bool
		wantError     bool
	}{
		{name: "success"},
		{name: "upstream failure", upstreamError: true, wantError: true},
		{name: "empty upstream failure", upstreamError: true, emptyContent: true, wantError: true},
		{name: "post hook rejects", postHookError: schemas.Ptr(true), wantError: true},
		{name: "post hook recovers", upstreamError: true, postHookError: schemas.Ptr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := server.NewMCPServer("debug", "1.0")
			calls := 0
			upstream.AddTool(mcp.NewTool("probe"), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				calls++
				result := mcp.NewToolResultText("probe response")
				result.IsError = tc.upstreamError
				if tc.emptyContent {
					result.Content = nil
				}
				return result, nil
			})
			conn, err := client.NewInProcessClient(upstream)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			_, err = conn.Initialize(context.Background(), mcp.InitializeRequest{Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "regression", Version: "1.0"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			manager := &testClientManager{
				clients: map[string]*schemas.MCPClientState{"debug": {ExecutionConfig: &schemas.MCPClientConfig{IsCodeModeClient: true}}},
				tools:   map[string][]schemas.ChatTool{"debug": {{Function: &schemas.ChatToolFunction{Name: "debug-probe"}}}},
				conn:    conn,
				postHook: func(resp *schemas.BifrostMCPResponse) {
					if tc.postHookError != nil {
						resp.ChatMessage.ChatToolMessage.IsError = tc.postHookError
					}
				},
			}
			mode := NewStarlarkCodeMode(nil, nil)
			mode.clientManager = manager
			ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
			msg, err := mode.handleExecuteToolCode(ctx, schemas.ChatAssistantMessageToolCall{
				ID: schemas.Ptr("call_nested"),
				Function: schemas.ChatAssistantMessageToolCallFunction{
					Name: schemas.Ptr("executeToolCode"), Arguments: `{"code":"result = debug.probe()"}`,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("expected one upstream call, got %d", calls)
			}
			if msg == nil || msg.ChatToolMessage == nil || msg.Content == nil || msg.Content.ContentStr == nil {
				t.Fatal("missing tool response")
			}
			isError := msg.ChatToolMessage.IsError != nil && *msg.ChatToolMessage.IsError
			text := *msg.Content.ContentStr
			if isError != tc.wantError {
				t.Errorf("IsError=%v, want %v; content=%s", isError, tc.wantError, text)
			}
			if strings.Contains(text, "Execution completed successfully") == tc.wantError {
				t.Errorf("incorrect execution status: %s", text)
			}
			wantText := "probe response"
			if tc.emptyContent {
				wantText = "returned an error"
			}
			if !strings.Contains(text, wantText) {
				t.Errorf("lost tool response: %s", text)
			}
		})
	}
}

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
