package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
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

// The admitted Virtual MCP tool set is the shared authorization boundary.
// Synthetic Code Mode tools must not disable native Apps on that connection.
func hasMCPAppSource(tools []schemas.ChatTool) bool {
	for _, tool := range tools {
		if tool.Function == nil {
			continue
		}
		if len(tool.MCPRawTool) == 0 {
			continue
		}
		_, uri, _, err := rewriteMCPAppTool(tool.MCPRawTool, tool.Function.Name)
		if err == nil && uri != "" {
			return true
		}
	}
	return false
}

func mcpAppClientName(publicToolName, originalToolName string) (string, bool) {
	clientName := strings.TrimSuffix(publicToolName, "-"+originalToolName)
	return clientName, originalToolName != "" && clientName != "" && clientName != publicToolName
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
		published, _, _, err := rewriteMCPAppTool(tool.MCPRawTool, tool.Function.Name)
		if err != nil {
			continue
		}
		var original mcp.Tool
		if json.Unmarshal(tool.MCPRawTool, &original) != nil {
			continue
		}
		// rewriteMCPAppTool decoded a fresh tool; these maps are privately owned.
		if published.Meta == nil {
			published.Meta = &mcp.Meta{}
		}
		if published.Meta.AdditionalFields == nil {
			published.Meta.AdditionalFields = make(map[string]any)
		}
		ui, _ := published.Meta.AdditionalFields["ui"].(map[string]any)
		if ui == nil {
			ui = make(map[string]any)
		}
		if visibility, explicit := ui["visibility"]; explicit {
			allowed := false
			if targets, ok := visibility.([]any); ok {
				for _, target := range targets {
					allowed = allowed || target == "app"
				}
			}
			if !allowed {
				continue
			}
		}
		counts[original.Name]++
		published.Name = original.Name
		ui["visibility"] = []string{"app"}
		published.Meta.AdditionalFields["ui"] = ui
		aliases[original.Name] = mcpAppToolAlias{publicName: tool.Function.Name, tool: published}
	}
	for name, count := range counts {
		if count != 1 || publicNames[name] {
			// Keep a tombstone so a collision cannot fall through to another public tool.
			aliases[name] = mcpAppToolAlias{}
		}
	}
	return aliases
}

func resolveMCPAppCallAlias(requestBody []byte, aliases map[string]mcpAppToolAlias) ([]byte, error) {
	if !json.Valid(requestBody) || providerUtils.GetJSONField(requestBody, "method").String() != "tools/call" {
		return requestBody, nil
	}
	alias, ok := aliases[providerUtils.GetJSONField(requestBody, "params.name").String()]
	if !ok {
		return requestBody, nil
	}
	if alias.publicName == "" {
		return nil, fmt.Errorf("ambiguous MCP App callback name; use a namespaced tool or a narrower Virtual MCP")
	}
	name, _ := json.Marshal(alias.publicName)
	updated, err := providerUtils.SetRawJSONField(requestBody, "params.name", name)
	if err != nil {
		return requestBody, nil
	}
	return updated, nil
}

func addMCPAppAliasesToList(requestBody, responseBody []byte, aliases map[string]mcpAppToolAlias, appSafe bool) []byte {
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
				if !appSafe {
					if meta, ok := tool["_meta"].(map[string]any); ok {
						if ui, ok := meta["ui"].(map[string]any); ok {
							delete(ui, "resourceUri")
						}
					}
				}
			}
		}
	}
	if !appSafe {
		aliases = nil
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
	clientName, ok := mcpAppClientName(publicToolName, tool.Name)
	if !ok {
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
	// Hosts using the compatibility field must read the same authorized gateway route.
	if _, exists := tool.Meta.AdditionalFields["ui/resourceUri"]; exists {
		tool.Meta.AdditionalFields["ui/resourceUri"] = publicURI
	}
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
func addMCPAppInitializeCapability(requestBody, responseBody []byte, appSafe bool) []byte {
	if !appSafe {
		return responseBody
	}
	var request struct {
		Method string `json:"method"`
		Params struct {
			Capabilities struct {
				Extensions map[string]struct {
					MIMETypes []string `json:"mimeTypes"`
				} `json:"extensions"`
			} `json:"capabilities"`
		} `json:"params"`
	}
	if json.Unmarshal(requestBody, &request) != nil || request.Method != "initialize" ||
		!slices.Contains(request.Params.Capabilities.Extensions["io.modelcontextprotocol/ui"].MIMETypes, mcpAppMIME) {
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
