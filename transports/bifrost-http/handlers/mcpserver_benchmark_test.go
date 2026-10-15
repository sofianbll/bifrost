package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore"
	"github.com/maximhq/bifrost/transports/v2/bifrost-http/lib"
)

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
