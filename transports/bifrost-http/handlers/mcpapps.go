package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/maximhq/bifrost/core/schemas"
)

const mcpAppMIME = "text/html;profile=mcp-app"

// MCPAppManager is the native MCP path behind the hosted /mcp endpoint.
type MCPAppManager interface {
	ExecuteNativeMCPTool(context.Context, *schemas.ChatAssistantMessageToolCall) (*mcp.CallToolResult, *schemas.BifrostError)
	ReadMCPAppResource(context.Context, string, string) (*mcp.ReadResourceResult, error)
}

type mcpAppResourceRoute struct {
	originalURI string
	toolNames   []string
}

type mcpAppToolAlias struct {
	publicName string
	tool       mcp.Tool
}

// Aliases are derived from the caller's admitted tools, so equal callback
// names in different upstream apps work on their separate /mcp/<slug> routes.
func collectMCPAppAliases(tools []schemas.ChatTool) map[string]mcpAppToolAlias {
	aliases := make(map[string]mcpAppToolAlias)
	counts := make(map[string]int)
	publicNames := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool.Function != nil {
			publicNames[tool.Function.Name] = true
		}
	}
	for _, tool := range tools {
		if tool.Function == nil || len(tool.MCPRawTool) == 0 {
			continue
		}
		published, sourceURI, _, err := rewriteMCPAppTool(tool.MCPRawTool, tool.Function.Name)
		if err != nil || (!tool.MCPAppOnly && sourceURI == "") {
			continue
		}
		var original mcp.Tool
		if json.Unmarshal(tool.MCPRawTool, &original) != nil || publicNames[original.Name] {
			continue
		}
		counts[original.Name]++
		published.Name = original.Name
		meta := &mcp.Meta{AdditionalFields: make(map[string]any)}
		if published.Meta != nil {
			meta.ProgressToken = published.Meta.ProgressToken
			for key, value := range published.Meta.AdditionalFields {
				meta.AdditionalFields[key] = value
			}
		}
		ui := map[string]any{"visibility": []string{"app"}}
		if originalUI, ok := meta.AdditionalFields["ui"].(map[string]any); ok {
			for key, value := range originalUI {
				ui[key] = value
			}
		}
		ui["visibility"] = []string{"app"}
		meta.AdditionalFields["ui"] = ui
		published.Meta = meta
		aliases[original.Name] = mcpAppToolAlias{publicName: tool.Function.Name, tool: published}
	}
	for name, count := range counts {
		if count != 1 {
			delete(aliases, name)
		}
	}
	return aliases
}

func resolveMCPAppCallAlias(requestBody []byte, aliases map[string]mcpAppToolAlias) []byte {
	var request map[string]any
	if json.Unmarshal(requestBody, &request) != nil || request["method"] != "tools/call" {
		return requestBody
	}
	params, ok := request["params"].(map[string]any)
	if !ok {
		return requestBody
	}
	name, _ := params["name"].(string)
	alias, ok := aliases[name]
	if !ok {
		return requestBody
	}
	params["name"] = alias.publicName
	updated, err := json.Marshal(request)
	if err != nil {
		return requestBody
	}
	return updated
}

func addMCPAppAliasesToList(requestBody, responseBody []byte, aliases map[string]mcpAppToolAlias) []byte {
	if len(aliases) == 0 {
		return responseBody
	}
	var request struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(requestBody, &request) != nil || request.Method != "tools/list" {
		return responseBody
	}
	var response map[string]any
	if json.Unmarshal(responseBody, &response) != nil {
		return responseBody
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return responseBody
	}
	listed, ok := result["tools"].([]any)
	if !ok {
		return responseBody
	}
	listedNames := make(map[string]bool, len(listed))
	for _, item := range listed {
		if tool, ok := item.(map[string]any); ok {
			if name, ok := tool["name"].(string); ok {
				listedNames[name] = true
			}
		}
	}
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !listedNames[aliases[name].publicName] {
			continue
		}
		encoded, err := json.Marshal(aliases[name].tool)
		if err != nil {
			continue
		}
		var item any
		if json.Unmarshal(encoded, &item) == nil {
			listed = append(listed, item)
		}
	}
	result["tools"] = listed
	updated, err := json.Marshal(response)
	if err != nil {
		return responseBody
	}
	return updated
}

func rewriteMCPAppTool(raw json.RawMessage, publicToolName string) (mcp.Tool, string, string, error) {
	var tool mcp.Tool
	if err := json.Unmarshal(raw, &tool); err != nil {
		return tool, "", "", err
	}
	clientName := strings.TrimSuffix(publicToolName, "-"+tool.Name)
	if clientName == publicToolName || clientName == "" {
		return tool, "", "", fmt.Errorf("tool name does not match its MCP client")
	}
	tool.Name = publicToolName
	if tool.Meta == nil {
		return tool, "", "", nil
	}
	ui, ok := tool.Meta.AdditionalFields["ui"].(map[string]any)
	if !ok {
		return tool, "", "", nil
	}
	originalURI, ok := ui["resourceUri"].(string)
	if !ok || !strings.HasPrefix(originalURI, "ui://") {
		return tool, "", "", nil
	}
	publicURI := "ui://bifrost/" + base64.RawURLEncoding.EncodeToString([]byte(clientName)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(originalURI))
	ui["resourceUri"] = publicURI
	return tool, originalURI, publicURI, nil
}

func rewriteMCPAppContents(contents []mcp.ResourceContents, publicURI string) []mcp.ResourceContents {
	for i, content := range contents {
		switch value := content.(type) {
		case mcp.TextResourceContents:
			value.URI = publicURI
			contents[i] = value
		case mcp.BlobResourceContents:
			value.URI = publicURI
			contents[i] = value
		}
	}
	return contents
}

// mcp-go v0.43.2 has no extensions field on InitializeResult. Preserve the
// pinned SDK and add the standard MCP Apps capability to its wire response.
func addMCPAppInitializeCapability(requestBody, responseBody []byte) []byte {
	var request struct {
		Method string `json:"method"`
	}
	if json.Unmarshal(requestBody, &request) != nil || request.Method != "initialize" {
		return responseBody
	}
	var response map[string]any
	if json.Unmarshal(responseBody, &response) != nil {
		return responseBody
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return responseBody
	}
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		return responseBody
	}
	extensions, ok := capabilities["extensions"].(map[string]any)
	if !ok {
		extensions = make(map[string]any)
		capabilities["extensions"] = extensions
	}
	extensions["io.modelcontextprotocol/ui"] = map[string]any{"mimeTypes": []string{mcpAppMIME}}
	updated, err := json.Marshal(response)
	if err != nil {
		return responseBody
	}
	return updated
}
