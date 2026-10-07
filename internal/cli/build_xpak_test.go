package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// xpakLink lays out a map's zone/ folder and replaces the link with one that
// writes the given files there and then succeeds or fails.
func xpakLink(t *testing.T, kind string, before, writes map[string]string, ok bool) (zone string, opts buildOpts) {
	t.Helper()
	t.Setenv("TA_GAME_PATH", "")
	tools := fakeTools(t)
	t.Setenv("TA_TOOLS_PATH", tools)
	zone = filepath.Join(tools, kind, "zm_x", "zone")
	if kind == "mods" {
		src := filepath.Join(tools, "mods", "zm_x", "zone_source")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "zm_mod.zone"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(zone, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range before {
		if err := os.WriteFile(filepath.Join(zone, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev := stageRunner
	t.Cleanup(func() { stageRunner = prev })
	stageRunner = func(label, _, _ string, _ time.Duration, _ bool, _ io.Writer, _ ...string) stageResult {
		if label == "link" {
			for name, body := range writes {
				if err := os.WriteFile(filepath.Join(zone, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		return stageResult{Name: label, OK: label != "link" || ok}
	}
	return zone, buildOpts{stages: "link", light: "medium", language: "english", freshXpak: true, skipGDT: true, isMod: kind == "mods"}
}

func zoneFiles(t *testing.T, zone string) map[string]string {
	t.Helper()
	es, err := os.ReadDir(zone)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range es {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(zone, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

func sameFiles(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// The link writes the neutral and the English .xpak from scratch: they replace
// the old ones, while French, which this link didn't build, keeps its own.
func TestFreshXpakKeepsWhatTheLinkWrote(t *testing.T) {
	mb := func(n float64, c string) string { return strings.Repeat(c, int(n*1e6)) }
	before := map[string]string{"zm_x.xpak": mb(3, "o"), "en_zm_x.xpak": mb(1, "e"), "fr_zm_x.xpak": "old-fr", "zm_x.ff": "ff"}
	writes := map[string]string{"zm_x.xpak": mb(2, "n"), "en_zm_x.xpak": mb(0.5, "m")}
	zone, o := xpakLink(t, "usermaps", before, writes, true)
	rep, err := runBuildReport(&o, "zm_x", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zm_x.xpak": writes["zm_x.xpak"], "en_zm_x.xpak": writes["en_zm_x.xpak"], "fr_zm_x.xpak": "old-fr", "zm_x.ff": "ff"}
	if got := zoneFiles(t, zone); !sameFiles(got, want) {
		t.Errorf("zone/ after the link: %d files, want %d", len(got), len(want))
	}
	// French was put back, not rewritten: only the two files the link wrote count
	if !rep.OK || rep.Stages[0].Note != ".xpak written from scratch: 4.0 MB -> 2.5 MB" {
		t.Errorf("report: %+v", rep)
	}
}

// A failed link gets every old .xpak back, over any it half wrote: they go
// with the fastfiles the link didn't replace.
func TestFreshXpakRestoresAfterAFailedLink(t *testing.T) {
	before := map[string]string{"zm_x.xpak": "old", "en_zm_x.xpak": "old-en"}
	zone, o := xpakLink(t, "usermaps", before, map[string]string{"zm_x.xpak": "partial"}, false)
	rep, err := runBuildReport(&o, "zm_x", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := zoneFiles(t, zone); !sameFiles(got, before) {
		t.Errorf("zone/ after a failed link: %v, want %v", got, before)
	}
	if rep.OK || rep.Stages[0].Note != "" {
		t.Errorf("report: %+v", rep)
	}
}

// A file a link that never finished left aside is put back when nothing
// replaced it, and dropped when the link did write its replacement.
func TestFreshXpakAfterAnInterruptedLink(t *testing.T) {
	before := map[string]string{"zm_x.xpak" + asideSuffix: "orphan", "en_zm_x.xpak": "en-current", "en_zm_x.xpak" + asideSuffix: "en-stale"}
	zone, o := xpakLink(t, "usermaps", before, map[string]string{"zm_x.xpak": "new"}, true)
	if _, err := runBuildReport(&o, "zm_x", io.Discard); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zm_x.xpak": "new", "en_zm_x.xpak": "en-current"}
	if got := zoneFiles(t, zone); !sameFiles(got, want) {
		t.Errorf("zone/: %v, want %v", got, want)
	}
}

// A mod's .xpak files live in mods/<mod>/zone: they are the ones set aside.
func TestFreshXpakForAMod(t *testing.T) {
	zone, o := xpakLink(t, "mods", map[string]string{"zm_mod.xpak": "old"}, map[string]string{"zm_mod.xpak": "new"}, true)
	rep, err := runBuildReport(&o, "zm_x", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := zoneFiles(t, zone); !sameFiles(got, map[string]string{"zm_mod.xpak": "new"}) {
		t.Errorf("zone/: %v", got)
	}
	if note := rep.Stages[0].Note; !strings.Contains(note, ".xpak written from scratch") {
		t.Errorf("the mod's .xpak wasn't set aside: %q", note)
	}
}

// A file that can't be set aside (the game holds it open) stops the stage
// before the link, with the ones already set aside put back.
func TestFreshXpakWhenAFileCantBeSetAside(t *testing.T) {
	before := map[string]string{"en_zm_x.xpak": "old-en", "zm_x.xpak": "old"}
	zone, o := xpakLink(t, "usermaps", before, map[string]string{"zm_x.xpak": "new", "en_zm_x.xpak": "new-en"}, true)
	blocker := filepath.Join(zone, "zm_x.xpak"+asideSuffix) // a non-empty folder where zm_x.xpak would go
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocker, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := runBuildReport(&o, "zm_x", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || len(rep.Stages[0].Errors) == 0 || !strings.Contains(rep.Stages[0].Errors[0], "can't set zm_x.xpak aside") {
		t.Errorf("report: %+v", rep)
	}
	if got := zoneFiles(t, zone); !sameFiles(got, before) {
		t.Errorf("zone/ (the link must not run, en_zm_x.xpak must be back): %v", got)
	}
}

func TestJoinNotes(t *testing.T) {
	for _, c := range [][3]string{{"", "b", "b"}, {"a", "", "a"}, {"a", "b", "a; b"}, {"", "", ""}} {
		if got := joinNotes(c[0], c[1]); got != c[2] {
			t.Errorf("joinNotes(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

// Without fresh_xpak the link leaves the .xpak to the linker.
func TestNoFreshXpakLeavesTheFile(t *testing.T) {
	zone, o := xpakLink(t, "usermaps", map[string]string{"zm_x.xpak": "old"}, nil, true)
	o.freshXpak = false
	if _, err := runBuildReport(&o, "zm_x", io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := zoneFiles(t, zone); !sameFiles(got, map[string]string{"zm_x.xpak": "old"}) {
		t.Errorf("zone/: %v", got)
	}
}
