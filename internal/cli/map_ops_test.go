package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// radiantRoot is a mod-tools root with Radiant, the bridge plugin and two
// maps, map_source/test.map and map_source/zm/zm_b.map.
func radiantRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"bin/radiant_modtools.exe", "bin/plugins/T7RadiantBridge/T7RadiantBridge.dll",
		"map_source/test.map", "map_source/zm/zm_b.map"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("iwmap 4\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TA_TOOLS_PATH", "")
	return root
}

// onMachine makes the map tools see these fake Radiants for the test.
func onMachine(t *testing.T, fakes ...*radianttest.Fake) *radianttest.Machine {
	t.Helper()
	m := radianttest.NewMachine(fakes...)
	old := radiantMachine
	radiantMachine = m
	t.Cleanup(func() { radiantMachine = old })
	return m
}

func TestMapFileNamesTheMapLikeTheLauncher(t *testing.T) {
	root := radiantRoot(t)
	zm := filepath.Join(root, "map_source", "zm", "zm_b.map")
	for _, tc := range []struct{ arg, want, err string }{
		{arg: "zm_b", want: zm},
		{arg: " zm_b ", want: zm},
		{arg: "test.map", want: filepath.Join(root, "map_source", "test.map")},
		{arg: filepath.Join("zm", "zm_b.map"), want: zm},
		{arg: zm, want: zm},
		{arg: "other.MAP", err: "no map at " + filepath.Join(root, "map_source", "other.MAP")}, // a path, not a name
		{arg: "zm_none", err: "no map at " + filepath.Join(root, "map_source", "zm", "zm_none.map")},
		{arg: "", err: "no map: pass"},
		{arg: "zm", err: "is no map name"},
		{arg: "zm/zm_b", err: "is no map name"},
	} {
		got, err := mapFile(root, tc.arg)
		switch {
		case tc.err != "":
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("mapFile(%q) = %q, %v, want error %q", tc.arg, got, err, tc.err)
			}
		case err != nil || !strings.EqualFold(got, tc.want):
			t.Errorf("mapFile(%q) = %q, %v, want %q", tc.arg, got, err, tc.want)
		}
	}
}

func TestRadiantOpenNeedsAToolsPath(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	if _, err := radiantOpen(context.Background(), "", "zm_b", 0, false); err == nil || !strings.Contains(err.Error(), "no mod-tools path") {
		t.Errorf("err = %v", err)
	}
}

// runRadiant runs `t7kb radiant <args>` against root: stdout and the error.
func runRadiant(root string, args ...string) (string, error) {
	cmd := newRadiantCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, "--tools-path", root))
	err := cmd.Execute()
	return out.String(), err
}

func TestRadiantOpenCommand(t *testing.T) {
	root := radiantRoot(t)
	f := radianttest.New(31, filepath.Join(root, "map_source", "test.map"))
	f.Modified = true
	onMachine(t, f)
	if _, err := runRadiant(root, "open", "zm_b"); err == nil || !strings.Contains(err.Error(), "unsaved changes") {
		t.Errorf("over unsaved changes: err = %v", err)
	}
	out, err := runRadiant(root, "open", "zm_b", "--discard", "--pid", "31")
	var got radiantOpenOut
	if err != nil || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("open --discard: %v\n%s", err, out)
	}
	if want := (radiantOpenOut{PID: 31, Map: filepath.Join(root, "map_source", "zm", "zm_b.map"), Opened: "reloaded", Plugin: "1.1.0"}); got != want {
		t.Errorf("open = %+v, want %+v", got, want)
	}
}

func TestRadiantOpenToolOverMCP(t *testing.T) {
	root := radiantRoot(t)
	m := onMachine(t)
	m.OnStart = func(pid int, _, _ string, args []string) *radianttest.Started {
		return &radianttest.Started{Fake: radianttest.New(pid, args[0])}
	}
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
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	text, isErr := call(t, c, "radiant_open", map[string]any{"map": "test.map", "tools_path": root})
	if isErr || !strings.Contains(text, `"opened":"launched"`) || !strings.Contains(text, `"pid":9001`) {
		t.Errorf("launch: %s", text)
	}
	text, isErr = call(t, c, "radiant_open", map[string]any{"map": "test.map", "tools_path": root})
	if isErr || !strings.Contains(text, `"opened":"already open"`) {
		t.Errorf("again: %s", text)
	}
	if text, isErr = call(t, c, "radiant_open", map[string]any{"map": "zm_nope", "tools_path": root}); !isErr || !strings.Contains(text, "no map at") {
		t.Errorf("a missing map: %s", text)
	}
}
