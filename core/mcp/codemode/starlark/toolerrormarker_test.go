//go:build !tinygo && !wasm

package starlark

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
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

func TestHandleExecuteToolCodeBudgetResults(t *testing.T) {
	mode := NewStarlarkCodeMode(nil, nil)
	mode.clientManager = &testClientManager{}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	for _, tc := range []struct {
		code, message string
		failed        bool
	}{
		{"result = len([0] * 200000000)", "resource limit", true},
		{"result = []\nresult.append(result)", "nesting limit", true},
		{"result = 1 << 128\nfor i in range(6):\n    result *= result", "integer exceeds size limit", true},
		{"result = 42", "Return value: 42", false},
	} {
		args, err := schemas.MarshalSorted(map[string]string{"code": tc.code})
		if err != nil {
			t.Fatal(err)
		}
		call := schemas.ChatAssistantMessageToolCall{ID: schemas.Ptr("budget"), Function: schemas.ChatAssistantMessageToolCallFunction{Name: schemas.Ptr("executeToolCode"), Arguments: string(args)}}
		msg, err := mode.handleExecuteToolCode(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		if msg.ChatToolMessage == nil || msg.Content == nil || msg.Content.ContentStr == nil {
			t.Fatalf("missing tool response: %+v", msg)
		}
		if failed := msg.IsError != nil && *msg.IsError; failed != tc.failed {
			t.Fatalf("is_error=%v for %s", failed, tc.code)
		}
		if !strings.Contains(*msg.Content.ContentStr, tc.message) {
			t.Fatalf("expected %q in %q", tc.message, *msg.Content.ContentStr)
		}
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

type protocolErrorClientManager struct {
	*testClientManager
	conn *client.Client
}

func (m *protocolErrorClientManager) AcquireClientConn(_ *schemas.BifrostContext, _ *schemas.MCPClientState) (*client.Client, func(), error) {
	return m.conn, func() {}, nil
}

func TestExecuteToolCodeCanUseProtocolMarkedRemoteErrorsAsValues(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{name: "without Error prefix", text: "permission denied"},
		{name: "with Error prefix", text: "Error: permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote := mcpserver.NewMCPServer("remote", "test", mcpserver.WithToolCapabilities(true))
			remote.AddTool(mcpgo.NewTool("lookup"), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
				return &mcpgo.CallToolResult{IsError: true, Content: []mcpgo.Content{mcpgo.TextContent{Type: "text", Text: tc.text}}}, nil
			})
			conn, err := client.NewInProcessClient(remote)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			if err := conn.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			initReq := mcpgo.InitializeRequest{}
			initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
			initReq.Params.ClientInfo = mcpgo.Implementation{Name: "test", Version: "1"}
			if _, err := conn.Initialize(context.Background(), initReq); err != nil {
				t.Fatal(err)
			}
			direct, directErr := conn.CallTool(context.Background(), mcpgo.CallToolRequest{
				Request: mcpgo.Request{Method: string(mcpgo.MethodToolsCall)},
				Params:  mcpgo.CallToolParams{Name: "lookup", Arguments: map[string]interface{}{}},
			})
			if directErr != nil || direct == nil || !direct.IsError {
				t.Fatalf("fixture did not return an MCP isError result: result=%+v err=%v", direct, directErr)
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
			mode := NewStarlarkCodeMode(&codemcp.CodeModeConfig{BindingLevel: schemas.CodeModeBindingLevelTool, ToolExecutionTimeout: time.Second}, nil)
			mode.clientManager = manager
			call := schemas.ChatAssistantMessageToolCall{
				ID: schemas.Ptr("protocol-error-value"),
				Function: schemas.ChatAssistantMessageToolCallFunction{
					Name: schemas.Ptr("executeToolCode"), Arguments: `{"code":"result = server.lookup()"}`,
				},
			}
			msg, err := mode.handleExecuteToolCode(schemas.NewBifrostContext(context.Background(), schemas.NoDeadline), call)
			if err != nil {
				t.Fatalf("script should consume the remote error value: %v", err)
			}
			if msg == nil || msg.ChatToolMessage == nil || msg.Content == nil || msg.Content.ContentStr == nil {
				t.Fatalf("expected successful outer tool result, got %+v", msg)
			}
			if msg.IsError != nil && *msg.IsError {
				t.Fatalf("remote protocol error was promoted to outer failure: %q", *msg.Content.ContentStr)
			}
			if !strings.Contains(*msg.Content.ContentStr, tc.text) {
				t.Fatalf("remote error value %q was not available to the script: %q", tc.text, *msg.Content.ContentStr)
			}
		})
	}
}
