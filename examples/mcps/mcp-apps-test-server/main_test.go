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
	maxState := strings.Repeat("x", maxSyntheticStateBytes)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fixture-test","version":"1"}}}`
	initialized := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	responses := runStdio(t, []string{"first", "dual"}, initialize, initialized,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"first-literal"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"changed-literal"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"first-literal"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"`+maxState+`"}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"`+maxState+`x"}}}`,
	)
	assertStateResponse(t, responses[1], "first:save_state", "first", "first-literal")
	assertStateResponse(t, responses[2], "first:read_state", "first", "first-literal")
	assertStateResponse(t, responses[3], "first:save_state", "first", "changed-literal")
	assertStateResponse(t, responses[4], "first:read_state", "first", "changed-literal")
	assertStateResponse(t, responses[5], "first:save_state", "first", "first-literal")
	assertStateResponse(t, responses[6], "first:read_state", "first", "first-literal")
	assertStateResponse(t, responses[7], "first:save_state", "first", maxState)
	assertStateResponse(t, responses[8], "first:read_state", "first", maxState)
	var tooLarge struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responses[9], &tooLarge); err != nil {
		t.Fatal(err)
	}
	if !tooLarge.Result.IsError || len(tooLarge.Result.Content) != 1 || tooLarge.Result.Content[0].Text != "state exceeds the 256-byte fixture limit" {
		t.Fatalf("257-byte state result = %+v, want the 256-byte limit error", tooLarge.Result)
	}

	otherSource := runStdio(t, []string{"second", "legacy"}, initialize, initialized,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_state","arguments":{"state":"second-literal"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_state","arguments":{}}}`,
	)
	assertStateResponse(t, otherSource[1], "second:read_state", "second", "")
	assertStateResponse(t, otherSource[2], "second:save_state", "second", "second-literal")
	assertStateResponse(t, otherSource[3], "second:read_state", "second", "second-literal")
}

func TestStdioResourceReturnsExecutablePinnedHostFixture(t *testing.T) {
	responses := runStdio(t, []string{"first", "dual", "mcp_apps_fixture"},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fixture-test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"ui://mcp-apps-test-server/view.html"}}`,
	)
	var read struct {
		Result struct {
			Contents []struct {
				URI      string                     `json:"uri"`
				MIMEType string                     `json:"mimeType"`
				Text     string                     `json:"text"`
				Meta     map[string]json.RawMessage `json:"_meta"`
			} `json:"contents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responses[1], &read); err != nil {
		t.Fatal(err)
	}
	if len(read.Result.Contents) != 1 {
		t.Fatalf("resources/read returned %d contents, want 1", len(read.Result.Contents))
	}
	content := read.Result.Contents[0]
	if content.URI != resourceURI || content.MIMEType != "text/html;profile=mcp-app" {
		t.Fatalf("resource identity = (%q, %q), want (%q, text/html;profile=mcp-app)", content.URI, content.MIMEType, resourceURI)
	}
	for _, want := range []string{
		`data-testid="mcp-app-fixture"`,
		`data-save-tool="mcp_apps_fixture-save_state"`,
		`data-read-tool="mcp_apps_fixture-read_state"`,
		`http://localhost:8080/mcp-app-fixture.js`,
		`default-src 'none'; script-src http://localhost:8080`,
		`data-testid="change-state"`,
		`data-testid="restore-state"`,
		`data-testid="corrupt-state"`,
	} {
		if !strings.Contains(content.Text, want) {
			t.Errorf("resource HTML missing %q", want)
		}
	}
	var meta struct {
		UI struct {
			CSP struct {
				ResourceDomains []string `json:"resourceDomains"`
				ConnectDomains  []string `json:"connectDomains"`
			} `json:"csp"`
		} `json:"ui"`
	}
	if err := json.Unmarshal(content.Meta["ui"], &meta.UI); err != nil {
		t.Fatalf("resource _meta.ui: %v", err)
	}
	if len(meta.UI.CSP.ResourceDomains) != 1 || meta.UI.CSP.ResourceDomains[0] != "http://localhost:8080" {
		t.Fatalf("resource domains = %v, want only the pinned local host asset origin", meta.UI.CSP.ResourceDomains)
	}
	if len(meta.UI.CSP.ConnectDomains) != 0 {
		t.Fatalf("connect domains = %v, want none", meta.UI.CSP.ConnectDomains)
	}
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
