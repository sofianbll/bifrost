package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

func runStdio(t *testing.T, args []string, requests ...string) [][]byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "."}, args...)...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdin = strings.NewReader(strings.Join(requests, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run fixture: %v\n%s", err, stderr.String())
	}
	scanner := bufio.NewScanner(&stdout)
	var responses [][]byte
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) != 0 {
			responses = append(responses, bytes.Clone(line))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(responses) != 2 {
		t.Fatalf("stdio returned %d responses, want initialize and tools/call/list responses: %s", len(responses), stdout.String())
	}
	return responses
}
