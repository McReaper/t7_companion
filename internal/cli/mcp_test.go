package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func callBuildTool(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = "build"
	req.Params.Arguments = args
	res, err := buildToolHandler()(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned a transport error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned a nil result")
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestBuildToolHandler_MissingName(t *testing.T) {
	res := callBuildTool(t, map[string]any{})
	if !res.IsError {
		t.Fatal("expected an error result when name is missing")
	}
}

func TestBuildToolHandler_NoToolsPath(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	res := callBuildTool(t, map[string]any{"name": "zm_test"})
	if !res.IsError {
		t.Fatal("expected an error result when no mod-tools path is available")
	}
	if txt := resultText(res); !strings.Contains(txt, "mod-tools path") {
		t.Fatalf("unexpected error text: %q", txt)
	}
}
