package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestConvertToolFunctionParametersToMCPInputSchemaPreservesDefs(t *testing.T) {
	params := &schemas.ToolFunctionParameters{
		Type: "object",
		Properties: schemas.NewOrderedMapFromPairs(
			schemas.KV("preferences", map[string]any{"$ref": "#/$defs/Preferences"}),
		),
		Required: []string{"preferences"},
		Defs: schemas.NewOrderedMapFromPairs(
			schemas.KV("Preferences", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"startHour": map[string]any{"type": "string"},
				},
			}),
		),
	}

	inputSchema := convertToolFunctionParametersToMCPInputSchema(params)

	require.Contains(t, inputSchema.Defs, "Preferences")
	data, err := json.Marshal(inputSchema)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"$defs"`)
	assert.Contains(t, string(data), `"$ref":"#/$defs/Preferences"`)
}

func TestConvertToolFunctionParametersToMCPInputSchemaPreservesLegacyDefinitionsAsDefs(t *testing.T) {
	params := &schemas.ToolFunctionParameters{
		Type: "object",
		Properties: schemas.NewOrderedMapFromPairs(
			schemas.KV("preferences", map[string]any{"$ref": "#/$defs/Preferences"}),
		),
		Definitions: schemas.NewOrderedMapFromPairs(
			schemas.KV("Preferences", map[string]any{"type": "object"}),
		),
	}

	inputSchema := convertToolFunctionParametersToMCPInputSchema(params)

	require.Contains(t, inputSchema.Defs, "Preferences")
	data, err := json.Marshal(inputSchema)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"$defs"`)
}

// Forwarding upstream MCP `instructions` on the gateway's own initialize response.

// fakeInstructionsToolManager reports a fixed aggregate and records the context it was asked
// with, so a test can assert the hook resolves the text per request rather than once at build.
type fakeInstructionsToolManager struct {
	instructions string
	seenCtx      context.Context
}

func (f *fakeInstructionsToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool {
	return nil
}

func (f *fakeInstructionsToolManager) GetMCPServerInstructions(ctx context.Context) string {
	f.seenCtx = ctx
	return f.instructions
}

func (f *fakeInstructionsToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return nil, nil
}

func (f *fakeInstructionsToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}

// initializeResult drives a real initialize through the built server and returns the decoded
// result, which is what a connecting MCP client would actually receive.
func initializeResult(t *testing.T, tm MCPToolManager, ctx context.Context) map[string]any {
	t.Helper()
	h := &MCPServerHandler{
		toolManager: tm,
		config:      &lib.Config{ClientConfig: &configstore.ClientConfig{}},
	}
	h.mcpServer.Store(h.buildServer(nil))

	raw := h.server().HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"0.0.1"}}}`))
	require.NotNil(t, raw)

	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	return envelope.Result
}

func TestInitializeForwardsInstructionsOnTheHandshake(t *testing.T) {
	tm := &fakeInstructionsToolManager{instructions: "<mcp_server name=\"github\">\nAlways call get_me first.\n</mcp_server>"}

	result := initializeResult(t, tm, context.Background())

	assert.Equal(t, "<mcp_server name=\"github\">\nAlways call get_me first.\n</mcp_server>", result["instructions"])
}

// Nothing to say means no field, rather than an empty one a client would have to special-case.
func TestInitializeOmitsInstructionsWhenNoUpstreamAdvertisesAny(t *testing.T) {
	tm := &fakeInstructionsToolManager{instructions: ""}

	result := initializeResult(t, tm, context.Background())

	_, present := result["instructions"]
	assert.False(t, present)
}

// The hook must resolve against the per-request context — that is the whole reason it is a
// hook rather than server.WithInstructions, which is fixed at construction. If the request's
// narrowing did not reach the resolver, a caller scoped to one upstream would be handed every
// upstream's text.
func TestInitializeResolvesInstructionsFromTheRequestContext(t *testing.T) {
	tm := &fakeInstructionsToolManager{instructions: "<mcp_server name=\"alpha\">\nscoped\n</mcp_server>"}
	ctx := context.WithValue(context.Background(), schemas.MCPContextKeyIncludeClients, []string{"alpha"})

	initializeResult(t, tm, ctx)

	require.NotNil(t, tm.seenCtx)
	assert.Equal(t, []string{"alpha"}, tm.seenCtx.Value(schemas.MCPContextKeyIncludeClients))
}

// A /mcp/<slug> request is narrowed by admitBySlug, which stamps the served tool list on
// MCPContextKeyIncludeTools rather than include-clients. The resolver has to see that narrowing
// too, or a Virtual MCP endpoint would answer initialize with every upstream's instructions.
func TestInitializeResolvesInstructionsForSlugNarrowedRequests(t *testing.T) {
	tm := &fakeInstructionsToolManager{instructions: "<mcp_server name=\"alpha\">\nscoped\n</mcp_server>"}
	ctx := context.WithValue(context.Background(), schemas.MCPContextKeyIncludeTools, []string{"alpha-echo"})

	result := initializeResult(t, tm, ctx)

	assert.Equal(t, "<mcp_server name=\"alpha\">\nscoped\n</mcp_server>", result["instructions"])
	require.NotNil(t, tm.seenCtx)
	assert.Equal(t, []string{"alpha-echo"}, tm.seenCtx.Value(schemas.MCPContextKeyIncludeTools),
		"the slug's served tool list must reach the instructions resolver")
}

type mcpAppsToolManager struct {
	tools        []schemas.ChatTool
	nativeResult json.RawMessage
	message      *schemas.ChatMessage
}

func (m *mcpAppsToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool { return m.tools }
func (m *mcpAppsToolManager) GetMCPServerInstructions(context.Context) string         { return "" }
func (m *mcpAppsToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return m.message, nil
}
func (m *mcpAppsToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}
func (m *mcpAppsToolManager) ExecuteMCPAppTool(_ *schemas.BifrostContext, _ *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, json.RawMessage, *schemas.BifrostError) {
	return m.message, m.nativeResult, nil
}
func (*mcpAppsToolManager) ReadMCPAppResource(_ *schemas.BifrostContext, _, upstreamURI string) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{Contents: []mcp.ResourceContents{mcp.TextResourceContents{URI: upstreamURI, MIMEType: "text/html;profile=mcp-app", Text: "<html>app</html>"}}}, nil
}
func BenchmarkMCPHTTPNoApp(b *testing.B) {
	tm := &mcpAppsToolManager{
		tools: []schemas.ChatTool{{
			Type: schemas.ChatToolTypeFunction,
			Function: &schemas.ChatToolFunction{
				Name:       "lookup",
				Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
			},
		}},
		message: &schemas.ChatMessage{
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("tool response")},
		},
	}
	h := &MCPServerHandler{
		toolManager: tm,
		config:      &lib.Config{ClientConfig: &configstore.ClientConfig{}},
	}
	h.mcpServer.Store(h.buildServer(tm.GetAvailableMCPTools(context.Background())))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{}}}`))
	ctx.Response.Reset()
	h.handleMCPServer(ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		b.Fatalf("preflight returned HTTP status %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
	var preflight struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(ctx.Response.Body(), &preflight); err != nil {
		b.Fatalf("preflight response is not JSON: %v: %s", err, ctx.Response.Body())
	}
	if len(preflight.Error) != 0 && string(preflight.Error) != "null" {
		b.Fatalf("preflight returned JSON-RPC error: %s", preflight.Error)
	}
	if len(preflight.Result.Content) != 1 || preflight.Result.Content[0].Text != "tool response" {
		b.Fatalf("preflight returned unexpected tool result: %s", ctx.Response.Body())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Response.Reset()
		h.handleMCPServer(ctx)
		if ctx.Response.StatusCode() != fasthttp.StatusOK {
			b.Fatalf("unexpected HTTP status: %d", ctx.Response.StatusCode())
		}
	}
}
