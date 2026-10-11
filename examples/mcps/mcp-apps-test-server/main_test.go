package main

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestResultRoundTripsThroughMCPGo(t *testing.T) {
	encoded, err := json.Marshal(result("instance-a", "hello"))
	if err != nil {
		t.Fatalf("marshal fixture result: %v", err)
	}

	var decoded mcp.CallToolResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("SDK rejected fixture result JSON %s: %v", encoded, err)
	}

	if len(decoded.Content) != 1 {
		t.Fatalf("decoded content count = %d, want 1", len(decoded.Content))
	}
	text, ok := decoded.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("decoded content type = %T, want mcp.TextContent", decoded.Content[0])
	}
	if text.Type != "text" || text.Text != `{"instance":"instance-a","message":"hello"}` {
		t.Fatalf("decoded text content = %#v, want type text and exact JSON payload", text)
	}

	structured, ok := decoded.StructuredContent.(map[string]any)
	if !ok || structured["instance"] != "instance-a" || structured["message"] != "hello" {
		t.Fatalf("decoded structured content = %#v, want exact fixture fields", decoded.StructuredContent)
	}
	if decoded.Meta == nil || decoded.Meta.AdditionalFields["fixture"] != "instance-a" {
		t.Fatalf("decoded _meta = %#v, want fixture instance identity", decoded.Meta)
	}
}
