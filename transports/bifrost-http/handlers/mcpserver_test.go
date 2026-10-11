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

type mcpServerResultTestToolManager struct {
	message *schemas.ChatMessage
}

func (m mcpServerResultTestToolManager) GetAvailableMCPTools(context.Context) []schemas.ChatTool {
	return nil
}
func (m mcpServerResultTestToolManager) ExecuteChatMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.BifrostError) {
	return m.message, nil
}
func (m mcpServerResultTestToolManager) ExecuteResponsesMCPTool(context.Context, *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.BifrostError) {
	return nil, nil
}

func TestBuildServerPreservesChatToolError(t *testing.T) {
	SetLogger(&mockLogger{})
	text := "Validation Error: invalid input"
	isError := true
	toolName := "return_error"
	tool := schemas.ChatTool{Function: &schemas.ChatToolFunction{Name: toolName}}
	handler := &MCPServerHandler{toolManager: mcpServerResultTestToolManager{message: &schemas.ChatMessage{
		Content:         &schemas.ChatMessageContent{ContentStr: &text},
		ChatToolMessage: &schemas.ChatToolMessage{IsError: &isError},
	}}}
	mcpServer := handler.buildServer([]schemas.ChatTool{tool})
	response := mcpServer.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"return_error","arguments":{}}}`))
	responseJSON, err := json.Marshal(response)
	require.NoError(t, err)
	var result struct {
		Result mcp.CallToolResult `json:"result"`
	}
	require.NoError(t, json.Unmarshal(responseJSON, &result))
	assert.True(t, result.Result.IsError)
	require.Len(t, result.Result.Content, 1)
	assert.Equal(t, text, result.Result.Content[0].(mcp.TextContent).Text)
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
