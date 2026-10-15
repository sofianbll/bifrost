package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStdioToolListMetadataVariants(t *testing.T) {
	resourceURI := "ui://mcp-apps-test-server/view.html"
	tests := []struct {
		variant string
		legacy  bool
		modern  bool
	}{
		{variant: "legacy", legacy: true},
		{variant: "modern", modern: true},
		{variant: "dual", legacy: true, modern: true},
	}
	for _, test := range tests {
		t.Run(test.variant, func(t *testing.T) {
			responses := runStdio(t, []string{"second", test.variant},
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fixture-test","version":"1"}}}`,
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
				`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
			)
			var list struct {
				Result struct {
					Tools []struct {
						Name string                     `json:"name"`
						Meta map[string]json.RawMessage `json:"_meta"`
					} `json:"tools"`
				} `json:"result"`
			}
			if err := json.Unmarshal(responses[1], &list); err != nil {
				t.Fatal(err)
			}
			var openView map[string]json.RawMessage
			for _, tool := range list.Result.Tools {
				if tool.Name == "open_view" {
					openView = tool.Meta
					break
				}
			}
			if openView == nil {
				t.Fatal("tools/list did not include open_view metadata")
			}
			_, hasLegacy := openView["ui/resourceUri"]
			if hasLegacy != test.legacy {
				t.Fatalf("legacy metadata present=%v, want %v", hasLegacy, test.legacy)
			}
			var modern map[string]string
			if raw, ok := openView["ui"]; ok {
				if err := json.Unmarshal(raw, &modern); err != nil {
					t.Fatal(err)
				}
			}
			if (modern != nil) != test.modern {
				t.Fatalf("modern metadata present=%v, want %v", modern != nil, test.modern)
			}
			if test.legacy {
				var uri string
				if err := json.Unmarshal(openView["ui/resourceUri"], &uri); err != nil || uri != resourceURI {
					t.Fatalf("legacy resource URI=%q, error=%v", uri, err)
				}
			}
			if test.modern && modern["resourceUri"] != resourceURI {
				t.Fatalf("modern resource URI=%q, want %q", modern["resourceUri"], resourceURI)
			}
		})
	}
}

func TestStdioStateCallbacksPersistWithinOneFixtureProcess(t *testing.T) {
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fixture-test","version":"1"}}}`
	initialized := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	responses := runStdio(t, []string{"first", "dual"}, initialize, initialized,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"first-literal"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"changed-literal"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"first-literal"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
	)
	assertStateResponse(t, responses[1], "first:save_state", "first", "first-literal")
	assertStateResponse(t, responses[2], "first:read_state", "first", "first-literal")
	assertStateResponse(t, responses[3], "first:save_state", "first", "changed-literal")
	assertStateResponse(t, responses[4], "first:read_state", "first", "changed-literal")
	assertStateResponse(t, responses[5], "first:save_state", "first", "first-literal")
	assertStateResponse(t, responses[6], "first:read_state", "first", "first-literal")

	otherSource := runStdio(t, []string{"second", "legacy"}, initialize, initialized,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"second-literal"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
	)
	assertStateResponse(t, otherSource[1], "second:read_state", "second", "")
	assertStateResponse(t, otherSource[2], "second:save_state", "second", "second-literal")
	assertStateResponse(t, otherSource[3], "second:read_state", "second", "second-literal")
}

func assertStateResponse(t *testing.T, response []byte, wantText, wantInstance, wantState string) {
	t.Helper()
	var call struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent struct {
				Instance string `json:"instance"`
				State    string `json:"state"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &call); err != nil {
		t.Fatal(err)
	}
	if len(call.Result.Content) != 1 || call.Result.Content[0].Text != wantText {
		t.Fatalf("state callback text = %+v, want %q", call.Result.Content, wantText)
	}
	if call.Result.StructuredContent.Instance != wantInstance || call.Result.StructuredContent.State != wantState {
		t.Fatalf("state callback structured content = %+v, want instance=%q state=%q",
			call.Result.StructuredContent, wantInstance, wantState)
	}
}

func runStdio(t *testing.T, args []string, requests ...string) [][]byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "."}, args...)...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("go run fixture: %v\n%s", err, stderr.String())
	}
	reader := bufio.NewReader(stdout)
	var responses [][]byte
	for _, request := range requests {
		if _, err := fmt.Fprintln(stdin, request); err != nil {
			t.Fatalf("write MCP request: %v", err)
		}
		var message struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal([]byte(request), &message); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(message.Method, "notifications/") {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				t.Fatalf("read MCP response: %v\n%s", err, stderr.String())
			}
			responses = append(responses, bytes.TrimSpace(line))
		}
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("go run fixture: %v\n%s", err, stderr.String())
	}
	if len(responses) != len(requests)-1 {
		t.Fatalf("stdio returned %d responses, want %d non-notification responses", len(responses), len(requests)-1)
	}
	return responses
}
