package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
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

// The server's instructions are what reaches an agent that loaded no skill: the
// always-on rule, and the pointer to the skills.
func TestMCPServerInstructions(t *testing.T) {
	c, err := client.NewInProcessClient(newMCPServer(nil, nil, false))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "0"}
	res, err := c.Initialize(ctx, init)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"search t7kb before answering from memory", "mid-task too", "t7kb:* skills", "load the one that owns the task"} {
		if !strings.Contains(res.Instructions, want) {
			t.Errorf("instructions lack %q:\n%s", want, res.Instructions)
		}
	}
	if n := len(strings.Fields(res.Instructions)); n > 120 {
		t.Errorf("instructions are %d words: they load in every session, keep them near 100", n)
	}
}
