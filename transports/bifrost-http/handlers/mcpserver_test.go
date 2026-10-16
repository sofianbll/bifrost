package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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

type markerToolManager struct {
	fakeInstructionsToolManager
	message *schemas.ChatMessage
}

func (f *markerToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool {
	return []schemas.ChatTool{{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       "lookup",
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
	}}
}

func (f *markerToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return f.message, nil
}

// A Code Mode failure must survive the gateway's real tools/call boundary as
// MCP's protocol-level isError field.
func TestCallToolPreservesCodeModeErrorMarker(t *testing.T) {
	message := &schemas.ChatMessage{
		Content:         &schemas.ChatMessageContent{ContentStr: schemas.Ptr("Execution runtime error: missing value")},
		ChatToolMessage: &schemas.ChatToolMessage{IsError: schemas.Ptr(true)},
	}
	tm := &markerToolManager{message: message}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tm.GetAvailableMCPTools(context.Background())))

	response := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{}}}`))
	require.NotNil(t, response)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	assert.True(t, envelope.Result.IsError)
	assert.Contains(t, envelope.Result.Content[0].Text, "Execution runtime error")
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

// BenchmarkMCPHTTPNoApp measures the ordinary public HTTP MCP tools/call path
// when the server has no App metadata. It includes gateway admission, dispatch,
// and JSON response encoding so the no-App alias fast path is represented.
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

type hiddenMCPToolManager struct{ *mcpAppsToolManager }

func (*hiddenMCPToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool { return nil }

type mcpAppAliasToolManager struct{ tools []schemas.ChatTool }

func (m *mcpAppAliasToolManager) GetAvailableMCPTools(ctx context.Context) []schemas.ChatTool {
	requested, _ := ctx.Value(schemas.MCPContextKeyIncludeTools).([]string)
	if len(requested) == 0 {
		return m.tools
	}
	allowed := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		allowed[name] = struct{}{}
	}
	filtered := make([]schemas.ChatTool, 0, len(m.tools))
	for _, tool := range m.tools {
		if tool.Function == nil {
			continue
		}
		if _, ok := allowed[tool.Function.Name]; ok {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func (*mcpAppAliasToolManager) GetMCPServerInstructions(context.Context) string { return "" }

func (*mcpAppAliasToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return nil, nil
}

func (*mcpAppAliasToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}

func (*mcpAppAliasToolManager) ExecuteMCPAppTool(_ *schemas.BifrostContext, call *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, json.RawMessage, *schemas.BifrostError) {
	name := ""
	if call != nil && call.Function.Name != nil {
		name = *call.Function.Name
	}
	result := mcp.CallToolResult{
		Content:           []mcp.Content{mcp.NewTextContent(name)},
		StructuredContent: map[string]any{"tool": name},
	}
	raw, _ := json.Marshal(result)
	return nil, raw, nil
}

func (*mcpAppAliasToolManager) ReadMCPAppResource(*schemas.BifrostContext, string, string) (*mcp.ReadResourceResult, error) {
	return nil, nil
}

func appCallbackTool(source, callback string) schemas.ChatTool {
	definition, _ := json.Marshal(mcp.Tool{
		Name:        callback,
		InputSchema: mcp.ToolInputSchema{Type: "object", Properties: map[string]any{}},
		Meta:        &mcp.Meta{AdditionalFields: map[string]any{"ui": map[string]any{"visibility": []string{"app"}}}},
	})
	return schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       source + "-" + callback,
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
		Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{
			RawDefinition: definition,
			SourceClient:  source,
			Visibility:    []string{"app"},
		}},
	}
}

func TestMCPAppsBareCallbackSelectsUniqueAvailableSource(t *testing.T) {
	tools := []schemas.ChatTool{appCallbackTool("first", "save_state")}
	tm := &mcpAppAliasToolManager{tools: tools}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"hello"}}}`))
	h.handleMCPServer(ctx)

	var response struct {
		Result *struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Structured map[string]string `json:"structuredContent"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &response))
	require.NotNil(t, response.Result, "a unique admitted App callback alias should execute through the MCP HTTP endpoint")
	require.Len(t, response.Result.Content, 1)
	assert.Equal(t, "first-save_state", response.Result.Content[0].Text, "the alias should route to the canonical source-qualified tool")
	assert.Equal(t, "first-save_state", response.Result.Structured["tool"], "the native execution path should receive the canonical name")
}

func TestMCPAppsBareCallbackIsAmbiguousWithoutDisclosingSources(t *testing.T) {
	tools := []schemas.ChatTool{
		appCallbackTool("first", "save_state"),
		appCallbackTool("second", "save_state"),
	}
	tm := &mcpAppAliasToolManager{tools: tools}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"save_state","arguments":{}}}`))
	h.handleMCPServer(ctx)

	var response struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &response))
	assert.Equal(t, mcp.INVALID_PARAMS, response.Error.Code)
	assert.Equal(t, "App callback alias is ambiguous", response.Error.Message)
	assert.NotContains(t, string(ctx.Response.Body()), "first-")
	assert.NotContains(t, string(ctx.Response.Body()), "second-")
}

func TestMCPAppsAmbiguousCallbackWithNullIDReturnsJSONRPCError(t *testing.T) {
	tools := []schemas.ChatTool{
		appCallbackTool("first", "save_state"),
		appCallbackTool("second", "save_state"),
	}
	tm := &mcpAppAliasToolManager{tools: tools}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":null,"method":"tools/call","params":{"name":"save_state","arguments":{}}}`))
	h.handleMCPServer(ctx)
	assert.Contains(t, string(ctx.Response.Body()), `"id":null`)
	assert.Contains(t, string(ctx.Response.Body()), `"code":-32602`)
}

func TestMCPAppsBareCallbackUsesOnlyAdmittedSourceAndCatalogStaysQualified(t *testing.T) {
	tools := []schemas.ChatTool{
		appCallbackTool("first", "save_state"),
		appCallbackTool("second", "save_state"),
	}
	tm := &mcpAppAliasToolManager{tools: tools}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("x-bf-mcp-include-tools", "second-save_state")
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"save_state","arguments":{}}}`))
	h.handleMCPServer(ctx)

	var called struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(ctx.Response.Body(), &called))
	require.Len(t, called.Result.Content, 1)
	assert.Equal(t, "second-save_state", called.Result.Content[0].Text)

	listCtx := &fasthttp.RequestCtx{}
	listCtx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":14,"method":"tools/list","params":{}}`))
	h.handleMCPServer(listCtx)
	assert.Contains(t, string(listCtx.Response.Body()), `"name":"first-save_state"`)
	assert.Contains(t, string(listCtx.Response.Body()), `"name":"second-save_state"`)
	assert.NotContains(t, string(listCtx.Response.Body()), `"name":"save_state"`)
}

func TestMCPAppsBareCallbackCannotBypassDeniedGatewayRequest(t *testing.T) {
	tools := []schemas.ChatTool{appCallbackTool("first", "save_state")}
	tm := &mcpAppAliasToolManager{tools: tools}
	denied := &schemas.BifrostError{StatusCode: schemas.Ptr(fasthttp.StatusForbidden), Error: &schemas.ErrorField{Message: "denied"}}
	h := &MCPServerHandler{toolManager: tm, admitter: &fakeAdmitter{refusal: denied}, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/mcp")
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBody([]byte(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"save_state","arguments":{}}}`))
	h.handleMCPServer(ctx)
	assert.Equal(t, fasthttp.StatusForbidden, ctx.Response.StatusCode())
	assert.NotContains(t, string(ctx.Response.Body()), "first-save_state")
}

func TestCallToolRefusesHiddenToolWithoutDisclosingItsName(t *testing.T) {
	tool := schemas.ChatTool{Type: schemas.ChatToolTypeFunction, Function: &schemas.ChatToolFunction{Name: "private-client-secret_tool", Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()}}, Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{SourceClient: "private-client", Visibility: []string{"app"}}}}
	tm := &hiddenMCPToolManager{mcpAppsToolManager: &mcpAppsToolManager{tools: []schemas.ChatTool{tool}}}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer([]schemas.ChatTool{tool}))
	response := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"private-client-secret_tool","arguments":{}}}`))
	require.NotNil(t, response)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-client-secret_tool")
	assert.Contains(t, string(encoded), "tool is unavailable")
}

func TestCallToolPreservesNativeMCPAppResultAndUsesEffectiveFallback(t *testing.T) {
	appTool := schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       "app-client-open_view",
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
		Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{ResourceURI: "ui://shared/view", SourceClient: "client", ModernURI: true}},
	}
	for _, test := range []struct {
		name       string
		native     string
		message    *schemas.ChatMessage
		wantText   string
		wantNative bool
	}{
		{
			name:       "native result",
			native:     `{"content":[{"type":"text","text":"native app output"}],"structuredContent":{"ok":true},"_meta":{"trace":"kept"},"isError":true}`,
			wantText:   "native app output",
			wantNative: true,
		},
		{
			name:     "effective post hook fallback",
			message:  &schemas.ChatMessage{Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("redacted output")}, ChatToolMessage: &schemas.ChatToolMessage{IsError: schemas.Ptr(true)}},
			wantText: "redacted output",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tm := &mcpAppsToolManager{tools: []schemas.ChatTool{appTool}, nativeResult: json.RawMessage(test.native), message: test.message}
			h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
			h.mcpServer.Store(h.buildServer(tm.tools))
			response := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"app-client-open_view","arguments":{}}}`))
			require.NotNil(t, response)
			encoded, err := json.Marshal(response)
			require.NoError(t, err)
			var envelope struct {
				Result struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
					Structured map[string]any `json:"structuredContent"`
					Meta       map[string]any `json:"_meta"`
					IsError    bool           `json:"isError"`
				} `json:"result"`
			}
			require.NoError(t, json.Unmarshal(encoded, &envelope))
			require.NotEmpty(t, envelope.Result.Content)
			assert.Equal(t, test.wantText, envelope.Result.Content[0].Text)
			if test.wantNative {
				assert.Equal(t, map[string]any{"ok": true}, envelope.Result.Structured)
				assert.Equal(t, map[string]any{"trace": "kept"}, envelope.Result.Meta)
				assert.True(t, envelope.Result.IsError)
			} else {
				assert.True(t, envelope.Result.IsError)
			}
		})
	}
}

func TestToolsListPreservesAndQualifiesMCPAppMetadata(t *testing.T) {
	appTool := schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       "app-client-open_view",
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
		Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{
			RawDefinition: []byte(`{"name":"open_view","inputSchema":{"type":"object","properties":{}},"_meta":{"ui":{"resourceUri":"ui://shared/view","visibility":["app"]},"ui/resourceUri":"ui://shared/view"}}`),
			ResourceURI:   "ui://shared/view",
			Visibility:    []string{"app"},
			SourceClient:  "mcp_apps_fixture",
			ModernURI:     true,
			LegacyURI:     true,
		}},
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
	assert.Equal(t, "ui://bifrost/bWNwX2FwcHNfZml4dHVyZQ/dWk6Ly9zaGFyZWQvdmlldw", resourceURI, "the source-qualified gateway URI must be advertised to the App")
	assert.Equal(t, resourceURI, response.Result.Tools[0].Meta.LegacyURI, "modern and legacy aliases must resolve to the same gateway resource")
	assert.Equal(t, []string{"app"}, response.Result.Tools[0].Meta.UI.Visibility)
}

func TestMCPAppsPreservesSupportedOutputSchema(t *testing.T) {
	const upstreamURI = "ui://shared/output-schema"
	upstream := mcp.Tool{
		Name: "open_view",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{},
		},
		OutputSchema: mcp.ToolOutputSchema{
			Type: "object",
			Properties: map[string]any{
				"instance": map[string]any{"type": "string"},
				"message":  map[string]any{"type": "string"},
			},
			Required: []string{"instance", "message"},
		},
		Meta: &mcp.Meta{AdditionalFields: map[string]any{
			"ui": map[string]any{"resourceUri": upstreamURI, "visibility": []string{"app"}},
		}},
	}
	rawDefinition, err := json.Marshal(upstream)
	require.NoError(t, err)
	tool := schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:       "app-client-open_view",
			Parameters: &schemas.ToolFunctionParameters{Type: "object", Properties: schemas.NewOrderedMapFromPairs()},
		},
		Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{
			RawDefinition: rawDefinition,
			ResourceURI:   upstreamURI,
			SourceClient:  "client",
			Visibility:    []string{"app"},
			ModernURI:     true,
		}},
	}
	tm := &mcpAppsToolManager{tools: []schemas.ChatTool{tool}}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tm.tools))
	response := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":8,"method":"tools/list","params":{}}`))
	require.NotNil(t, response)
	encoded, err := json.Marshal(response)
	require.NoError(t, err)
	var envelope struct {
		Result struct {
			Tools []struct {
				Name         string          `json:"name"`
				OutputSchema json.RawMessage `json:"outputSchema"`
				Meta         json.RawMessage `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	require.Len(t, envelope.Result.Tools, 1)
	assert.JSONEq(t, `{"type":"object","properties":{"instance":{"type":"string"},"message":{"type":"string"}},"required":["instance","message"]}`, string(envelope.Result.Tools[0].OutputSchema), "the public MCP App definition must retain the SDK-supported output schema")
	var metadata struct {
		UI struct {
			ResourceURI string `json:"resourceUri"`
		} `json:"ui"`
	}
	require.NoError(t, json.Unmarshal(envelope.Result.Tools[0].Meta, &metadata))
	assert.Equal(t, mcpAppGatewayURI("client", upstreamURI), metadata.UI.ResourceURI, "the qualified App resource metadata must remain overlaid on the original definition")
}

func TestResourcesListAndReadAreSourceQualified(t *testing.T) {
	const upstreamURI = "ui://shared/view"
	tools := []schemas.ChatTool{
		{Type: schemas.ChatToolTypeFunction, Function: &schemas.ChatToolFunction{Name: "first-open_view"}, Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{SourceClient: "first", ResourceURI: upstreamURI, ModernURI: true}}},
		{Type: schemas.ChatToolTypeFunction, Function: &schemas.ChatToolFunction{Name: "second-open_view"}, Annotations: &schemas.MCPToolAnnotations{MCPApp: &schemas.MCPAppToolMetadata{SourceClient: "second", ResourceURI: upstreamURI, ModernURI: true}}},
	}
	tm := &mcpAppsToolManager{tools: tools}
	h := &MCPServerHandler{toolManager: tm, config: &lib.Config{ClientConfig: &configstore.ClientConfig{}}}
	h.mcpServer.Store(h.buildServer(tools))
	list := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":7,"method":"resources/list","params":{}}`))
	require.NotNil(t, list)
	listJSON, err := json.Marshal(list)
	require.NoError(t, err)
	var listed struct {
		Result struct {
			Resources []struct {
				URI  string `json:"uri"`
				MIME string `json:"mimeType"`
			} `json:"resources"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(listJSON, &listed))
	require.Len(t, listed.Result.Resources, 2)
	firstURI := mcpAppGatewayURI("first", upstreamURI)
	secondURI := mcpAppGatewayURI("second", upstreamURI)
	assert.NotEqual(t, firstURI, secondURI)
	assert.ElementsMatch(t, []string{firstURI, secondURI}, []string{listed.Result.Resources[0].URI, listed.Result.Resources[1].URI})
	assert.Equal(t, "text/html;profile=mcp-app", listed.Result.Resources[0].MIME)

	read := h.server().HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":8,"method":"resources/read","params":{"uri":"`+firstURI+`"}}`))
	require.NotNil(t, read)
	readJSON, err := json.Marshal(read)
	require.NoError(t, err)
	var readEnvelope struct {
		Result struct {
			Contents []struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"contents"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(readJSON, &readEnvelope))
	require.Len(t, readEnvelope.Result.Contents, 1)
	assert.Equal(t, firstURI, readEnvelope.Result.Contents[0].URI)
	assert.Equal(t, "<html>app</html>", readEnvelope.Result.Contents[0].Text)
}
