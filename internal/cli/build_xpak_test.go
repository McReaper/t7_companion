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
	before := map[string]string{"zm_x.xpak": "old+dead", "en_zm_x.xpak": "old-en+dead", "fr_zm_x.xpak": "old-fr", "zm_x.ff": "ff"}
	zone, o := xpakLink(t, "usermaps", before, map[string]string{"zm_x.xpak": "new", "en_zm_x.xpak": "new-en"}, true)
	rep, err := runBuildReport(&o, "zm_x", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zm_x.xpak": "new", "en_zm_x.xpak": "new-en", "fr_zm_x.xpak": "old-fr", "zm_x.ff": "ff"}
	if got := zoneFiles(t, zone); !sameFiles(got, want) {
		t.Errorf("zone/ after the link: %v, want %v", got, want)
	}
	if !rep.OK || !strings.Contains(rep.Stages[0].Note, "0.0 MB -> 0.0 MB") {
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

// A mod's .xpak files live in mods/<mod>/zone.
func TestFreshXpakForAMod(t *testing.T) {
	zone, o := xpakLink(t, "mods", map[string]string{"zm_mod.xpak": "old"}, map[string]string{"zm_mod.xpak": "new"}, true)
	if _, err := runBuildReport(&o, "zm_x", io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := zoneFiles(t, zone); !sameFiles(got, map[string]string{"zm_mod.xpak": "new"}) {
		t.Errorf("zone/: %v", got)
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
