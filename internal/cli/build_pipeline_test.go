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
		args = append([]string(nil), args...)
		for i, a := range args {
			if strings.HasPrefix(a, f.root) { // a path, not a flag: on Linux "/update" is absolute too
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
	gameRunner = func(game string, args []string) stageResult {
		f.calls = append(f.calls, "run "+rel(game)+" "+strings.Join(args, " "))
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
		zones  []string          // a mod's zone files to lay out under mods/zm_test/zone_source
	}{
		{
			name: "usermap, every stage",
			opts: buildOpts{stages: "compile,light,link,run", light: "high", language: "english"},
			calls: []string{
				"gdt gdtdb gdtdb.exe /update",
				"compile bin cod2map64.exe -platform pc -navmesh -navvolume -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp",
				"light bin map_source/zm/zm_test.map share/raw/maps/zm/zm_test.led high",
				"link bin linker_modtools.exe -language english -modsource zm_test",
				"run . +set fs_game zm_test +devmap zm_test",
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
			name:  "mod: no compile or light, each zone linked with -fs_game",
			opts:  buildOpts{stages: "compile,light,link", light: "medium", language: "french", isMod: true},
			zones: []string{"zm_mod", "core_mod"},
			calls: []string{"gdt gdtdb gdtdb.exe /update",
				"link bin linker_modtools.exe -language french -fs_game zm_test -modsource core_mod",
				"link bin linker_modtools.exe -language french -fs_game zm_test -modsource zm_mod"},
			ok:    true,
			notes: map[string]string{"compile": "not applicable to a mod", "light": "not applicable to a mod", "link": "linked core_mod, zm_mod"},
		},
		{
			name:   "mod with no zone file: nothing to link",
			opts:   buildOpts{stages: "link", light: "medium", isMod: true, skipGDT: true},
			failed: "link",
			notes:  map[string]string{"link": "no core_mod, mp_mod, cp_mod or zm_mod .zone"},
		},
		{
			name:  "skip gdt, run only",
			opts:  buildOpts{stages: "run", light: "medium", skipGDT: true, dvars: []string{"developer=2", "logfile=2"}},
			calls: []string{"run . +set developer 2 +set logfile 2 +set fs_game zm_test +devmap zm_test"},
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
			for _, z := range tc.zones {
				zf := filepath.Join(root, "mods", "zm_test", "zone_source", z+".zone")
				if err := os.MkdirAll(filepath.Dir(zf), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(zf, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
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
	want := "light bin game/map_source/mp/mp_test.map game/share/raw/maps/mp/mp_test.led medium\nrun game +set fs_game mp_test +devmap mp_test"
	if got := strings.Join(f.calls, "\n"); got != want {
		t.Errorf("the map source and game come from TA_GAME_PATH:\n%s\nwant:\n%s", got, want)
	}
}

func TestGameEnvAttachesToSteam(t *testing.T) {
	env := gameEnv([]string{"PATH=/bin", "SteamAppId=455130", "steamgameid=1", "HOME=/h"})
	got := strings.Join(env, " ")
	if got != "PATH=/bin HOME=/h SteamAppId=311210 SteamGameId=311210" {
		t.Fatalf("the game gets BO3's app id, whatever the parent had (the mod tools are another app): %s", got)
	}
}

func TestGameArgsMatchTheLauncher(t *testing.T) {
	// what the stock modlauncher.exe passes, captured from the running game
	if got := strings.Join(gameArgs("zm_init_sample_map", false), " "); got != "+set fs_game zm_init_sample_map +devmap zm_init_sample_map" {
		t.Errorf("usermap: %s", got)
	}
	if got := strings.Join(gameArgs("my_mod", true), " "); got != "+set fs_game my_mod" {
		t.Errorf("a mod is mounted, not devmap'd: %s", got)
	}
}

func TestDvarsAsTheLauncherPassesThem(t *testing.T) {
	// the Launcher's saved options, as readLauncherDvars returns them (checkboxes stored as true/false)
	saved := []dvar{{"ai_disableSpawn", launcherValue("false")}, {"developer", "2"}, {"g_password", ""}, {"logfile", "2"},
		{"scr_mod_enable_devblock", launcherValue("true")}, {"connect", ""}, {"set_gametype", ""}, {"splitscreen", launcherValue("true")}, {"splitscreen_playerCount", "2"}}
	got := strings.Join(append(dvarArgs(saved), gameArgs("zm_init_sample_map", false)...), " ")
	// the user's own Launcher run, captured from the console: empty ones left out, 0 kept
	want := "+set ai_disableSpawn 0 +set developer 2 +set logfile 2 +set scr_mod_enable_devblock 1 +set splitscreen 1 +set splitscreen_playerCount 2 +set fs_game zm_init_sample_map +devmap zm_init_sample_map"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	extra, err := parseDvars([]string{"Developer=1", "sv_cheats=1", "connect=127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	merged := strings.Join(dvarArgs(mergeDvars(saved[:2], extra)), " ")
	if merged != "+set ai_disableSpawn 0 +set Developer 1 +set sv_cheats 1 +connect 127.0.0.1" {
		t.Fatalf("an explicit dvar overrides a saved one in place, a new one goes last, connect is a command: %s", merged)
	}
	if _, err := parseDvars([]string{"novalue"}); err == nil {
		t.Fatal("name=value is required")
	}
	pairs, err := dvarPairs(map[string]any{"logfile": float64(2), "developer": "2", "scr_mod_enable_devblock": true})
	if err != nil || strings.Join(pairs, " ") != "developer=2 logfile=2 scr_mod_enable_devblock=1" {
		t.Fatalf("MCP dvars: %v %v", pairs, err)
	}
}

// The linker exits non-zero on warnings too: its errorlog tells them apart,
// ^3 for a warning, ^1 for an error (lines as a real link writes them).
func TestWarningsOnly(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
		return p
	}
	start := time.Now()
	warn := write("warn.errorlog", "return 1000\r\n^3Found 5 bad bulletmeshes, dumped to x_bulletreport.csv file.\r\n", 0)
	if note, ok := warningsOnly(warn, start); !ok || !strings.Contains(note, "Found 5 bad bulletmeshes") {
		t.Errorf("warnings only: %q %v", note, ok)
	}
	bad := write("bad.errorlog", "return 1001000\r\n^1ERROR: xmodel 'x' is missing\r\n  xmodel:x\r\n^3Found 5 bad bulletmeshes\r\n", 0)
	if _, ok := warningsOnly(bad, start); ok {
		t.Error("a ^1 line is an error")
	}
	old := write("old.errorlog", "return 1000\r\n^3a warning\r\n", time.Hour)
	if _, ok := warningsOnly(old, start); ok {
		t.Error("an errorlog older than this link is not its verdict")
	}
	if _, ok := warningsOnly(filepath.Join(dir, "none.errorlog"), start); ok {
		t.Error("no errorlog, no verdict")
	}
}
