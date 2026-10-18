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

// What cod2map printed on a real compile: the navmesh's note on every map
// with no nav_volume, a problem it found, prefabs it couldn't find.
const (
	navNote   = "WARNING: NavVolume generation is skipped. Flying units might not behave correctly. This might be due to there is no nav_volume brush in the level, or the generation is skipped purposely by skipping -navvolume option.\r\n"
	problems  = "WARNING: PROBLEMS FOUND [Load with Error File in Radiant]\r\nWARNING: Texture (script_floor_ceiling) repeats more than once per inch. (x1)\r\n"
	noPrefab  = "ERROR: Prefab '_prefabs/zm/zm_core/power_switch.map' not found\r\n"
	errlogRow = "\"C:\\m\\zm_x.map\" 0 220 -50 -50 18 -0 -0 -1 \"Texture (script_floor_ceiling) repeats more than once per inch.\"\r\n"
)

// A compile's verdict is what cod2map logged: an ERROR fails it, a WARNING is
// listed, a problem the fresh .errlog places is listed by entity, brush and
// point instead of its summary line; the navmesh's note and the problems'
// header are left out, and a .errlog older than the compile is not read.
func TestACompilesVerdictIsWhatCod2mapLogged(t *testing.T) {
	dir := t.TempDir()
	errlog := filepath.Join(dir, "zm_x.errlog")
	start := time.Now()
	many := ""
	for k := range 25 {
		many += fmt.Sprintf("\"C:\\m\\zm_x.map\" 0 %d 1 2 3 0 0 1 \"Brush %d is bad.\"\n", k, k)
	}
	for _, c := range []struct {
		name, out, errlog string
		stale             bool
		ok                bool
		errors, warnings  []string
	}{
		{"clean", "Compiling...\r\n" + navNote, "", false, true, nil, nil},
		{"a placed problem", navNote + problems, errlogRow, false, true, nil,
			[]string{"entity 0 brush 220 at -50 -50 18: Texture (script_floor_ceiling) repeats more than once per inch."}},
		{"a stale .errlog", problems, errlogRow, true, true, nil,
			[]string{"WARNING: Texture (script_floor_ceiling) repeats more than once per inch. (x1)"}},
		{"a missing prefab", noPrefab + navNote, "", false, false, []string{"ERROR: Prefab '_prefabs/zm/zm_core/power_switch.map' not found"}, nil},
		{"many placed", "", many, false, true, nil, nil},
	} {
		os.Remove(errlog)
		if c.errlog != "" {
			if err := os.WriteFile(errlog, []byte(c.errlog), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.stale {
				old := start.Add(-time.Hour)
				if err := os.Chtimes(errlog, old, old); err != nil {
					t.Fatal(err)
				}
			}
		}
		sr := StageResult{OK: true}
		compileVerdict(&sr, c.out, errlog, start)
		if c.name == "many placed" {
			if len(sr.Warnings) != maxPlaced+1 || !strings.HasPrefix(sr.Warnings[maxPlaced], "and 5 more in ") || sr.Warnings[0] != "entity 0 brush 0 at 1 2 3: Brush 0 is bad." {
				t.Errorf("%s: %q", c.name, sr.Warnings)
			}
			continue
		}
		if sr.OK != c.ok || strings.Join(sr.Errors, "|") != strings.Join(c.errors, "|") || strings.Join(sr.Warnings, "|") != strings.Join(c.warnings, "|") {
			t.Errorf("%s: ok %v, errors %q, warnings %q; want %v, %q, %q", c.name, sr.OK, sr.Errors, sr.Warnings, c.ok, c.errors, c.warnings)
		}
	}
}

// The compile stage judges cod2map by what it printed and the .errlog beside
// the map once the .d3dbsp is written: a prefab it couldn't find fails the
// build, a placed problem goes in the report.
func TestTheCompileStageReportsWhatCod2mapLogged(t *testing.T) {
	root := t.TempDir()
	p := &buildPlan{o: &Options{}, stages: map[string]bool{"compile": true}, bin: filepath.Join(root, "bin"),
		mapSrc: filepath.Join(root, "map_source", "zm", "zm_x.map"), d3dbsp: filepath.Join(root, "share", "raw", "maps", "zm", "zm_x.d3dbsp")}
	for _, d := range []string{filepath.Dir(p.mapSrc), filepath.Dir(p.d3dbsp)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prev := stageRunner
	t.Cleanup(func() { stageRunner = prev })
	for _, c := range []struct {
		out, errlog string
		ok          bool
		want        string
	}{
		{navNote + problems, errlogRow, true, "entity 0 brush 220 at -50 -50 18: Texture (script_floor_ceiling) repeats more than once per inch."},
		{noPrefab, "", false, "ERROR: Prefab '_prefabs/zm/zm_core/power_switch.map' not found"},
	} {
		os.Remove(strings.TrimSuffix(p.mapSrc, ".map") + ".errlog")
		stageRunner = func(label, _, _ string, _ time.Duration, _ bool, _ io.Writer, _ ...string) StageResult {
			if err := os.WriteFile(p.d3dbsp, []byte("bsp"), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.errlog != "" {
				if err := os.WriteFile(strings.TrimSuffix(p.mapSrc, ".map")+".errlog", []byte(c.errlog), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return StageResult{Name: label, OK: true, output: c.out}
		}
		sr, ran := p.compileStage()
		got := strings.Join(append(sr.Errors, sr.Warnings...), "|")
		if !ran || sr.OK != c.ok || got != c.want || sr.Note != "wrote zm_x.d3dbsp" {
			t.Errorf("ok %v, %q, note %q; want %v, %q", sr.OK, got, sr.Note, c.ok, c.want)
		}
	}
}
