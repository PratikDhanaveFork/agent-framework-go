// Copyright (c) Microsoft. All rights reserved.

package mcptool_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/microsoft/agent-framework-go/tool/mcptool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAddToolPreservesNativeMCPContent(t *testing.T) {
	text := &mcp.TextContent{Text: "Screenshot captured", Meta: mcp.Meta{"source": "browser"}}
	image := &mcp.ImageContent{
		Data: []byte{1, 2, 3}, MIMEType: "image/png",
		Meta:        mcp.Meta{"filename": "screenshot.png"},
		Annotations: &mcp.Annotations{Audience: []mcp.Role{"user"}, Priority: 0.5},
	}
	audio := &mcp.AudioContent{Data: []byte{4, 5, 6}, MIMEType: "audio/wav"}
	link := &mcp.ResourceLink{URI: "https://example.com/report.pdf", Name: "report.pdf", MIMEType: "application/pdf"}
	resource := &mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "file:///report.txt", Text: "Report", MIMEType: "text/plain"}}
	for _, tc := range []struct {
		name   string
		result any
		want   []mcp.Content
	}{
		{"text", text, []mcp.Content{text}},
		{"image", image, []mcp.Content{image}},
		{"audio", audio, []mcp.Content{audio}},
		{"resource link", link, []mcp.Content{link}},
		{"embedded resource", resource, []mcp.Content{resource}},
		{"mixed content", []mcp.Content{text, image}, []mcp.Content{text, image}},
		{"wrapped image", &mcp.CallToolResult{Content: []mcp.Content{image}}, []mcp.Content{image}},
		{"empty slice", []mcp.Content{}, []mcp.Content{}},
		{"nil slice", []mcp.Content(nil), []mcp.Content{}},
		{"typed nil", (*mcp.ImageContent)(nil), []mcp.Content{&mcp.TextContent{Text: "null"}}},
		{"nil entries", []mcp.Content{nil, (*mcp.ImageContent)(nil), image}, []mcp.Content{&mcp.TextContent{Text: "null"}, &mcp.TextContent{Text: "null"}, image}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, isSlice := tc.result.([]mcp.Content)
			snapshot := slices.Clone(original)
			server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
			mcptool.AddTool(server, stubFuncTool{
				name: "native_content", schema: map[string]any{"type": "object"},
				call: func(context.Context, string) (any, error) { return tc.result, nil },
			})
			session := connectInMemory(t, t.Context(), server)
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "native_content", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if isSlice && !slices.Equal(original, snapshot) {
				t.Fatal("conversion modified the caller's content slice")
			}
			if result.IsError || result.StructuredContent != nil {
				t.Fatalf("unexpected error or structured content: %#v", result)
			}
			if len(result.Content) != len(tc.want) {
				t.Fatalf("got %d content blocks, want %d", len(result.Content), len(tc.want))
			}
			for i, want := range tc.want {
				gotJSON, err := json.Marshal(result.Content[i])
				if err != nil {
					t.Fatal(err)
				}
				wantJSON, err := json.Marshal(want)
				if err != nil {
					t.Fatal(err)
				}
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("content[%d] = %s, want %s", i, gotJSON, wantJSON)
				}
			}
		})
	}
}
