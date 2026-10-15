package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	"github.com/maximhq/bifrost/transports/v2/bifrost-http/lib"
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

func TestInitializeNegotiatesMCPAppsExtensionOnlyWhenRequested(t *testing.T) {
	tm := &fakeInstructionsToolManager{}
	h := &MCPServerHandler{
		toolManager: tm,
		config:      &lib.Config{ClientConfig: &configstore.ClientConfig{}},
	}
	h.mcpServer.Store(h.buildServer(nil))

	requestBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"extensions":{"io.modelcontextprotocol/ui":{}}},"clientInfo":{"name":"apps-host","version":"1"}}}`)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetBody(requestBody)
	h.handleMCPServer(ctx)
	var response struct {
		Result struct {
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &response))
	require.Contains(t, response.Result.Capabilities, "extensions")
	var extensions map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.Result.Capabilities["extensions"], &extensions))
	assert.Contains(t, extensions, "io.modelcontextprotocol/ui")
	var uiSupport struct {
		MIMETypes []string `json:"mimeTypes"`
	}
	require.NoError(t, json.Unmarshal(extensions["io.modelcontextprotocol/ui"], &uiSupport))
	assert.Equal(t, []string{"text/html;profile=mcp-app"}, uiSupport.MIMETypes)

	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"plain-host","version":"1"}}}`))
	h.handleMCPServer(ctx)
	response.Result.Capabilities = nil
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &response))
	assert.NotContains(t, response.Result.Capabilities, "extensions")
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

// slugAdmitter admits everything and resolves slugs from fixed maps.
type slugAdmitter struct {
	vmcps   map[string][]string
	clients map[string][]string
}

func (slugAdmitter) AdmitMCPGatewayRequest(*schemas.BifrostContext) (schemas.Access, *schemas.BifrostError) {
	return nil, nil
}

func (a slugAdmitter) VirtualMCPToolAccess(_ *schemas.BifrostContext, slug string, _ schemas.Access) ([]string, bool) {
	served, ok := a.vmcps[slug]
	return served, ok
}

func (a slugAdmitter) MCPClientToolAccess(_ *schemas.BifrostContext, slug string, _ schemas.Access) ([]string, bool) {
	served, ok := a.clients[slug]
	return served, ok
}

// admitBySlug answers 403 for a real refusal, stamps the served tools when the slug names a granted
// Virtual MCP or client, and without an admitter serves a configured client whole, as /mcp does.
func TestAdmitBySlug(t *testing.T) {
	resolver := slugAdmitter{
		vmcps:   map[string][]string{"research-tools": {"alpha-search"}},
		clients: map[string][]string{"context7": {"context7-*"}},
	}
	cases := []struct {
		name       string
		admitter   MCPGatewayAdmitter
		slug       string
		wantStatus int
		wantTools  []string
	}{
		{name: "nil admitter serves a configured client unrestricted", slug: "context7", wantTools: []string{"context7-*"}},
		{name: "nil admitter refuses an unknown slug", slug: "definitely-does-not-exist", wantStatus: 403},
		{name: "granted virtual mcp", admitter: resolver, slug: "research-tools", wantTools: []string{"alpha-search"}},
		{name: "granted mcp client", admitter: resolver, slug: "context7", wantTools: []string{"context7-*"}},
		{name: "unknown slug", admitter: resolver, slug: "definitely-does-not-exist", wantStatus: 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &lib.Config{MCPConfig: &schemas.MCPConfig{ClientConfigs: []*schemas.MCPClientConfig{
				{ID: "c7", Name: "context7", EndpointSlug: "context7"},
			}}}
			h := &MCPServerHandler{admitter: tc.admitter, config: cfg}
			bctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
			refusal := h.admitBySlug(bctx, tc.slug, nil)
			if tc.wantStatus != 0 {
				require.NotNil(t, refusal)
				assert.Equal(t, tc.wantStatus, refusal.status)
				if tc.wantStatus == 403 {
					assert.Equal(t, "access_denied", refusal.message)
				}
				return
			}
			require.Nil(t, refusal)
			assert.Equal(t, tc.wantTools, bctx.Value(schemas.MCPContextKeyIncludeTools))
		})
	}
}

// BenchmarkMCPToolCallResponse measures the public JSON-RPC tool-call handler
// for successful results and protocol-marked Code Mode failures.
func BenchmarkMCPToolCallResponse(b *testing.B) {
	for _, tc := range []struct {
		name    string
		isError bool
	}{
		{name: "success"},
		{name: "codemode_error", isError: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			isError := schemas.Ptr(tc.isError)
			if !tc.isError {
				isError = nil
			}
			tm := &markerToolManager{message: &schemas.ChatMessage{
				Content:         &schemas.ChatMessageContent{ContentStr: schemas.Ptr("tool response")},
				ChatToolMessage: &schemas.ChatToolMessage{IsError: isError},
			}}
			h := &MCPServerHandler{
				toolManager: tm,
				config:      &lib.Config{ClientConfig: &configstore.ClientConfig{}},
			}
			h.mcpServer.Store(h.buildServer(tm.GetAvailableMCPTools(context.Background())))
			request := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{}}}`)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				response := h.server().HandleMessage(context.Background(), request)
				if response == nil {
					b.Fatal("tools/call returned no response")
				}
				if _, err := json.Marshal(response); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type mcpAppsToolManager struct{ tools []schemas.ChatTool }

func (m *mcpAppsToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool { return m.tools }
func (m *mcpAppsToolManager) GetMCPServerInstructions(context.Context) string         { return "" }
func (m *mcpAppsToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return &schemas.ChatMessage{}, nil
}
func (m *mcpAppsToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}

func TestToolsListPreservesAndQualifiesMCPAppMetadata(t *testing.T) {
	appTool := schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       "app-client-open_view",
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
		MCPApp: &schemas.MCPAppToolMetadata{
			RawDefinition: []byte(`{"name":"open_view","inputSchema":{"type":"object","properties":{}},"_meta":{"ui":{"resourceUri":"ui://shared/view","visibility":["app"]},"ui/resourceUri":"ui://shared/view"}}`),
			ResourceURI:   "ui://shared/view",
			Visibility:    []string{"app"},
		},
	}
	tm := &mcpAppsToolManager{tools: []schemas.ChatTool{appTool}}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tm.tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`))
	h.handleMCPServer(ctx)

	var response struct {
		Result struct {
			Tools []struct {
				Meta struct {
					UI struct {
						ResourceURI string   `json:"resourceUri"`
						Visibility  []string `json:"visibility"`
					} `json:"ui"`
					LegacyURI string `json:"ui/resourceUri"`
				} `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &response))
	require.Len(t, response.Result.Tools, 1)
	resourceURI := response.Result.Tools[0].Meta.UI.ResourceURI
	assert.Contains(t, resourceURI, "bifrost-mcp-app://", "the source-qualified gateway URI must be advertised to the App")
	assert.Equal(t, resourceURI, response.Result.Tools[0].Meta.LegacyURI, "modern and legacy aliases must resolve to the same gateway resource")
	assert.Equal(t, []string{"app"}, response.Result.Tools[0].Meta.UI.Visibility)
}
