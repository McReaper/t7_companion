package cli

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// splitFloor is a fake command that makes a light and turns the floor brush
// into two, leaving the light selected.
func splitFloor(f *radianttest.Fake, args string, _ []byte) radianttest.Reply {
	if args != "Selection/CSG/CSG Subtract" {
		return radianttest.Err("missing", "no menu entry "+args)
	}
	f.Text = strings.Replace(f.Text, "{5EC8DE49-C58E-11F1-B542-00155DD185EA}", "{0C000001-0000-0000-0000-000000000000}", 1)
	f.Text += "{\nguid \"{0C000009-0000-0000-0000-000000000000}\"\n\"classname\" \"light\"\n\"origin\" \"1 2 3\"\n}\n"
	f.Selection, f.UndoLabel = []string{"{0C000001-0000-0000-0000-000000000000}"}, "subtract"
	return radianttest.OK("", nil)
}

func TestMapCommandAnswersWhatTheCommandDid(t *testing.T) {
	f := testRadiant(t)
	f.Handle("command", splitFloor)
	out, err := mapCommand(context.Background(), "Selection/CSG/CSG Subtract", []string{"5ec8de49"}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.Command != "Selection/CSG/CSG Subtract" || out.Undo != "subtract" || !slices.Equal(out.Deleted, []string{"5ec8de49"}) || len(out.Created) != 2 {
		t.Fatalf("answer = %+v", out)
	}
	floor, light := out.Created[0], out.Created[1]
	if floor.ID != "0c000001" || floor.Material != "aac_wood_mahogany_01" || floor.Class != "" || floor.Bounds[2] != -64 {
		t.Errorf("the floor = %+v", floor)
	}
	if light.ID != "0c000009" || light.Class != "light" || light.Material != "" || light.Bounds[0] != 1 {
		t.Errorf("the light = %+v", light)
	}
	if !slices.Equal(out.Selected, []string{"0c000001"}) {
		t.Errorf("selected %q: not all it made, so said", out.Selected)
	}
}

func TestMapCommandSaysTheSelectionOnceWhenItIsWhatWasMade(t *testing.T) {
	f := testRadiant(t)
	f.Handle("command", func(f *radianttest.Fake, _ string, _ []byte) radianttest.Reply {
		f.Text = strings.Replace(f.Text, "{5EC8DE49-C58E-11F1-B542-00155DD185EA}", "{0D000001-0000-0000-0000-000000000000}", 1)
		f.Selection = []string{"{0d000001-0000-0000-0000-000000000000}"}
		return radianttest.OK("", nil)
	})
	out, err := mapCommand(context.Background(), "Brush/Anything", []string{"5ec8de49"}, nil, 0)
	if err != nil || len(out.Created) != 1 || out.Selected != nil {
		t.Errorf("answer = %+v, %v", out, err)
	}
	if !sameSet([]string{"{A}", "{b}"}, []string{"{B}", "{a}"}) || sameSet([]string{"{A}"}, []string{"{B}"}) || sameSet([]string{"{A}"}, nil) {
		t.Error("sameSet")
	}
}

func TestMapCommandListsAMenu(t *testing.T) {
	f := testRadiant(t)
	f.Menus = map[string]string{"Selection/CSG": "CSG Hollow\t1\t0\nCSG Merge\t0\t0\nMore\t1\t1\n", "": "File\t1\t1\nSelection\t1\t1\n"}
	csg, bar, nope := "Selection/CSG", "", "Nope"
	out, err := mapCommand(context.Background(), "", nil, &csg, 0)
	if err != nil || out.Menu != "Selection/CSG" || !slices.Equal(out.Entries, []string{"CSG Hollow", "CSG Merge (disabled)", "More >"}) || out.Command != "" {
		t.Errorf("menu = %+v, %v", out, err)
	}
	if out, err := mapCommand(context.Background(), "", nil, &bar, 0); err != nil || !slices.Equal(out.Entries, []string{"File >", "Selection >"}) {
		t.Errorf("the menu bar = %+v, %v", out, err)
	}
	if _, err := mapCommand(context.Background(), "", nil, &nope, 0); err == nil {
		t.Error("an unknown menu")
	}
	onMachine(t)
	if _, err := mapCommand(context.Background(), "", nil, &csg, 0); err == nil {
		t.Error("a menu with no Radiant")
	}
}

// The menu bar is an empty menu: --menu= (or --menu "") on the command line,
// where Git Bash turns a lone / into a Windows path, and "" in the tool,
// whose missing menu still runs the command.
func TestMapCommandListsTheMenuBarOnBothSurfaces(t *testing.T) {
	f := testRadiant(t)
	f.Menus = map[string]string{"": "File\t1\t1\n", "Brush": "Rotate\t1\t0\n"}
	run := func(args ...string) (string, error) {
		cmd := newMapCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"command"}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	for _, args := range [][]string{{"--menu="}, {"--menu", ""}, {"--menu", "/"}} {
		out, err := run(args...)
		var answer mapCommandOut
		if err != nil || json.Unmarshal([]byte(out), &answer) != nil || !slices.Equal(answer.Entries, []string{"File >"}) {
			t.Errorf("%q: %v\n%s", args, err, out)
		}
	}
	tool := func(args map[string]any) string {
		res, err := mapCommandTool().Handler(context.Background(), mcpRequest("map_command", args))
		if err != nil {
			t.Fatal(err)
		}
		return resultText(res)
	}
	if got := tool(map[string]any{"menu": ""}); !strings.Contains(got, `"entries":["File >"]`) {
		t.Errorf(`menu "": %s`, got)
	}
	if got := tool(map[string]any{"menu": "Brush"}); !strings.Contains(got, `"entries":["Rotate"]`) {
		t.Errorf("menu Brush: %s", got)
	}
	if got := tool(map[string]any{"menu": "Brush >"}); !strings.Contains(got, `"menu":"Brush","entries":["Rotate"]`) {
		t.Errorf("menu Brush > (pasted from the bar's listing): %s", got)
	}
	if got := tool(map[string]any{}); !strings.Contains(got, "no command") {
		t.Errorf("no menu and no command: %s", got)
	}
	f.Handle("command", splitFloor) // an agent that sends every option, empty or not
	if got := tool(map[string]any{"command": "Selection/CSG/CSG Subtract", "ids": "5ec8de49", "menu": ""}); !strings.Contains(got, `"undo":"subtract"`) {
		t.Errorf("a command with an empty menu: %s", got)
	}
	if out, err := run("--menu=Selection", "Selection/CSG"); err == nil || !strings.Contains(err.Error(), `nothing after it (got "Selection/CSG")`) {
		t.Errorf("a path after --menu: %v\n%s", err, out)
	}
	if out, err := run(); err == nil || !strings.Contains(err.Error(), "--menu= the menu bar") {
		t.Errorf("no path names the menu bar's spelling: %v\n%s", err, out)
	}
}

// An entry pasted back from a menu's listing, its marks on, names it: the
// labels are sent to Radiant without " >" or " (disabled)".
func TestPlainMenuPathTakesOffTheListingsMarks(t *testing.T) {
	for in, want := range map[string]string{
		"Selection >/CSG > /CSG Subtract": "Selection/CSG/CSG Subtract",
		"More > (disabled)":               "More",
		"CSG Merge (disabled)":            "CSG Merge",
		" Selection / CSG ":               "Selection/CSG",
		"Make Geo... >":                   "Make Geo...",
		"Selection > Select By >":         "Selection/Select By",
		"/":                               "",
		"":                                "",
	} {
		if got := plainMenuPath(in); got != want {
			t.Errorf("plainMenuPath(%q) = %q, want %q", in, got, want)
		}
	}
	f := testRadiant(t)
	f.Handle("command", splitFloor)
	if out, err := mapCommand(context.Background(), "Selection >/CSG >/CSG Subtract", []string{"5ec8de49"}, nil, 0); err != nil || out.Command != "Selection/CSG/CSG Subtract" {
		t.Errorf("a pasted command = %+v, %v", out, err)
	}
}

func TestMapCommandToolAndCommand(t *testing.T) {
	f := testRadiant(t)
	f.Handle("command", splitFloor)
	res, err := mapCommandTool().Handler(context.Background(), mcpRequest("map_command",
		map[string]any{"command": "Selection/CSG/CSG Subtract", "ids": "5ec8de49"}))
	if err != nil || res.IsError || !strings.Contains(resultText(res), `"undo":"subtract"`) {
		t.Errorf("tool: %v %s", err, resultText(res))
	}
	run := func(args ...string) (string, error) {
		cmd := newMapCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"command"}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "give a menu path") {
		t.Errorf("no path: %v", err)
	}
	f.Update(func(f *radianttest.Fake) { f.Menus = map[string]string{"Brush": "Rotate\t1\t0\n"} })
	out, err := run("--menu", "Brush")
	var answer mapCommandOut
	if err != nil || json.Unmarshal([]byte(out), &answer) != nil || !slices.Equal(answer.Entries, []string{"Rotate"}) {
		t.Errorf("--menu: %v\n%s", err, out)
	}
	if _, err := run("Nope/Nothing", "f2d51a52"); err == nil || !strings.Contains(err.Error(), "no menu entry") {
		t.Errorf("a missing entry: %v", err)
	}
}
