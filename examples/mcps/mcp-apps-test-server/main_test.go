package main

import (
	"reflect"
	"testing"
)

func TestAppToolMetadataVariants(t *testing.T) {
	tests := []struct {
		name string
		want map[string]any
	}{
		{
			name: "legacy",
			want: map[string]any{"ui/resourceUri": "ui://mcp-apps-test-server/view.html"},
		},
		{
			name: "modern",
			want: map[string]any{"ui": map[string]any{"resourceUri": "ui://mcp-apps-test-server/view.html"}},
		},
		{
			name: "dual",
			want: map[string]any{
				"ui":             map[string]any{"resourceUri": "ui://mcp-apps-test-server/view.html"},
				"ui/resourceUri": "ui://mcp-apps-test-server/view.html",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := appToolMetadata(test.name)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("metadata = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestAppToolMetadataRejectsUnknownVariant(t *testing.T) {
	if _, err := appToolMetadata("unexpected"); err == nil {
		t.Fatal("unknown metadata variant was accepted")
	}
}
