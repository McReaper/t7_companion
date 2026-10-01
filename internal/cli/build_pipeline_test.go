package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakePipeline replaces the stage runners: each call is recorded as
// "<stage> <cwd-relative-to-root> <exe> <args…>", and fail names a stage that fails.
// A compile writes the .d3dbsp unless noBSP — cod2map's success is judged by it.
type fakePipeline struct {
	root, fail string
	noBSP      bool
	calls      []string
}

func (f *fakePipeline) install(t *testing.T) {
	t.Helper()
	rel := func(p string) string {
		r, err := filepath.Rel(f.root, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(r)
	}
	prevStage, prevLight, prevGame := stageRunner, lightRunner, gameRunner
	t.Cleanup(func() { stageRunner, lightRunner, gameRunner = prevStage, prevLight, prevGame })
	stageRunner = func(label, dir, exe string, _ time.Duration, _ bool, _ io.Writer, args ...string) stageResult {
		for i, a := range args {
			if filepath.IsAbs(a) {
				args[i] = rel(a)
			}
		}
		f.calls = append(f.calls, strings.TrimSpace(strings.Join([]string{label, rel(dir), filepath.Base(exe), strings.Join(args, " ")}, " ")))
		if label == "compile" && !f.noBSP {
			bsp := filepath.Join(f.root, filepath.FromSlash(args[len(args)-1]))
			if err := os.MkdirAll(filepath.Dir(bsp), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bsp, []byte("bsp"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return stageResult{Name: label, OK: label != f.fail}
	}
	lightRunner = func(bin, mapSrc, led, quality string) stageResult {
		f.calls = append(f.calls, strings.Join([]string{"light", rel(bin), rel(mapSrc), rel(led), quality}, " "))
		return stageResult{Name: "light", OK: f.fail != "light"}
	}
	gameRunner = func(game, name string, isMod bool) stageResult {
		f.calls = append(f.calls, "run "+rel(game)+" "+name+map[bool]string{true: " mod", false: ""}[isMod])
		return stageResult{Name: "run", OK: true}
	}
}

func TestRunBuildReportPipeline(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	cases := []struct {
		name   string
		opts   buildOpts
		fail   string
		noBSP  bool
		calls  []string
		ok     bool
		failed string
		notes  map[string]string // stage -> note or first error it must contain
	}{
		{
			name: "usermap, every stage",
			opts: buildOpts{stages: "compile,light,link,run", light: "high", language: "english"},
			calls: []string{
				"gdt gdtdb gdtdb.exe /update",
				"compile bin cod2map64.exe -platform pc -navmesh -navvolume -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp",
				"light bin map_source/zm/zm_test.map share/raw/maps/zm/zm_test.led high",
				"link bin linker_modtools.exe -language english -modsource zm_test",
				"run . zm_test",
			},
			ok:    true,
			notes: map[string]string{"compile": "wrote zm_test.d3dbsp"},
		},
		{
			name:  "onlyents, rebuild the gdt db",
			opts:  buildOpts{stages: "compile", light: "medium", onlyEnts: true, gdtRebuild: true},
			calls: []string{"gdt gdtdb gdtdb.exe /rebuild", "compile bin cod2map64.exe -platform pc -onlyents -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp"},
			ok:    true,
		},
		{
			name:  "mod: no compile or light, linked with -fs_game",
			opts:  buildOpts{stages: "compile,light,link", light: "medium", language: "french", isMod: true},
			calls: []string{"gdt gdtdb gdtdb.exe /update", "link bin linker_modtools.exe -language french -fs_game zm_test -modsource zm_test"},
			ok:    true,
			notes: map[string]string{"compile": "not applicable to a mod", "light": "not applicable to a mod"},
		},
		{
			name:  "skip gdt, run only",
			opts:  buildOpts{stages: "run", light: "medium", skipGDT: true},
			calls: []string{"run . zm_test"},
			ok:    true,
		},
		{
			name:   "a failed gdt pass stops the build",
			opts:   buildOpts{stages: "compile,link", light: "medium"},
			fail:   "gdt",
			calls:  []string{"gdt gdtdb gdtdb.exe /update"},
			failed: "gdt",
		},
		{
			name:   "cod2map exiting 0 without writing the bsp is a failure",
			opts:   buildOpts{stages: "compile,link", light: "medium", skipGDT: true},
			noBSP:  true,
			calls:  []string{"compile bin cod2map64.exe -platform pc -navmesh -navvolume -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp"},
			failed: "compile",
			notes:  map[string]string{"compile": "was not written"},
		},
		{
			name:   "a failed light stops before link",
			opts:   buildOpts{stages: "light,link", light: "low", skipGDT: true},
			fail:   "light",
			calls:  []string{"light bin map_source/zm/zm_test.map share/raw/maps/zm/zm_test.led low"},
			failed: "light",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeTools(t)
			f := &fakePipeline{root: root, fail: tc.fail, noBSP: tc.noBSP}
			f.install(t)
			opts := tc.opts
			opts.toolsPath = root
			rep, err := runBuildReport(&opts, "zm_test", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(f.calls, "\n") != strings.Join(tc.calls, "\n") {
				t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(f.calls, "\n"), strings.Join(tc.calls, "\n"))
			}
			if rep.OK != tc.ok || rep.FailedStage != tc.failed {
				t.Errorf("ok=%v failed=%q, want ok=%v failed=%q", rep.OK, rep.FailedStage, tc.ok, tc.failed)
			}
			wantKind := "usermap"
			if opts.isMod {
				wantKind = "mod"
			}
			if rep.Target != "zm_test" || rep.Kind != wantKind {
				t.Errorf("target/kind = %q/%q", rep.Target, rep.Kind)
			}
			for stage, want := range tc.notes {
				found := false
				for _, s := range rep.Stages {
					if s.Name == stage && (strings.Contains(s.Note, want) || strings.Contains(strings.Join(s.Errors, " "), want)) {
						found = true
					}
				}
				if !found {
					t.Errorf("stage %s should say %q: %+v", stage, want, rep.Stages)
				}
			}
		})
	}
}

func TestRunBuildReportUsesTheGamePath(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	root := fakeTools(t)
	game := filepath.Join(root, "game")
	t.Setenv("TA_GAME_PATH", game)
	f := &fakePipeline{root: root}
	f.install(t)
	if _, err := runBuildReport(&buildOpts{toolsPath: root, stages: "light,run", light: "medium", skipGDT: true}, "mp_test", io.Discard); err != nil {
		t.Fatal(err)
	}
	want := "light bin game/map_source/mp/mp_test.map game/share/raw/maps/mp/mp_test.led medium\nrun game mp_test"
	if got := strings.Join(f.calls, "\n"); got != want {
		t.Errorf("the map source and game come from TA_GAME_PATH:\n%s\nwant:\n%s", got, want)
	}
}
