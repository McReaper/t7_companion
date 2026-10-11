package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writeLog writes zoneSource/<sub> with CRLF lines, as the linker does, and
// sets its modification time age before now.
func writeLog(t *testing.T, zoneSource, sub, body string, age time.Duration) {
	t.Helper()
	p := filepath.Join(zoneSource, filepath.FromSlash(sub))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.ReplaceAll(body, "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-age)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

// Every errorlog the link wrote is read — the language-neutral pass's and a
// language's — each entry with its indented asset chain, an exact repeat once;
// an errorlog older than the link is not its record.
func TestReadLinkLogs(t *testing.T) {
	zs := t.TempDir()
	writeLog(t, zs, "all/assetinfo/zm_x.errorlog", "return 1001000\n"+
		"  an indented line before any entry\n"+
		"^3Found 5 bad bulletmeshes, dumped to x_bulletreport.csv file.\n"+
		"^318 warnings encountered while processing xmodel 'm_lod0'. Log can be found here: m.log\n"+
		"  xmodel:m\n"+
		"    csv:zone_source/zm_x.zone\n"+
		"^318 warnings encountered while processing xmodel 'm_lod0'. Log can be found here: m.log\n"+
		"  xmodel:m\n"+
		"    csv:zone_source/zm_x.zone\n"+
		"^1ERROR: xmodel 'x' is missing\n"+
		"  xmodel:x\n"+
		"^1effect_simple.hlsl#1fee4e98: Shader preprocessor error:\n"+
		"bin\\<memory>(28,10-29): error X1507: failed to open source file: 'effect_simple.hlsl'\n"+
		"  DRAW_METHOD:ship\n"+
		"    techset:wc/t7kb_test#882572b8\n"+
		"^3GetTechnique(unlit) failed\n"+
		"  material:wc/x\n", 0)
	if err := os.MkdirAll(filepath.Join(zs, "all", "assetinfo", "a_folder.errorlog"), 0o755); err != nil { // unreadable, sorted first
		t.Fatal(err)
	}
	writeLog(t, zs, "japanese/assetinfo/ja_zm_x.errorlog", "return 1000\n^3a japanese warning\n", 0)
	writeLog(t, zs, "english/assetinfo/en_zm_x.errorlog", "return 1001000\n^1an old error\n", time.Hour)
	log, ok := readLinkLogs(zs, time.Now())
	if !ok {
		t.Fatal("no errorlog read")
	}
	want := linkLog{
		codes: []string{"1001000", "1000"},
		errors: []string{
			"ERROR: xmodel 'x' is missing (xmodel:x)",
			"effect_simple.hlsl#1fee4e98: Shader preprocessor error: bin\\<memory>(28,10-29): error X1507: failed to open source file: 'effect_simple.hlsl' (DRAW_METHOD:ship < techset:wc/t7kb_test#882572b8)",
		},
		warnings: []string{
			"Found 5 bad bulletmeshes, dumped to x_bulletreport.csv file.",
			"18 warnings encountered while processing xmodel 'm_lod0'. Log can be found here: m.log (xmodel:m < csv:zone_source/zm_x.zone)",
			"GetTechnique(unlit) failed (material:wc/x)",
			"a japanese warning",
		},
		files: []string{"all/assetinfo/zm_x.errorlog", "japanese/assetinfo/ja_zm_x.errorlog"},
	}
	if !reflect.DeepEqual(log, want) {
		t.Errorf("got  %#v\nwant %#v", log, want)
	}
	if _, ok := readLinkLogs(t.TempDir(), time.Now()); ok {
		t.Error("no errorlog, no record")
	}
	only := t.TempDir()
	if err := os.MkdirAll(filepath.Join(only, "all", "assetinfo", "zm_x.errorlog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := readLinkLogs(only, time.Now()); ok {
		t.Error("an errorlog that can't be read is no record")
	}
}

// An errorlog more than 2 s older than the link isn't its record (the margin
// absorbs the file system's clock).
func TestReadLinkLogsAge(t *testing.T) {
	zs := t.TempDir()
	start := time.Now()
	for _, c := range []struct {
		age   time.Duration
		fresh bool
	}{{1500 * time.Millisecond, true}, {2500 * time.Millisecond, false}} {
		writeLog(t, zs, "all/assetinfo/zm_x.errorlog", "return 1000\n^3w\n", 0)
		at := start.Add(-c.age)
		if err := os.Chtimes(filepath.Join(zs, "all", "assetinfo", "zm_x.errorlog"), at, at); err != nil {
			t.Fatal(err)
		}
		if _, ok := readLinkLogs(zs, start); ok != c.fresh {
			t.Errorf("an errorlog %v older than the link: read %v", c.age, ok)
		}
	}
}

// A long list stops at maxLogEntries and says where the rest are.
func TestCappedLogEntries(t *testing.T) {
	var many []string
	for i := range maxLogEntries + 5 {
		many = append(many, fmt.Sprint(i))
	}
	got := capped(many, []string{"all/assetinfo/zm_x.errorlog"})
	if len(got) != 31 || got[30] != "…and 5 more in all/assetinfo/zm_x.errorlog" || got[29] != "29" || len(many) != 35 {
		t.Errorf("capped: %d entries, last %q", len(got), got[len(got)-1])
	}
	if got := capped(many[:maxLogEntries], nil); len(got) != 30 || got[29] != "29" {
		t.Errorf("exactly 30 entries are kept whole: %d", len(got))
	}
	if got := capped(many[:3], nil); len(got) != 3 {
		t.Errorf("a short list is kept whole: %q", got)
	}
}

// The linker's exit code says failed on warnings too: the errorlogs decide.
func TestApplyLinkLog(t *testing.T) {
	zs := t.TempDir()
	start := time.Now()
	writeLog(t, zs, "all/assetinfo/zm_x.errorlog", "return 1000\n^3a warning\n", 0)
	sr := stageResult{Name: "link", Errors: []string{"exit status 1000"}}
	applyLinkLog(&sr, zs, "zm_mod: ", start)
	if !sr.OK || sr.Errors != nil || !reflect.DeepEqual(sr.Warnings, []string{"zm_mod: a warning"}) ||
		sr.Note != "zm_mod: the linker returned 1000 for warnings only: the fastfile is built" {
		t.Errorf("warnings only: %+v", sr)
	}
	ok := stageResult{Name: "link", OK: true, Note: "kept"}
	applyLinkLog(&ok, zs, "", start)
	if !ok.OK || ok.Note != "kept" || len(ok.Warnings) != 1 {
		t.Errorf("a link that exited 0 keeps its note: %+v", ok)
	}
	writeLog(t, zs, "all/assetinfo/zm_x.errorlog", "return 1001000\n^1an error\n^3a warning\n", 0)
	failed := stageResult{Name: "link", Errors: []string{"UNRECOVERABLE ERROR:"}}
	applyLinkLog(&failed, zs, "", start)
	if failed.OK || !reflect.DeepEqual(failed.Errors, []string{"an error"}) || len(failed.Warnings) != 1 {
		t.Errorf("errors replace the console's: %+v", failed)
	}
	none := stageResult{Name: "link", Errors: []string{"exit status 1"}}
	applyLinkLog(&none, t.TempDir(), "", start)
	if none.OK || len(none.Errors) != 1 {
		t.Errorf("no errorlog leaves the result as it is: %+v", none)
	}
}

// Through the build: a map's warnings-only link is OK with its warnings, and
// a mod's zone too, each warning named by its zone; the summary prints them.
func TestLinkReturnsTheErrorlogs(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	for _, mod := range []bool{false, true} {
		root := fakeTools(t)
		f := &fakePipeline{root: root, fail: "link"} // the linker exits non-zero
		f.install(t)
		fake := stageRunner
		stageRunner = func(label, dir, exe string, d time.Duration, v bool, w io.Writer, args ...string) stageResult {
			if label == "link" {
				kind, zoneName := "usermaps", "zm_test"
				if mod {
					kind, zoneName = "mods", args[len(args)-1]
				}
				writeLog(t, filepath.Join(root, kind, "zm_test", "zone_source"), "all/assetinfo/"+zoneName+".errorlog", "return 1000\n^3Found 2 bad bulletmeshes\n", 0)
			}
			return fake(label, dir, exe, d, v, w, args...)
		}
		if mod {
			zf := filepath.Join(root, "mods", "zm_test", "zone_source", "zm_mod.zone")
			if err := os.MkdirAll(filepath.Dir(zf), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(zf, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		rep, err := runBuildReport(&buildOpts{toolsPath: root, stages: "link", light: "medium", language: "english", skipGDT: true, isMod: mod}, "zm_test", io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		want := "Found 2 bad bulletmeshes"
		if mod {
			want = "zm_mod: " + want
		}
		link := rep.Stages[len(rep.Stages)-1]
		if !rep.OK || !reflect.DeepEqual(link.Warnings, []string{want}) {
			t.Errorf("mod=%v: %+v", mod, rep.Stages)
		}
		var out bytes.Buffer
		printBuildReport(&out, rep)
		if !strings.Contains(out.String(), "      warning: "+want+"\n") {
			t.Errorf("summary:\n%s", out.String())
		}
	}
}

// A mod's zone whose errorlog holds errors fails the build, every error named
// by its zone once.
func TestModLinkErrorsFromTheErrorlog(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	root := fakeTools(t)
	f := &fakePipeline{root: root, fail: "link"}
	f.install(t)
	fake := stageRunner
	stageRunner = func(label, dir, exe string, d time.Duration, v bool, w io.Writer, args ...string) stageResult {
		writeLog(t, filepath.Join(root, "mods", "zm_test", "zone_source"), "all/assetinfo/zm_mod.errorlog", "return 1001000\n^1first\n^1second\n", 0)
		return fake(label, dir, exe, d, v, w, args...)
	}
	zf := filepath.Join(root, "mods", "zm_test", "zone_source", "zm_mod.zone")
	if err := os.MkdirAll(filepath.Dir(zf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zf, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := runBuildReport(&buildOpts{toolsPath: root, stages: "link", light: "medium", language: "english", skipGDT: true, isMod: true}, "zm_test", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	link := rep.Stages[len(rep.Stages)-1]
	if rep.OK || !reflect.DeepEqual(link.Errors, []string{"zm_mod: first", "zm_mod: second"}) {
		t.Errorf("%+v", link)
	}
}
