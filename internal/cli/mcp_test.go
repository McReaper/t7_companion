package cli

import (
	"context"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// mcpClient is an initialized in-process client of the server runMCP serves.
func mcpClient(t *testing.T) (*client.Client, *mcp.InitializeResult) {
	t.Helper()
	c, err := client.NewInProcessClient(newMCPServer(nil, nil, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
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
	return c, res
}

// Every tool's schema refuses an argument it doesn't declare, and the server
// checks each call against it: a misspelt or mistyped argument is an error
// naming it, never dropped for its default.
func TestMCPRejectsUndeclaredAndMistypedArguments(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "") // a call that passes must stop at the handler's own check
	t.Setenv("TA_GAME_PATH", "")
	c, _ := mcpClient(t)
	ctx := context.Background()
	list, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) == 0 {
		t.Fatal("the server lists no tools")
	}
	for _, tool := range list.Tools {
		if tool.InputSchema.AdditionalProperties != false {
			t.Errorf("tool %q accepts undeclared arguments (additionalProperties %v)", tool.Name, tool.InputSchema.AdditionalProperties)
		}
	}
	for _, tc := range []struct {
		tool       string
		args       any
		want, also string // the error starts with want, and holds also
	}{
		{"build", map[string]any{"name": "zm_x", "stage": "link"}, `build: unknown argument "stage" (it takes dvars, `, " stages, "},
		{"build", map[string]any{"name": 5}, "build: argument name: got number, want string", ""},
		{"build", nil, `build: missing argument "name"`, ""},
		{"zone_explain", map[string]any{"name": "zm_x"}, `zone_explain: missing argument "asset"`, ""},
		{"build", map[string]any{"name": "zm_x", "stages": "link"}, "no mod-tools path", ""}, // valid: the handler answers
	} {
		req := mcp.CallToolRequest{}
		req.Params.Name = tc.tool
		req.Params.Arguments = tc.args
		res, err := c.CallTool(ctx, req)
		if err != nil {
			t.Fatalf("%s %v: transport error: %v", tc.tool, tc.args, err)
		}
		if txt := resultText(res); !res.IsError || !strings.HasPrefix(txt, tc.want) || !strings.Contains(txt, tc.also) {
			t.Errorf("%s %v: error %v, %q\nwant it to start with %q and hold %q", tc.tool, tc.args, res.IsError, txt, tc.want, tc.also)
		}
	}
}

// The server's instructions are what reaches an agent that loaded no skill: the
// always-on rule, and the pointer to the skills.
func TestMCPServerInstructions(t *testing.T) {
	_, res := mcpClient(t)
	for _, want := range []string{"search t7kb before answering from memory", "mid-task too", "t7kb:* skills", "load the one that owns the task"} {
		if !strings.Contains(res.Instructions, want) {
			t.Errorf("instructions lack %q:\n%s", want, res.Instructions)
		}
	}
	if n := len(strings.Fields(res.Instructions)); n > 120 {
		t.Errorf("instructions are %d words: they load in every session, keep them near 100", n)
	}
}

// The server hands memory back once, after every warm-up is done: before, the
// vectors' load still makes garbage; the warm-ups themselves run together.
func TestWarmUpReleasesAfterEveryWarmUp(t *testing.T) {
	var done atomic.Int32
	gate := make(chan struct{})
	slow := func() { <-gate; done.Add(1) }
	fast := func() { done.Add(1) }
	released := make(chan int32, 1)
	warmUp([]func(){slow, fast}, func() { released <- done.Load() })

	deadline := time.After(5 * time.Second)
	for done.Load() < 1 { // fast finishes while slow waits: they run together
		select {
		case <-deadline:
			t.Fatal("fast warm-up waited for slow one")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case n := <-released:
		t.Fatalf("released after %d of 2 warm-ups", n)
	case <-time.After(50 * time.Millisecond):
	}
	close(gate)
	select {
	case n := <-released:
		if n != 2 {
			t.Errorf("released after %d of 2 warm-ups", n)
		}
	case <-deadline:
		t.Fatal("never released")
	}
}

// The server collects at serverGCPercent, unless GOGC is set: then Go's own
// reading of it stands.
func TestUseServerGC(t *testing.T) {
	old := debug.SetGCPercent(100)
	defer debug.SetGCPercent(old)
	t.Setenv("GOGC", "")
	useServerGC()
	if got := debug.SetGCPercent(100); got != serverGCPercent {
		t.Errorf("GOGC unset: collecting at %d%%, want %d%%", got, serverGCPercent)
	}
	t.Setenv("GOGC", "50")
	useServerGC()
	if got := debug.SetGCPercent(100); got != 100 {
		t.Errorf("GOGC set: useServerGC changed it to %d%%", got)
	}
}
