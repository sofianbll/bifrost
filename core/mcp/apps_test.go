package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/stretchr/testify/require"
)

type appCaptureTransport struct {
	transport.Interface
	request transport.JSONRPCRequest
}

func (t *appCaptureTransport) SendRequest(_ context.Context, request transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	t.request = request
	return &transport.JSONRPCResponse{}, nil
}

func TestMCPAppInitializeCapabilityForwardedUpstream(t *testing.T) {
	upstream := &appCaptureTransport{}
	wrapped := appTransport{upstream}
	_, err := wrapped.SendRequest(context.Background(), transport.JSONRPCRequest{
		Method: "initialize",
		Params: map[string]any{"capabilities": map[string]any{"tools": map[string]any{}, "extensions": map[string]any{"example/other": map[string]any{"enabled": true}}}},
	})
	require.NoError(t, err)
	params := upstream.request.Params.(map[string]any)
	capabilities := params["capabilities"].(map[string]any)
	require.Contains(t, capabilities, "tools")
	extensions := capabilities["extensions"].(map[string]any)
	require.Contains(t, extensions, "example/other")
	ui := extensions["io.modelcontextprotocol/ui"].(map[string]any)
	require.Equal(t, []string{mcpAppMIME}, ui["mimeTypes"])
}
