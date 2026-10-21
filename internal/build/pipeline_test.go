package build

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
	gameLog    string // what the game writes in its root's console_mp.log, none when ""
	calls      []string
	timeouts   map[string]time.Duration // each stage's, by label
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
	f.timeouts = map[string]time.Duration{}
	stageRunner = func(label, dir, exe string, timeout time.Duration, _ bool, _ io.Writer, args ...string) StageResult {
		f.timeouts[label] = timeout
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
		return StageResult{Name: label, OK: label != f.fail}
	}
	lightRunner = func(bin, mapSrc, led, quality string) StageResult {
		f.calls = append(f.calls, strings.Join([]string{"light", rel(bin), rel(mapSrc), rel(led), quality}, " "))
		return StageResult{Name: "light", OK: f.fail != "light"}
	}
	gameRunner = func(game string, args []string) StageResult {
		f.calls = append(f.calls, "run "+rel(game)+" "+strings.Join(args, " "))
		if f.gameLog != "" {
			writeFile(t, filepath.Join(game, "console_mp.log"), f.gameLog)
		}
		return StageResult{Name: "run", OK: true}
	}
}

func TestRunBuildReportPipeline(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	cases := []struct {
		name         string
		opts         Options
		fail         string
		noBSP        bool
		calls        []string
		ok           bool
		failed       string
		notes        map[string]string // stage -> note or first error it must contain
		gameLog      string            // the game's console_mp.log, none when ""
		zones        []string          // a mod's zone files to lay out under mods/zm_test/zone_source
		localization string            // the game's localization.txt, none when ""
	}{
		{
			name: "usermap, every stage",
			opts: Options{Stages: "compile,light,link,run", Light: "high", Language: "english"},
			calls: []string{
				"gdt gdtdb gdtdb.exe /update",
				"compile bin cod2map64.exe -platform pc -navmesh -navvolume -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp",
				"light bin map_source/zm/zm_test.map share/raw/maps/zm/zm_test.led high",
				"link bin linker_modtools.exe -language english -modsource zm_test",
				"run . +set logfile 2 +set fs_game zm_test +devmap zm_test",
			},
			ok:    true,
			notes: map[string]string{"compile": "wrote zm_test.d3dbsp", "run": "the game wrote no console_mp.log"},
		},
		{
			name:  "onlyents, rebuild the gdt db",
			opts:  Options{Stages: "compile", Light: "medium", OnlyEnts: true, GDTRebuild: true},
			calls: []string{"gdt gdtdb gdtdb.exe /rebuild", "compile bin cod2map64.exe -platform pc -onlyents -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp"},
			ok:    true,
		},
		{
			name:  "mod: no compile or light, each zone linked with -fs_game",
			opts:  Options{Stages: "compile,light,link", Light: "medium", Language: "french", Mod: true},
			zones: []string{"zm_mod", "core_mod"},
			calls: []string{"gdt gdtdb gdtdb.exe /update",
				"link bin linker_modtools.exe -language french -fs_game zm_test -modsource core_mod",
				"link bin linker_modtools.exe -language french -fs_game zm_test -modsource zm_mod"},
			ok:    true,
			notes: map[string]string{"compile": "not applicable to a mod", "light": "not applicable to a mod", "link": "linked core_mod, zm_mod"},
		},
		{
			name:         "no language asked: the game's",
			opts:         Options{Stages: "link", Light: "medium", SkipGDT: true},
			localization: "french\r\n",
			calls:        []string{"link bin linker_modtools.exe -language french -modsource zm_test"},
			ok:           true,
			notes:        map[string]string{"link": "language french: the game's, from its localization.txt"},
		},
		{
			name:  "no language asked or saved: english",
			opts:  Options{Stages: "link", Light: "medium", SkipGDT: true},
			calls: []string{"link bin linker_modtools.exe -language english -modsource zm_test"},
			ok:    true,
			notes: map[string]string{"link": "language english: the game's install names none (localization.txt, Steam's manifest)"},
		},
		{
			name:   "mod with no zone file: nothing to link",
			opts:   Options{Stages: "link", Light: "medium", Mod: true, SkipGDT: true},
			failed: "link",
			notes:  map[string]string{"link": "no core_mod, mp_mod, cp_mod or zm_mod .zone"},
		},
		{
			name:    "the run names the log the game wrote",
			opts:    Options{Stages: "run", Light: "medium", SkipGDT: true},
			gameLog: "logfile opened on Tue Oct 20 01:52:15 2026",
			calls:   []string{"run . +set logfile 2 +set fs_game zm_test +devmap zm_test"},
			ok:      true,
			notes:   map[string]string{"run": "console_mp.log (game_log reads it)"},
		},
		{
			name:  "skip gdt, run only",
			opts:  Options{Stages: "run", Light: "medium", SkipGDT: true, Dvars: []string{"developer=2", "logfile=2"}},
			calls: []string{"run . +set developer 2 +set logfile 2 +set fs_game zm_test +devmap zm_test"},
			ok:    true,
		},
		{
			name:   "a failed gdt pass stops the build",
			opts:   Options{Stages: "compile,link", Light: "medium"},
			fail:   "gdt",
			calls:  []string{"gdt gdtdb gdtdb.exe /update"},
			failed: "gdt",
		},
		{
			name:   "cod2map exiting 0 without writing the bsp is a failure",
			opts:   Options{Stages: "compile,link", Light: "medium", SkipGDT: true},
			noBSP:  true,
			calls:  []string{"compile bin cod2map64.exe -platform pc -navmesh -navvolume -loadFrom map_source/zm/zm_test.map share/raw/maps/zm/zm_test.d3dbsp"},
			failed: "compile",
			notes:  map[string]string{"compile": "was not written"},
		},
		{
			name:   "a failed light stops before link",
			opts:   Options{Stages: "light,link", Light: "low", SkipGDT: true},
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
			f := &fakePipeline{root: root, fail: tc.fail, noBSP: tc.noBSP, gameLog: tc.gameLog}
			f.install(t)
			if tc.localization != "" {
				if err := os.WriteFile(filepath.Join(root, "localization.txt"), []byte(tc.localization), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			opts := tc.opts
			opts.ToolsPath = root
			rep, err := Run(&opts, "zm_test", io.Discard)
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
			if opts.Mod {
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
	if _, err := Run(&Options{ToolsPath: root, Stages: "light,run", Light: "medium", SkipGDT: true}, "mp_test", io.Discard); err != nil {
		t.Fatal(err)
	}
	want := "light bin game/map_source/mp/mp_test.map game/share/raw/maps/mp/mp_test.led medium\nrun game +set logfile 2 +set fs_game mp_test +devmap mp_test"
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
	for _, parent := range [][]string{nil, {"SteamAppId=1"}} {
		if got := strings.Join(gameEnv(parent), " "); got != "SteamAppId=311210 SteamGameId=311210" {
			t.Errorf("from %q: %s", parent, got)
		}
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
}

// Each tool gets time for its slow case before it is killed: a first link
// converts every image it packs (~23 min for the ZM Advanced template), and
// the light bake is waited on for longer than a high-quality bake takes,
// polling without spinning.
func TestStageTimeouts(t *testing.T) {
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	f := &fakePipeline{root: fakeTools(t)}
	f.install(t)
	if _, err := Run(&Options{ToolsPath: f.root, Stages: "compile,link", Light: "medium", Language: "english"}, "zm_test", io.Discard); err != nil {
		t.Fatal(err)
	}
	for stage, least := range map[string]time.Duration{"gdt": 5 * time.Minute, "compile": 10 * time.Minute, "link": 30 * time.Minute} {
		if got := f.timeouts[stage]; got < least {
			t.Errorf("%s times out after %v, want at least %v", stage, got, least)
		}
	}
	if lightTimeout < 20*time.Minute || lightPoll <= 0 || lightPoll > 5*time.Second {
		t.Errorf("light waits %v, polling every %v", lightTimeout, lightPoll)
	}
}
