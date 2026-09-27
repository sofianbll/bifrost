package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type appListToolManager struct {
	tools      []schemas.ChatTool
	chatResult *schemas.ChatMessage
}

type appResourceToolManager struct {
	appListToolManager
	nativeResult   *mcp.CallToolResult
	resourceResult *mcp.ReadResourceResult
	nativeCalls    []schemas.ChatAssistantMessageToolCall
	resourceCalls  []struct{ toolName, uri string }
}

func (m *appResourceToolManager) ExecuteNativeMCPTool(_ context.Context, call *schemas.ChatAssistantMessageToolCall) (*mcp.CallToolResult, *schemas.BifrostError) {
	m.nativeCalls = append(m.nativeCalls, *call)
	return m.nativeResult, nil
}

func (m *appResourceToolManager) ReadMCPAppResource(_ context.Context, toolName, uri string) (*mcp.ReadResourceResult, error) {
	m.resourceCalls = append(m.resourceCalls, struct{ toolName, uri string }{toolName, uri})
	return m.resourceResult, nil
}

func TestMCPAppEmptyResourcesListIsArray(t *testing.T) {
	h := &MCPServerHandler{toolManager: &appResourceToolManager{}}
	response := h.buildServer(nil).HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`))
	data, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Result struct {
			Resources json.RawMessage `json:"resources"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.JSONEq(t, `[]`, string(decoded.Result.Resources))
}

func TestMCPAppNativeToolCallPreservesStructuredResultAndError(t *testing.T) {
	SetLogger(&mockLogger{})
	tool := schemas.ChatTool{
		Function:   &schemas.ChatToolFunction{Name: "alpha-view"},
		MCPRawTool: json.RawMessage(`{"name":"view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://app/view"}}}`),
	}
	manager := &appResourceToolManager{
		appListToolManager: appListToolManager{tools: []schemas.ChatTool{tool}},
		nativeResult: &mcp.CallToolResult{
			Content:           []mcp.Content{mcp.TextContent{Text: "render failed"}},
			StructuredContent: map[string]any{"scene": "example"},
			IsError:           true,
		},
	}
	h := &MCPServerHandler{toolManager: manager}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.MCPContextKeyAllowAppOnly, true)

	response := h.buildServer([]schemas.ChatTool{tool}).HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"alpha-view","arguments":{"prompt":"draw"}}}`))
	body, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Result struct {
			Content           []map[string]any `json:"content"`
			StructuredContent map[string]any   `json:"structuredContent"`
			IsError           bool             `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "example", decoded.Result.StructuredContent["scene"])
	assert.True(t, decoded.Result.IsError)
	require.Len(t, decoded.Result.Content, 1)
	assert.Equal(t, "render failed", decoded.Result.Content[0]["text"])
	require.Len(t, manager.nativeCalls, 1)
	assert.Equal(t, "alpha-view", *manager.nativeCalls[0].Function.Name)
	assert.JSONEq(t, `{"prompt":"draw"}`, manager.nativeCalls[0].Function.Arguments)
}

func TestMCPAppResourceReadDispatchesAndRewritesContents(t *testing.T) {
	SetLogger(&mockLogger{})
	tool := schemas.ChatTool{
		Function:   &schemas.ChatToolFunction{Name: "alpha-view"},
		MCPRawTool: json.RawMessage(`{"name":"view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://app/view"}}}`),
	}
	manager := &appResourceToolManager{
		appListToolManager: appListToolManager{tools: []schemas.ChatTool{tool}},
		resourceResult: &mcp.ReadResourceResult{Contents: []mcp.ResourceContents{
			mcp.TextResourceContents{URI: "ui://app/view", MIMEType: "text/html;profile=mcp-app", Text: "<main>app</main>"},
			mcp.BlobResourceContents{URI: "ui://app/view", MIMEType: "application/octet-stream", Blob: "AQID"},
		}},
	}
	h := &MCPServerHandler{toolManager: manager}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	ctx.SetValue(schemas.MCPContextKeyAllowAppOnly, true)
	server := h.buildServer([]schemas.ChatTool{tool})
	_, _, publicURI, err := rewriteMCPAppTool(tool.MCPRawTool, tool.Function.Name)
	require.NoError(t, err)
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": map[string]any{"uri": publicURI}})
	require.NoError(t, err)
	response := server.HandleMessage(ctx, request)
	body, err := json.Marshal(response)
	require.NoError(t, err)
	var decoded struct {
		Result struct {
			Contents []map[string]any `json:"contents"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Len(t, decoded.Result.Contents, 2)
	assert.Equal(t, publicURI, decoded.Result.Contents[0]["uri"])
	assert.Equal(t, "text/html;profile=mcp-app", decoded.Result.Contents[0]["mimeType"])
	assert.Equal(t, "<main>app</main>", decoded.Result.Contents[0]["text"])
	assert.Equal(t, publicURI, decoded.Result.Contents[1]["uri"])
	assert.Equal(t, "application/octet-stream", decoded.Result.Contents[1]["mimeType"])
	assert.Equal(t, "AQID", decoded.Result.Contents[1]["blob"])
	require.Len(t, manager.resourceCalls, 1)
	assert.Equal(t, "alpha-view", manager.resourceCalls[0].toolName)
	assert.Equal(t, "ui://app/view", manager.resourceCalls[0].uri)
}

func TestMCPAppResourceReadWithoutAdmissionDoesNotDispatch(t *testing.T) {
	SetLogger(&mockLogger{})
	tool := schemas.ChatTool{
		Function:   &schemas.ChatToolFunction{Name: "alpha-view"},
		MCPRawTool: json.RawMessage(`{"name":"view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://app/view"}}}`),
	}
	manager := &appResourceToolManager{appListToolManager: appListToolManager{tools: []schemas.ChatTool{tool}}}
	h := &MCPServerHandler{toolManager: manager}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	_, _, publicURI, err := rewriteMCPAppTool(tool.MCPRawTool, tool.Function.Name)
	require.NoError(t, err)
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": map[string]any{"uri": publicURI}})
	require.NoError(t, err)
	response := h.buildServer([]schemas.ChatTool{tool}).HandleMessage(ctx, request)
	body, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(body), "not available")
	assert.Empty(t, manager.resourceCalls)
}

func (m appListToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool { return m.tools }
func (m appListToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return m.chatResult, nil
}

func TestMCPServerChatToolResultPreservesError(t *testing.T) {
	SetLogger(&mockLogger{})
	for _, marker := range []*bool{nil, schemas.Ptr(false), schemas.Ptr(true)} {
		tool := schemas.ChatTool{Function: &schemas.ChatToolFunction{Name: "executeToolCode"}}
		manager := &appListToolManager{chatResult: &schemas.ChatMessage{
			Content:         &schemas.ChatMessageContent{ContentStr: schemas.Ptr("script result")},
			ChatToolMessage: &schemas.ChatToolMessage{IsError: marker},
		}}
		h := &MCPServerHandler{toolManager: manager}
		response := h.buildServer([]schemas.ChatTool{tool}).HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"executeToolCode","arguments":{"code":"result = missing"}}}`))
		body, err := json.Marshal(response)
		require.NoError(t, err)
		var decoded struct {
			Result *mcp.CallToolResult `json:"result"`
		}
		require.NoError(t, json.Unmarshal(body, &decoded))
		require.NotNil(t, decoded.Result, string(body))
		assert.Equal(t, marker != nil && *marker, decoded.Result.IsError, string(body))
		require.Len(t, decoded.Result.Content, 1)
		assert.Equal(t, "script result", decoded.Result.Content[0].(mcp.TextContent).Text)
	}
}
func (appListToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}

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

func TestMCPAppDuplicateCallbackNeverChoosesAnUpstream(t *testing.T) {
	tools := []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "alpha-create_view"}, MCPRawTool: json.RawMessage(`{"name":"create_view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://alpha/view"}}}`)},
		{Function: &schemas.ChatToolFunction{Name: "alpha-read_checkpoint"}, MCPRawTool: json.RawMessage(`{"name":"read_checkpoint","inputSchema":{"type":"object"},"_meta":{"ui":{"visibility":["app"]}}}`), MCPAppOnly: true},
		{Function: &schemas.ChatToolFunction{Name: "beta-create_view"}, MCPRawTool: json.RawMessage(`{"name":"create_view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://beta/view"}}}`)},
		{Function: &schemas.ChatToolFunction{Name: "beta-read_checkpoint"}, MCPRawTool: json.RawMessage(`{"name":"read_checkpoint","inputSchema":{"type":"object"},"_meta":{"ui":{"visibility":["app"]}}}`), MCPAppOnly: true},
	}
	response := []byte(`{"result":{"tools":[{"name":"alpha-create_view","_meta":{"ui":{"resourceUri":"ui://bifrost/YWxwaGE/dWk6Ly9hbHBoYS92aWV3"}}},{"name":"beta-create_view","_meta":{"ui":{"resourceUri":"ui://bifrost/YmV0YQ/dWk6Ly9iZXRhL3ZpZXc"}}}]}}`)
	_, aliasErr := resolveMCPAppCallAlias([]byte(`{"method":"tools/call","params":{"name":"read_checkpoint"}}`), collectMCPAppAliases(tools))
	require.ErrorContains(t, aliasErr, "ambiguous")
	got := addMCPAppAliasesToList([]byte(`{"method":"tools/list"}`), response, collectMCPAppAliases(tools), hasMCPAppSource(tools))
	var decoded struct {
		Result struct {
			Tools []struct {
				Meta struct {
					UI map[string]any `json:"ui"`
				} `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(got, &decoded))
	for _, tool := range decoded.Result.Tools {
		assert.NotEmpty(t, tool.Meta.UI["resourceUri"])
	}
}

func TestMCPAppAggregateWithAnotherClientPreservesUI(t *testing.T) {
	tools := []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "alpha-create_view"}, MCPRawTool: json.RawMessage(`{"name":"create_view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://alpha/view"}}}`)},
		{Function: &schemas.ChatToolFunction{Name: "alpha-private_callback"}, MCPRawTool: json.RawMessage(`{"name":"private_callback","inputSchema":{"type":"object"},"_meta":{"ui":{"visibility":["app"]}}}`), MCPAppOnly: true},
		{Function: &schemas.ChatToolFunction{Name: "beta-search"}, MCPRawTool: json.RawMessage(`{"name":"search","inputSchema":{"type":"object"}}`)},
	}
	response := []byte(`{"result":{"tools":[{"name":"alpha-create_view","_meta":{"ui":{"resourceUri":"ui://bifrost/YWxwaGE/dWk6Ly9hbHBoYS92aWV3"}}},{"name":"alpha-private_callback","_meta":{"ui":{"visibility":["app"]}}},{"name":"beta-search"}]}}`)
	got := addMCPAppAliasesToList([]byte(`{"method":"tools/list"}`), response, collectMCPAppAliases(tools), hasMCPAppSource(tools))
	var decoded struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
				Meta struct {
					UI map[string]any `json:"ui"`
				} `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(got, &decoded))
	assert.Greater(t, len(decoded.Result.Tools), 3)
	assert.NotEmpty(t, decoded.Result.Tools[0].Meta.UI["resourceUri"])
}

func TestMCPAppUIAcceptsNativeSourcesAlongsideSyntheticTools(t *testing.T) {
	assert.False(t, hasMCPAppSource(nil))
	assert.False(t, hasMCPAppSource([]schemas.ChatTool{{Function: &schemas.ChatToolFunction{Name: "local-tool"}}}))
	assert.True(t, hasMCPAppSource([]schemas.ChatTool{{Function: &schemas.ChatToolFunction{Name: "app-create_view"}, MCPRawTool: json.RawMessage(`{"name":"create_view","_meta":{"ui":{"resourceUri":"ui://app/view"}}}`)}, {Function: &schemas.ChatToolFunction{Name: "executeToolCode"}}}))
}

func TestMCPAppHelperAliasesRespectVisibility(t *testing.T) {
	tools := []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "app-helper"}, MCPRawTool: json.RawMessage(`{"name":"helper","inputSchema":{"type":"object"}}`)},
		{Function: &schemas.ChatToolFunction{Name: "app-model_only"}, MCPRawTool: json.RawMessage(`{"name":"model_only","inputSchema":{"type":"object"},"_meta":{"ui":{"visibility":["model"]}}}`)},
	}
	aliases := collectMCPAppAliases(tools)
	require.Contains(t, aliases, "helper")
	assert.NotContains(t, aliases, "model_only")
	assert.Equal(t, []string{"app"}, aliases["helper"].tool.Meta.AdditionalFields["ui"].(map[string]any)["visibility"])
	assert.JSONEq(t, `{"name":"helper","inputSchema":{"type":"object"}}`, string(tools[0].MCPRawTool))
	resolved, err := resolveMCPAppCallAlias([]byte(`{"method":"tools/call","params":{"name":"helper","arguments":{"id":"test"}}}`), aliases)
	require.NoError(t, err)
	assert.JSONEq(t, `{"method":"tools/call","params":{"name":"app-helper","arguments":{"id":"test"}}}`, string(resolved))
}

func TestMCPAppAliasPreservesArgumentsAndRejectsPublicNameCollision(t *testing.T) {
	tools := []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "alpha-callback"}, MCPRawTool: json.RawMessage(`{"name":"callback","inputSchema":{"type":"object"}}`)},
		{Function: &schemas.ChatToolFunction{Name: "callback"}},
	}
	aliases := collectMCPAppAliases(tools)
	_, err := resolveMCPAppCallAlias([]byte(`{"method":"tools/call","params":{"name":"callback"}}`), aliases)
	require.ErrorContains(t, err, "ambiguous")

	aliases = collectMCPAppAliases(tools[:1])
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"callback","arguments":{"large":9007199254740993,"nested":{"raw":1e300}}}}`)
	forwarded, err := resolveMCPAppCallAlias(request, aliases)
	require.NoError(t, err)
	assert.Contains(t, string(forwarded), `"name":"alpha-callback"`)
	assert.Contains(t, string(forwarded), `"large":9007199254740993`)
	assert.Contains(t, string(forwarded), `"raw":1e300`)
}

func TestMCPAppAggregateHidesAppOnlyTools(t *testing.T) {
	manager := appListToolManager{tools: []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "alpha-create_view"}},
		{Function: &schemas.ChatToolFunction{Name: "alpha-private_callback"}, MCPAppOnly: true},
	}}
	h := &MCPServerHandler{toolManager: manager}
	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	tools := []mcp.Tool{{Name: "alpha-create_view"}, {Name: "alpha-private_callback"}}
	ctx.SetValue(schemas.MCPContextKeyAllowAppOnly, false)
	filtered := h.makeIncludeClientsFilter()(ctx, tools)
	require.Len(t, filtered, 1)
	assert.Equal(t, "alpha-create_view", filtered[0].Name)
	ctx.SetValue(schemas.MCPContextKeyAllowAppOnly, true)
	assert.Len(t, h.makeIncludeClientsFilter()(ctx, tools), 2)
}

func TestMCPAppSingleClientCallbackAlias(t *testing.T) {
	tools := []schemas.ChatTool{
		{Function: &schemas.ChatToolFunction{Name: "excalidraw-create_view"}, MCPRawTool: json.RawMessage(`{"name":"create_view","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://excalidraw/view"}}}`)},
		{Function: &schemas.ChatToolFunction{Name: "excalidraw-read_checkpoint"}, MCPRawTool: json.RawMessage(`{"name":"read_checkpoint","inputSchema":{"type":"object"},"_meta":{"ui":{"visibility":["app"]}}}`), MCPAppOnly: true},
	}
	aliases := collectMCPAppAliases(tools)
	require.Contains(t, aliases, "read_checkpoint")
	assert.Equal(t, "excalidraw-read_checkpoint", aliases["read_checkpoint"].publicName)
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_checkpoint","arguments":{"id":"abc"}}}`)
	var forwarded struct {
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	resolved, err := resolveMCPAppCallAlias(request, aliases)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(resolved, &forwarded))
	assert.Equal(t, "excalidraw-read_checkpoint", forwarded.Params.Name)

	response := []byte(`{"result":{"tools":[{"name":"excalidraw-create_view","_meta":{"ui":{"resourceUri":"ui://bifrost/ZXhjYWxpZHJhdw/dWk6Ly9leGNhbGlkcmF3L3ZpZXc"}}},{"name":"excalidraw-read_checkpoint","_meta":{"ui":{"visibility":["app"]}}}]}}`)
	listed := addMCPAppAliasesToList([]byte(`{"method":"tools/list"}`), response, aliases, hasMCPAppSource(tools))
	var decoded struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
				Meta struct {
					UI map[string]any `json:"ui"`
				} `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(listed, &decoded))
	assert.Len(t, decoded.Result.Tools, 4)
	assert.NotEmpty(t, decoded.Result.Tools[0].Meta.UI["resourceUri"])
	for _, tool := range decoded.Result.Tools {
		if tool.Name == "read_checkpoint" {
			assert.Equal(t, []any{"app"}, tool.Meta.UI["visibility"])
		}
	}
}

func TestMCPAppInitializeAdvertisesUIExtension(t *testing.T) {
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"extensions":{"io.modelcontextprotocol/ui":{"mimeTypes":["text/html;profile=mcp-app"]}}}}}`)
	response := []byte(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{"listChanged":true}}}}`)
	var decoded struct {
		Result struct {
			Capabilities struct {
				Extensions map[string]struct {
					MIMETypes []string `json:"mimeTypes"`
				} `json:"extensions"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(addMCPAppInitializeCapability(request, response, true), &decoded))
	assert.Equal(t, []string{mcpAppMIME}, decoded.Result.Capabilities.Extensions["io.modelcontextprotocol/ui"].MIMETypes)
}

func TestMCPAppInitializeWithoutClientSupportDoesNotNegotiateUI(t *testing.T) {
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`)
	response := []byte(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}}}}`)
	got := addMCPAppInitializeCapability(request, response, true)
	var decoded struct {
		Result struct {
			Capabilities struct {
				Extensions map[string]any `json:"extensions"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(got, &decoded))
	assert.NotContains(t, decoded.Result.Capabilities.Extensions, "io.modelcontextprotocol/ui")
}

func TestMCPAppAggregateDoesNotNegotiateUI(t *testing.T) {
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{"extensions":{"io.modelcontextprotocol/ui":{"mimeTypes":["text/html;profile=mcp-app"]}}}}}`)
	response := []byte(`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}}}}`)
	got := addMCPAppInitializeCapability(request, response, false)
	assert.JSONEq(t, string(response), string(got))
}
