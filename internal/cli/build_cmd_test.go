package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/build"
)

func sampleReport(ok bool) build.Report {
	rep := build.Report{Target: "zm_test", Kind: "usermap", OK: ok, Stages: []build.StageResult{
		{Name: "compile", OK: true, Seconds: 12.3, Note: "wrote zm_test.d3dbsp"},
		{Name: "link", OK: ok, Seconds: 4, Warnings: []string{"Found 2 bad bulletmeshes"}, Missing: []string{"image:i_rust"}},
	}}
	if !ok {
		rep.FailedStage = "link"
		rep.Stages[1].Errors = []string{"Could not find scriptparsetree"}
	}
	return rep
}

// The text summary: one line per stage, then its errors, warnings and what the
// linker linked without.
func TestPrintBuildReport(t *testing.T) {
	var out bytes.Buffer
	if err := finishBuild(&out, false, sampleReport(true)); err != nil {
		t.Fatal(err)
	}
	want := "build zm_test (usermap)\n" +
		"  compile  ok     12.3s  wrote zm_test.d3dbsp\n" +
		"  link     ok      4.0s\n" +
		"      warning: Found 2 bad bulletmeshes\n" +
		"      linked without (looked up, found in no GDT):\n" +
		"        image:i_rust\n" +
		"build OK\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	out.Reset()
	err := finishBuild(&out, false, sampleReport(false))
	if err == nil || err.Error() != "build failed at link" {
		t.Errorf("err = %v", err)
	}
	for _, line := range []string{"  link     FAIL", "      Could not find scriptparsetree\n", "build FAILED at link\n"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("missing %q in:\n%s", line, out.String())
		}
	}
}

func TestFinishBuildJSON(t *testing.T) {
	var out bytes.Buffer
	err := finishBuild(&out, true, sampleReport(false))
	if err == nil {
		t.Error("a failed build must still fail the command")
	}
	var rep build.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if rep.FailedStage != "link" || len(rep.Stages) != 2 || rep.Stages[1].Missing[0] != "image:i_rust" {
		t.Errorf("%+v", rep)
	}
	if !strings.Contains(out.String(), "\n  \"target\"") {
		t.Errorf("want indented JSON:\n%s", out.String())
	}
}

// The MCP dvars object becomes the CLI's name=value pairs, sorted; a number or
// a boolean is a value too.
func TestDvarPairs(t *testing.T) {
	pairs, err := dvarPairs(map[string]any{"logfile": float64(2), "developer": "2", "scr_mod_enable_devblock": true, "ai_disableSpawn": false, "timescale": 0.5})
	if err != nil || strings.Join(pairs, " ") != "ai_disableSpawn=0 developer=2 logfile=2 scr_mod_enable_devblock=1 timescale=0.5" {
		t.Fatalf("MCP dvars: %v %v", pairs, err)
	}
	if pairs, err := dvarPairs(nil); err != nil || pairs != nil {
		t.Errorf("no dvars: %v %v", pairs, err)
	}
	if _, err := dvarPairs("developer=2"); err == nil {
		t.Error("a string instead of an object: want an error")
	}
	if _, err := dvarPairs(map[string]any{"developer": []any{2}}); err == nil {
		t.Error("a list value: want an error")
	}
}

// `t7kb build` hands its flags to build.Run and fails with its preflight error.
func TestBuildCommandPreflightError(t *testing.T) {
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"build", "zm_test", "--tools-path", t.TempDir(), "--json"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mod tools not found") {
		t.Errorf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("no stage ran, yet it printed %q", out.String())
	}
}

// The CLI's flags and the MCP tool's arguments ask build.Run for the same
// build: every flag lands in its option, and the two surfaces agree.
func TestBuildCLIAndMCPAskForTheSameBuild(t *testing.T) {
	var got []build.Options
	old := buildStart
	t.Cleanup(func() { buildStart = old })
	buildStart = func(o *build.Options, name string) (build.State, error) {
		if name != "zm_x" {
			t.Errorf("name %q", name)
		}
		got = append(got, *o)
		return build.State{Target: name, Kind: "usermap"}, nil
	}
	want := build.Options{ToolsPath: "C:/bo3", GamePath: "D:/game", Mod: true, Stages: "link,run", OnlyEnts: true,
		Light: "high", Language: "french", SkipGDT: true, GDTRebuild: true, FreshXpak: true,
		Dvars: []string{"developer=2", "logfile=2"}, LauncherDvars: true}

	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"build", "zm_x", "--tools-path", "C:/bo3", "--game-path", "D:/game", "--mod", "--stages", "link,run",
		"--onlyents", "--light", "high", "--language", "french", "--skip-gdt", "--gdt-rebuild", "--fresh-xpak",
		"--dvar", "developer=2", "--dvar", "logfile=2", "--launcher-dvars", "--verbose", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	callBuildTool(t, map[string]any{"name": "zm_x", "tools_path": "C:/bo3", "game_path": "D:/game", "mod": true, "stages": "link,run",
		"onlyents": true, "light": "high", "language": "french", "skip_gdt": true, "gdt_rebuild": true, "fresh_xpak": true,
		"dvars": map[string]any{"logfile": float64(2), "developer": "2"}, "launcher_dvars": true})
	if len(got) != 2 {
		t.Fatalf("build started %d times", len(got))
	}
	cli, mcp := got[0], got[1]
	if !cli.Verbose || mcp.Verbose {
		t.Errorf("verbose streams tool output to a terminal only: cli %v, mcp %v", cli.Verbose, mcp.Verbose)
	}
	cli.Verbose = false
	if !reflect.DeepEqual(cli, want) || !reflect.DeepEqual(mcp, want) {
		t.Errorf("cli %+v\nmcp %+v\nwant %+v", cli, mcp, want)
	}

	got = nil
	root = newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"build", "zm_x"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	callBuildTool(t, map[string]any{"name": "zm_x"})
	defaults := build.Options{Stages: "compile,light,link", Light: "medium"}
	for i, o := range got {
		if !reflect.DeepEqual(o, defaults) {
			t.Errorf("surface %d defaults: %+v", i, o)
		}
	}
}

func TestMegabytesAreDecimal(t *testing.T) {
	for n, want := range map[int64]string{1_349_999: "1.3 MB", 1_350_001: "1.4 MB", 0: "0.0 MB"} {
		if got := megabytes(n); got != want {
			t.Errorf("megabytes(%d) = %q, want %q", n, got, want)
		}
	}
}
