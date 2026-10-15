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

func TestStdioReturnErrorPrefixPayload(t *testing.T) {
	responses := runErrorStdio(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"fixture-test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"return_error","arguments":{"error_type":"prefix"}}}`,
	)
	var call struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responses[1], &call); err != nil {
		t.Fatal(err)
	}
	if !call.Result.IsError || len(call.Result.Content) != 1 {
		t.Fatalf("tools/call response = %+v, want one error content item", call.Result)
	}
	if got, want := call.Result.Content[0].Text, "Error: synthetic fixture error value"; got != want {
		t.Fatalf("tools/call error text = %q, want %q", got, want)
	}
}

func runErrorStdio(t *testing.T, requests ...string) [][]byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdin = strings.NewReader(strings.Join(requests, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run error fixture: %v\n%s", err, stderr.String())
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
		t.Fatalf("stdio returned %d responses, want initialize and tools/call responses: %s", len(responses), stdout.String())
	}
	return responses
}
