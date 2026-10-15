package configstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	bifrostmcp "github.com/maximhq/bifrost/core/mcp"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateMCPClientTools_TargetedColumnUpdate pins that UpdateMCPClientTools
// only ever touches discovered_tools_json/tool_name_mapping_json — unlike
// UpdateMCPClientConfig's full-row overwrite, a periodic tool-sync writer
// using this method can never clobber an unrelated field a concurrent config
// edit just wrote (e.g. Name).
func TestUpdateMCPClientTools_TargetedColumnUpdate(t *testing.T) {
	s := setupRDBTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateMCPClientConfig(ctx, &schemas.MCPClientConfig{
		ID:               "mcp-tools-client",
		Name:             "original_name",
		ConnectionType:   schemas.MCPConnectionTypeHTTP,
		ConnectionString: schemas.NewSecretVar("https://example.invalid/mcp"),
	}))

	tools := map[string]schemas.ChatTool{
		"echo": {Type: "function"},
		"app-open": {
			Type:     "function",
			Function: &schemas.ChatToolFunction{Name: "app-open"},
			MCPApp: &schemas.MCPAppToolMetadata{
				RawDefinition: []byte(`{"name":"open","inputSchema":{"type":"object","properties":{}},"_meta":{"ui":{"resourceUri":"ui://app/open"}}}`),
				ResourceURI:   "ui://app/open",
			},
		},
	}
	mapping := map[string]string{"echo": "echo-server"}

	require.NoError(t, s.UpdateMCPClientTools(ctx, "mcp-tools-client", tools, mapping, ""))

	got, err := s.GetMCPClientByID(ctx, "mcp-tools-client")
	require.NoError(t, err)
	assert.Equal(t, tools, got.DiscoveredTools)
	// The database keeps the original flat map and an additive per-tool cache sidecar.
	var stockShape map[string]struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	var stored tables.TableMCPClient
	require.NoError(t, s.DB().WithContext(ctx).Where("client_id = ?", "mcp-tools-client").Take(&stored).Error)
	require.NoError(t, json.Unmarshal([]byte(stored.DiscoveredToolsJSON), &stockShape))
	assert.Equal(t, "app-open", stockShape["app-open"].Function.Name)
	assert.Equal(t, "function", stockShape["app-open"].Type)
	assert.Equal(t, mapping, got.DiscoveredToolNameMapping)
	// Untouched column proves this was a targeted update, not a full-row one.
	assert.Equal(t, "original_name", got.Name)
}

// TestUpdateMCPClientTools_EmptyMapPersistsAsLegitimateZeroTools confirms an
// empty (non-nil) result is written as-is — "the server has zero tools" is a
// real, distinct outcome from "never discovered", and must round-trip rather
// than being silently skipped.
func TestUpdateMCPClientTools_EmptyMapPersistsAsLegitimateZeroTools(t *testing.T) {
	s := setupRDBTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateMCPClientConfig(ctx, &schemas.MCPClientConfig{
		ID:               "mcp-empty-tools-client",
		Name:             "empty_tools_client",
		ConnectionType:   schemas.MCPConnectionTypeHTTP,
		ConnectionString: schemas.NewSecretVar("https://example.invalid/mcp"),
		DiscoveredTools:  map[string]schemas.ChatTool{"stale": {Type: "function"}},
	}))

	require.NoError(t, s.UpdateMCPClientTools(ctx, "mcp-empty-tools-client", map[string]schemas.ChatTool{}, map[string]string{}, ""))

	got, err := s.GetMCPClientByID(ctx, "mcp-empty-tools-client")
	require.NoError(t, err)
	assert.Empty(t, got.DiscoveredTools)
}

// TestUpdateMCPClientTools_UnknownClientReturnsNotFound mirrors
// UpdateMCPClientOAuthConfigID's own not-found contract for the same
// Updates()+RowsAffected==0 pattern.
func TestUpdateMCPClientTools_UnknownClientReturnsNotFound(t *testing.T) {
	s := setupRDBTestStore(t)
	ctx := context.Background()

	err := s.UpdateMCPClientTools(ctx, "does-not-exist", map[string]schemas.ChatTool{}, map[string]string{}, "")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestMCPClientConfigUnknownIDReturnsNotFound pins that deleting or updating an MCP
// client id with no row reports ErrNotFound, so a handler can answer 404 rather than
// 500, while the message still names the id.
func TestMCPClientConfigUnknownIDReturnsNotFound(t *testing.T) {
	s := setupRDBTestStore(t)
	ctx := context.Background()

	err := s.DeleteMCPClientConfig(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorContains(t, err, "does-not-exist")

	err = s.UpdateMCPClientConfig(ctx, "does-not-exist", &tables.TableMCPClient{Name: "renamed"})
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorContains(t, err, "does-not-exist")
}

// TestMCPClientConfig_OpenAPIConfigRoundTripsThroughStore covers the three
// store paths an openapi client's config travels: create, read back (both
// GetMCPConfig and GetMCPClientByID) and the map-based update, including the
// PATCH convention that a nil openapi_config preserves the stored one.
func TestMCPClientConfig_OpenAPIConfigRoundTripsThroughStore(t *testing.T) {
	s := setupRDBTestStore(t)
	ctx := context.Background()
	base := "https://petstore.example.com/v1"
	spec := "openapi: 3.0.3\ninfo: {title: Petstore, version: 1.0.0}\npaths: {}\n"

	require.NoError(t, s.CreateMCPClientConfig(ctx, &schemas.MCPClientConfig{
		ID:             "openapi-store-1",
		Name:           "petstore",
		ConnectionType: schemas.MCPConnectionTypeOpenAPI,
		AuthType:       schemas.MCPAuthTypeNone,
		ToolsToExecute: schemas.WhiteList{"listPets"},
		OpenAPIConfig: &schemas.MCPOpenAPIConfig{
			Spec:                spec,
			BaseURL:             &base,
			SecurityCredentials: map[string]schemas.MCPOpenAPICredential{"ApiKeyAuth": {Value: schemas.NewSecretVar("k-1")}},
			SpecTitle:           "Petstore",
			OperationCount:      3,
		},
	}))

	byID, err := s.GetMCPClientByID(ctx, "openapi-store-1")
	require.NoError(t, err)
	require.NotNil(t, byID.OpenAPIConfig)
	assert.Equal(t, spec, byID.OpenAPIConfig.Spec)
	assert.Equal(t, "k-1", byID.OpenAPIConfig.SecurityCredentials["ApiKeyAuth"].Value.GetValue())
	assert.Equal(t, 3, byID.OpenAPIConfig.OperationCount)

	all, err := s.GetMCPConfig(ctx)
	require.NoError(t, err)
	var found *schemas.MCPClientConfig
	for _, c := range all.ClientConfigs {
		if c.ID == "openapi-store-1" {
			found = c
		}
	}
	require.NotNil(t, found)
	require.NotNil(t, found.OpenAPIConfig, "GetMCPConfig carries openapi_config")
	assert.Equal(t, base, *found.OpenAPIConfig.BaseURL)

	// A PATCH-style update without openapi_config keeps the stored block.
	row, err := s.GetMCPClientByName(ctx, "petstore")
	require.NoError(t, err)
	row.OpenAPIConfig = nil
	row.ToolsToExecute = schemas.WhiteList{"listPets", "getPetById"}
	require.NoError(t, s.UpdateMCPClientConfig(ctx, "openapi-store-1", row))
	afterPatch, err := s.GetMCPClientByID(ctx, "openapi-store-1")
	require.NoError(t, err)
	require.NotNil(t, afterPatch.OpenAPIConfig, "nil openapi_config on update preserves the stored one")
	assert.Equal(t, spec, afterPatch.OpenAPIConfig.Spec)
	assert.Equal(t, schemas.WhiteList{"listPets", "getPetById"}, afterPatch.ToolsToExecute)

	// A replacement spec and credential set lands in both columns.
	newBase := "https://other.example.com"
	row.OpenAPIConfig = &schemas.MCPOpenAPIConfig{
		Spec:                spec + "# v2\n",
		BaseURL:             &newBase,
		SecurityCredentials: map[string]schemas.MCPOpenAPICredential{"ApiKeyAuth": {Value: schemas.NewSecretVar("k-2")}},
		OperationCount:      4,
	}
	require.NoError(t, s.UpdateMCPClientConfig(ctx, "openapi-store-1", row))
	afterReplace, err := s.GetMCPClientByID(ctx, "openapi-store-1")
	require.NoError(t, err)
	assert.Equal(t, spec+"# v2\n", afterReplace.OpenAPIConfig.Spec)
	assert.Equal(t, newBase, *afterReplace.OpenAPIConfig.BaseURL)
	assert.Equal(t, "k-2", afterReplace.OpenAPIConfig.SecurityCredentials["ApiKeyAuth"].Value.GetValue())
	assert.Equal(t, 4, afterReplace.OpenAPIConfig.OperationCount)
}

func TestMCPAppToolMetadataSurvivesRefreshAndManagerReload(t *testing.T) {
	ctx := context.Background()
	store := setupRDBTestStore(t)
	const (
		clientID   = "cache-lifecycle-app"
		clientName = "cache_lifecycle"
		oldURI     = "ui://catalog/old"
		newURI     = "ui://catalog/new"
	)

	upstream := server.NewMCPServer("cache-lifecycle-fixture", "1.0.0", server.WithToolCapabilities(true))
	toolHandler := func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultText("opened"), nil
	}
	upstream.AddTool(cacheLifecycleAppTool(oldURI), toolHandler)
	streamable := server.NewStreamableHTTPServer(upstream)
	httpServer := httptest.NewServer(http.HandlerFunc(streamable.ServeHTTP))
	t.Cleanup(func() {
		httpServer.CloseClientConnections()
		httpServer.Close()
	})

	config := &schemas.MCPClientConfig{
		ID:                     clientID,
		Name:                   clientName,
		ConnectionType:         schemas.MCPConnectionTypeHTTP,
		AuthType:               schemas.MCPAuthTypeNone,
		ConnectionString:       schemas.NewSecretVar(httpServer.URL),
		ToolsToExecute:         schemas.WhiteList{"*"},
		NeedsSessionStickiness: schemas.Ptr(false),
	}
	require.NoError(t, store.CreateMCPClientConfig(ctx, config))

	manager := bifrostmcp.NewMCPManager(ctx, schemas.MCPConfig{}, nil, nil, nil)
	t.Cleanup(func() { require.NoError(t, manager.Cleanup()) })
	var callbackMu sync.Mutex
	var callbackWrites int
	var callbackErr error
	manager.SetToolsChangeCallback(func(id, _ string, tools map[string]schemas.ChatTool, mapping map[string]string, instructions string) {
		err := store.UpdateMCPClientTools(ctx, id, tools, mapping, instructions)
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbackWrites++
		if err != nil {
			callbackErr = err
		}
	})
	assertCallbackWrites := func(want int) {
		t.Helper()
		callbackMu.Lock()
		defer callbackMu.Unlock()
		require.NoError(t, callbackErr)
		assert.Equal(t, want, callbackWrites)
	}

	// AddClient performs a real tools/list request. The callback writes its
	// discovered catalog through the SQLite ConfigStore API.
	require.NoError(t, manager.AddClient(ctx, config))
	assertCallbackWrites(1)
	initial, err := store.GetMCPClientConfigByID(ctx, clientID)
	require.NoError(t, err)
	initialTool := requireCacheLifecycleAppTool(t, initial.DiscoveredTools, clientName+"-open_view")
	assert.Equal(t, oldURI, initialTool.MCPApp.ResourceURI)
	assert.Contains(t, string(initialTool.MCPApp.RawDefinition), oldURI)
	assert.Equal(t, []string{"app"}, initialTool.MCPApp.Visibility)
	assert.Equal(t, "Open application", *initialTool.Function.Description)

	initialRow, err := store.GetMCPClientByID(ctx, clientID)
	require.NoError(t, err)
	count, err := manager.RefreshClientTools(ctx, clientID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assertCallbackWrites(1)
	unchangedRow, err := store.GetMCPClientByID(ctx, clientID)
	require.NoError(t, err)
	assert.Equal(t, initialRow.UpdatedAt, unchangedRow.UpdatedAt,
		"an unchanged tools/list must not issue another cache write")

	// Only the UI URI changes: name, description and input schema are the same.
	// This still changes the public native catalog and therefore must persist.
	upstream.AddTool(cacheLifecycleAppTool(newURI), toolHandler)
	count, err = manager.RefreshClientTools(ctx, clientID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assertCallbackWrites(2)

	reloaded, err := store.GetMCPConfig(ctx)
	require.NoError(t, err)
	var reloadedClient *schemas.MCPClientConfig
	for _, candidate := range reloaded.ClientConfigs {
		if candidate.ID == clientID {
			reloadedClient = candidate
			break
		}
	}
	require.NotNil(t, reloadedClient)
	reloadedTool := requireCacheLifecycleAppTool(t, reloadedClient.DiscoveredTools, clientName+"-open_view")
	assert.Equal(t, newURI, reloadedTool.MCPApp.ResourceURI)
	assert.Contains(t, string(reloadedTool.MCPApp.RawDefinition), newURI)
	assert.Equal(t, []string{"app"}, reloadedTool.MCPApp.Visibility)
	assert.Equal(t, "Open application", *reloadedTool.Function.Description)
	assert.Equal(t, initialTool.Function.Parameters, reloadedTool.Function.Parameters,
		"metadata-only refresh must leave the ordinary tool schema unchanged")

	// The second manager is restored from the ConfigStore cache after the
	// upstream goes away. This proves catalog retention does not depend on a
	// fresh tools/list succeeding at startup.
	httpServer.CloseClientConnections()
	httpServer.Close()
	restarted := bifrostmcp.NewMCPManager(ctx, schemas.MCPConfig{}, nil, nil, nil)
	t.Cleanup(func() { require.NoError(t, restarted.Cleanup()) })
	require.NoError(t, restarted.AddClient(ctx, reloadedClient))
	publicCatalog := restarted.GetAvailableToolsForMCP(schemas.NewBifrostContext(ctx, schemas.NoDeadline))
	publicTool := findCacheLifecycleTool(publicCatalog, clientName+"-open_view")
	require.NotNil(t, publicTool, "reloaded manager must serve the cached app tool")
	require.NotNil(t, publicTool.MCPApp)
	assert.Equal(t, newURI, publicTool.MCPApp.ResourceURI)
	assert.Contains(t, string(publicTool.MCPApp.RawDefinition), newURI)
	assert.Equal(t, []string{"app"}, publicTool.MCPApp.Visibility)
	assert.Equal(t, "Open application", *publicTool.Function.Description)
	assert.Equal(t, initialTool.Function.Parameters, publicTool.Function.Parameters)
}

func cacheLifecycleAppTool(uri string) mcpgo.Tool {
	tool := mcpgo.NewTool("open_view",
		mcpgo.WithDescription("Open application"),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Search query")),
	)
	tool.Meta = &mcpgo.Meta{AdditionalFields: map[string]any{
		"ui": map[string]any{
			"resourceUri": uri,
			"visibility":  []string{"app"},
		},
	}}
	return tool
}

func requireCacheLifecycleAppTool(t *testing.T, tools map[string]schemas.ChatTool, name string) schemas.ChatTool {
	t.Helper()
	tool, ok := tools[name]
	require.True(t, ok, "discovered catalog must include %s", name)
	require.NotNil(t, tool.Function)
	require.NotNil(t, tool.Function.Description)
	require.NotNil(t, tool.Function.Parameters)
	require.NotNil(t, tool.MCPApp)
	return tool
}

func findCacheLifecycleTool(tools []schemas.ChatTool, name string) *schemas.ChatTool {
	for i := range tools {
		if tools[i].Function != nil && tools[i].Function.Name == name {
			return &tools[i]
		}
	}
	return nil
}
