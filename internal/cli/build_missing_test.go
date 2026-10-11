package cli

import (
	"bytes"
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
		stageRunner = func(label, dir, exe string, d time.Duration, v bool, w io.Writer, args ...string) stageResult {
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
		rep, err := runBuildReport(&buildOpts{toolsPath: root, stages: "link", light: "medium", language: "english", skipGDT: true, isMod: mod}, "zm_test", io.Discard)
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
		var out bytes.Buffer
		printBuildReport(&out, rep)
		if !strings.Contains(out.String(), "linked without (looked up, found in no GDT):\n        "+want) {
			t.Errorf("summary:\n%s", out.String())
		}
	}
}

// A .deps older than the link isn't its record; a long list is capped.
func TestMissingAfterStaleOrCapped(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "assetinfo")
	var many []string
	for i := range maxMissing + 3 {
		many = append(many, fmt.Sprintf("img%02d", i))
	}
	p := writeDepsAt(t, dir, "zm_x", many...)
	got := missingAfter(root, dir, "zm_x", time.Now())
	if len(got) != maxMissing+1 || got[maxMissing] != "…and 3 more" || !strings.HasPrefix(got[0], "image img00 ") {
		t.Errorf("capped: %q", got)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if got := missingAfter(root, dir, "zm_x", time.Now()); got != nil {
		t.Errorf("stale .deps: %q", got)
	}
	if got := missingAfter(root, dir, "zm_none", time.Time{}); got != nil {
		t.Errorf("no .deps: %q", got)
	}
}
