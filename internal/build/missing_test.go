package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeDepsAt writes <dir>/<zone>.deps with the given missing image lookups,
// each under its own material.
func writeDepsAt(t *testing.T, dir, zoneName string, missing ...string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("version,3421953,593\n")
	for _, m := range missing {
		fmt.Fprintf(&b, "material,mc/mtl_%s\n\tgdt,image,%s,\n", m, m)
	}
	p := filepath.Join(dir, zoneName+".deps")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The link stage lists what the linker looked for and found nowhere, for a
// map and per zone for a mod, and the summary prints it.
func TestLinkListsWhatTheLinkerFoundNowhere(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	for _, mod := range []bool{false, true} {
		root := fakeTools(t)
		f := &fakePipeline{root: root}
		f.install(t)
		fake := stageRunner
		stageRunner = func(label, dir, exe string, d time.Duration, v bool, w io.Writer, args ...string) StageResult {
			if label == "link" { // the linker writes the zone's .deps
				kind, zoneName := "usermaps", "zm_test"
				if mod {
					kind, zoneName = "mods", args[len(args)-1]
				}
				writeDepsAt(t, filepath.Join(root, kind, "zm_test", "zone_source", "all", "assetinfo"), zoneName, "img_gone")
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
		rep, err := Run(&Options{ToolsPath: root, Stages: "link", Light: "medium", Language: "english", SkipGDT: true, Mod: mod}, "zm_test", io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		want := "image img_gone (named by material mc/mtl_img_gone)"
		if mod {
			want = "zm_mod: " + want
		}
		link := rep.Stages[len(rep.Stages)-1]
		if !rep.OK || len(link.Missing) != 1 || link.Missing[0] != want {
			t.Errorf("mod=%v: %+v", mod, rep.Stages)
		}
	}
}

// A .deps more than 2 s older than the link isn't its record (the margin
// absorbs the file system's clock); a long list stops at 10.
func TestMissingAfterStaleOrCapped(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "assetinfo")
	var many []string
	for i := range 13 {
		many = append(many, fmt.Sprintf("img%02d", i))
	}
	p := writeDepsAt(t, dir, "zm_x", many...)
	got := missingAfter(root, dir, "zm_x", time.Now())
	if len(got) != 11 || got[10] != "…and 3 more" || !strings.HasPrefix(got[0], "image img00 ") {
		t.Errorf("capped: %q", got)
	}
	start := time.Now()
	for _, c := range []struct {
		age   time.Duration
		stale bool
	}{{1500 * time.Millisecond, false}, {2500 * time.Millisecond, true}, {time.Hour, true}} {
		at := start.Add(-c.age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		if got := missingAfter(root, dir, "zm_x", start); (got == nil) != c.stale {
			t.Errorf(".deps %v older than the link: %d listed", c.age, len(got))
		}
	}
	if got := missingAfter(root, dir, "zm_none", time.Time{}); got != nil {
		t.Errorf("no .deps: %q", got)
	}
}
