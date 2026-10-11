package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpproto "github.com/mark3labs/mcp-go/mcp"
	"github.com/maximhq/bifrost/core/schemas"
)

const mcpAppMIME = "text/html;profile=mcp-app"

// mcp-go v0.43.2 has no initialize.extensions field. Add the UI capability
// at the transport boundary while retaining its normal session handling.
type appTransport struct{ transport.Interface }

func (t appTransport) SendRequest(ctx context.Context, request transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	if request.Method == "initialize" {
		encoded, err := json.Marshal(request.Params)
		if err != nil {
			return nil, err
		}
		var params map[string]any
		if err := json.Unmarshal(encoded, &params); err != nil {
			return nil, err
		}
		capabilities, ok := params["capabilities"].(map[string]any)
		if !ok {
			capabilities = make(map[string]any)
			params["capabilities"] = capabilities
		}
		extensions, ok := capabilities["extensions"].(map[string]any)
		if !ok {
			extensions = make(map[string]any)
			capabilities["extensions"] = extensions
		}
		extensions["io.modelcontextprotocol/ui"] = map[string]any{"mimeTypes": []string{mcpAppMIME}}
		request.Params = params
	}
	return t.Interface.SendRequest(ctx, request)
}

func (t appTransport) SetProtocolVersion(version string) {
	if connection, ok := t.Interface.(transport.HTTPConnection); ok {
		connection.SetProtocolVersion(version)
	}
}

func (t appTransport) SetRequestHandler(handler transport.RequestHandler) {
	if connection, ok := t.Interface.(transport.BidirectionalInterface); ok {
		connection.SetRequestHandler(handler)
	}
}

func newMCPAppClient(connection transport.Interface) *client.Client {
	return client.NewClient(appTransport{connection})
}

// ExecuteNativeTool uses the same admission, connection and plugin path as
// ExecuteChatTool, while retaining the MCP result for MCP clients. If a plugin
// changes the text or error status, its final response takes precedence.
func (m *MCPManager) ExecuteNativeTool(ctx *schemas.BifrostContext, toolCall *schemas.ChatAssistantMessageToolCall) (*mcpproto.CallToolResult, *schemas.BifrostError) {
	if toolCall == nil {
		return mcpproto.NewToolResultError("tool call cannot be nil"), nil
	}
	req := getMCPRequest()
	req.RequestType = schemas.MCPRequestTypeChatToolCall
	req.ChatAssistantMessageToolCall = toolCall
	defer releaseMCPRequest(req)

	result, bErr := m.executeToolWithHooks(ctx, req, schemas.ChatCompletionRequest)
	if bErr != nil {
		return nil, bErr
	}
	if result == nil || result.ChatMessage == nil {
		return mcpproto.NewToolResultError("tool execution returned no result"), nil
	}
	finalText := ""
	if result.ChatMessage.Content != nil && result.ChatMessage.Content.ContentStr != nil {
		finalText = *result.ChatMessage.Content.ContentStr
	}
	finalError := result.ChatMessage.ChatToolMessage != nil && result.ChatMessage.ChatToolMessage.IsError != nil && *result.ChatMessage.ChatToolMessage.IsError
	if len(result.MCPRawResult) > 0 {
		var raw mcpproto.CallToolResult
		toolName := ""
		if toolCall.Function.Name != nil {
			toolName = *toolCall.Function.Name
		}
		if err := json.Unmarshal(result.MCPRawResult, &raw); err == nil &&
			finalText == extractTextFromMCPResponse(&raw, toolName) && finalError == raw.IsError {
			return &raw, nil
		}
	}
	fallback := mcpproto.NewToolResultText(finalText)
	fallback.IsError = finalError
	return fallback, nil
}

// ReadAppResource borrows the same upstream connection and applies the same
// tool-level permissions as the UI tool that advertised this resource.
func (m *MCPManager) ReadAppResource(ctx *schemas.BifrostContext, linkedToolName, originalURI string) (*mcpproto.ReadResourceResult, error) {
	if linkedToolName == "" || originalURI == "" {
		return nil, fmt.Errorf("app resource is not available")
	}
	req := &schemas.BifrostMCPRequest{
		RequestType: schemas.MCPRequestTypeChatToolCall,
		ChatAssistantMessageToolCall: &schemas.ChatAssistantMessageToolCall{
			Function: schemas.ChatAssistantMessageToolCallFunction{Name: &linkedToolName},
		},
	}
	state, conn, release, err := m.prepareToolExecution(ctx, req)
	if err != nil {
		return nil, err
	}
	defer release()
	if state == nil || conn == nil {
		return nil, fmt.Errorf("app resource is not available")
	}
	m.mu.RLock()
	tool, exists := state.ToolMap[linkedToolName]
	m.mu.RUnlock()
	if !exists || len(tool.MCPRawTool) == 0 {
		return nil, fmt.Errorf("app resource is not available")
	}
	var original mcpproto.Tool
	if err := json.Unmarshal(tool.MCPRawTool, &original); err != nil || original.Meta == nil {
		return nil, fmt.Errorf("app resource is not available")
	}
	ui, ok := original.Meta.AdditionalFields["ui"].(map[string]any)
	if !ok || ui["resourceUri"] != originalURI {
		return nil, fmt.Errorf("app resource is not available")
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return conn.ReadResource(readCtx, mcpproto.ReadResourceRequest{
		Params: mcpproto.ReadResourceParams{URI: originalURI},
	})
}
