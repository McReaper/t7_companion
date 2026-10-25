package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/journal"
)

// readJournal is every entry of the journal in dir, in order.
func readJournal(t *testing.T, dir string) []journal.Entry {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, journal.File))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []journal.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 4<<20)
	for sc.Scan() {
		var e journal.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		out = append(out, e)
	}
	return out
}

// The journal records a call, not the MCP server, which records each of its
// tool calls, nor the watch, a listener, nor the interactive browse.
func TestTheJournalSkipsTheServerAndTheListeners(t *testing.T) {
	for args, want := range map[string]bool{"": false, "mcp": false, "watch": false, "map find": true, "--version": true, "search --bm25 light": true} {
		if got := journaled(strings.Fields(args)); got != want {
			t.Errorf("journaled(%q) = %v", args, got)
		}
	}
}

// With T7KB_DEBUG on, a command line is one entry: its arguments, what it read
// and printed, its failure, and a copy of the image its --out names; off, it
// writes nothing.
func TestACommandIsRecordedWithWhatItReadAndAnswered(t *testing.T) {
	testRadiant(t)
	dir := filepath.Join(t.TempDir(), "debug")
	runIt := func(stdin string, args ...string) (string, int) {
		var out, errs strings.Builder
		code := run(args, strings.NewReader(stdin), &out, &errs)
		return out.String() + errs.String(), code
	}
	t.Setenv(journal.Env, "")
	if _, code := runIt("", "map", "find", "--limit", "1"); code != 0 {
		t.Fatal("map find failed")
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("off, the journal wrote")
	}
	t.Setenv(journal.Env, dir)
	png := filepath.Join(t.TempDir(), "top.png")
	for _, c := range []struct {
		stdin string
		args  []string
		code  int
	}{
		{"", []string{"map", "find", "--limit", "1"}, 0},
		{"", []string{"map", "view", "--view", "top", "--out", png}, 0},
		{`[{"entity": {"class": "light", "origin": [1, 2, 3]}}]`, []string{"map", "edit", "--ops", "-"}, 0},
		{"", []string{"map", "find", "--bogus"}, 1},
		{"", []string{"map", "view", "--size", "-1", "--out", png}, 1}, // refused: the image there is the last view's
		{"", []string{"map", "view", "--view", "top", "--out", filepath.Join(t.TempDir(), "top.img")}, 0},
		{"", []string{"watch", "--help"}, 0},
	} {
		if out, code := runIt(c.stdin, c.args...); code != c.code {
			t.Errorf("%q: exit %d, want %d\n%s", c.args, code, c.code, out)
		}
	}
	got := readJournal(t, dir)
	if len(got) != 6 {
		t.Fatalf("%d entries, want 6 (the watch unrecorded): %+v", len(got), got)
	}
	find, view, edit, bad := got[0], got[1], got[2], got[3]
	if refused, img := got[4], got[5]; refused.Images != nil || !refused.Failed || len(img.Images) != 1 {
		t.Errorf("a refused view copies no image, and a view's PNG named .img is copied: %+v, %+v", refused, img)
	}
	if find.Via != "cli" || find.Command != "map find" || strings.Join(find.Args, " ") != "map find --limit 1" || !strings.Contains(find.Output, `"found"`) || find.Failed || find.Time == "" {
		t.Errorf("find = %+v", find)
	}
	if view.Command != "map view" || len(view.Images) != 1 || !strings.Contains(view.Errors, "wrote") {
		t.Errorf("view = %+v", view)
	} else if b, err := os.ReadFile(filepath.Join(dir, view.Images[0])); err != nil || len(b) < 8 || string(b[1:4]) != "PNG" {
		t.Errorf("the view's image copy: %v", err)
	}
	if edit.Command != "map edit" || !strings.Contains(edit.Stdin, `"light"`) || !strings.Contains(edit.Output, `"made"`) {
		t.Errorf("edit = %+v", edit)
	}
	if !bad.Failed || !strings.Contains(bad.Errors, "error: unknown flag: --bogus") {
		t.Errorf("a failure = %+v", bad)
	}
}

// With T7KB_DEBUG on, the MCP server records each tool call: its arguments,
// its text, the images it answered, whether it failed.
func TestTheServerRecordsEachToolCall(t *testing.T) {
	testRadiant(t)
	dir := t.TempDir()
	t.Setenv(journal.Env, dir)
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
	if _, isErr := call(t, c, "map_view", map[string]any{"view": "top"}); isErr {
		t.Fatal("map_view failed")
	}
	if _, isErr := call(t, c, "map_find", map[string]any{"near": "1 2"}); !isErr {
		t.Fatal("a bad near passed")
	}
	got := readJournal(t, dir)
	if len(got) != 2 {
		t.Fatalf("%d entries: %+v", len(got), got)
	}
	view, find := got[0], got[1]
	if view.Via != "mcp" || view.Command != "map_view" || view.Input["view"] != "top" || len(view.Images) != 1 || !strings.Contains(view.Output, `"view":"xy"`) || view.Failed {
		t.Errorf("view = %+v", view)
	} else if _, err := os.Stat(filepath.Join(dir, view.Images[0])); err != nil {
		t.Errorf("the image: %v", err)
	}
	if find.Command != "map_find" || !find.Failed || !strings.Contains(find.Output, "give a point") {
		t.Errorf("a failed call = %+v", find)
	}
}
