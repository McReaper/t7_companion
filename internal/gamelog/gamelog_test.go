package gamelog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setTime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// A run's log, in the shapes a real one has: an earlier launch log_append
// kept, lines with and without the engine's prefix (developer on and off), the
// stock lookups every load fails, a script error block (captured from a real
// log) and a fatal error's line.
const runLog = `Build 0
logfile opened on Mon Oct 19 22:10:03 2026
ERROR: an earlier launch's problem
logfile opened on Tue Oct 20 01:52:15 2026

----- FS_Startup -----
[    1203][DB]  Error: Could not find fx "weapon/fx_trail_rocket_multi_lob".
Error: Could not find fx "zombie/fx_glow_eye_orange".
Error: Could not find material "mc/hud_outline_model_scriptint".
Error: Could not find material "MC/My_Wall".
Error: Could not find xmodel "p7_my_crate".
Error: Could not find xmodel "p7_my_crate".
Error: Waited 781 msec for missing asset "mod.cfg".
Error: couldn't exec zm_mod.cfg
Error: BG_Cache_RegisterWeapon unknown weapon 'chaser_cp'
ERROR: the map's sun is missing
Error:
Redundant scriptbundle asset 'c_54i_grunt_fx_table' found in zone 'zm_levelcommon' while loading zone 'zm_test'
[  20001][         ]  ------- Game Initialization -------
[  541341][SCRIPTERROR]  Error: throwing script exception: Error: cannot cast undefined to boolError:
[  541344][SCRIPTERROR]
******* script error *******
[  541345][SCRIPTERROR]  Error: cannot cast undefined to bool
[  541346][SCRIPTERROR]  Error:
******* Call stack *******
[  541347][SCRIPTERROR]  Error: 	file 'scripts/zm/zm_test.gsc', line 42 ::
[  541348][SCRIPTERROR]  Error: 	file 'scripts/shared/callbacks_shared.gsc' - missing line information
[  541352][SCRIPTERROR]  Error: ************************************
ModLoad done: zm_test
------- Game Initialization -------
Com_ERROR: ERROR: Could not find zone 'fr_zm_test'
`

const testZone = `>mapname,zm_test
// xmodel,p7_commented_out
scriptparsetree,scripts/zm/zm_test.gsc
xmodel,p7_my_crate
material,mc/my_wall
`

func readRunLog(t *testing.T) (Launch, string) {
	t.Helper()
	game := t.TempDir()
	writeFile(t, filepath.Join(game, "usermaps", "zm_test", "zone_source", "zm_test.zone"), testZone)
	path := filepath.Join(game, LogName)
	writeFile(t, path, strings.ReplaceAll(runLog, "\n", "\r\n"))
	l, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return l, game
}

func TestReadKeepsTheLastLaunchWithTheFilesLineNumbers(t *testing.T) {
	l, _ := readRunLog(t)
	if l.Opened != "Tue Oct 20 01:52:15 2026" || l.Total != 31 {
		t.Errorf("opened %q, %d lines", l.Opened, l.Total)
	}
	if len(l.Lines) != 27 || l.Lines[0] != (Line{5, ""}) || l.Lines[1] != (Line{6, "----- FS_Startup -----"}) {
		t.Errorf("the lines after the last header, numbered in the file, without their \\r: %d, %q", len(l.Lines), l.Lines[:2])
	}
}

func TestSortGivesTheMapsOwnProblemsAndCountsTheStockNoise(t *testing.T) {
	l, game := readRunLog(t)
	s := Sort(l, "zm_test", Zoned(game, "zm_test", false))
	if s.Loaded != 29 || len(s.MapLoads) != 2 || s.MapLoads[0] != 19 || s.MapLoads[1] != 30 {
		t.Errorf("ModLoad done at %d, map loads %v", s.Loaded, s.MapLoads)
	}
	wantScript := Line{22, "script error: cannot cast undefined to bool | file 'scripts/zm/zm_test.gsc', line 42 :: | file 'scripts/shared/callbacks_shared.gsc' - missing line information"}
	if len(s.Script) != 1 || s.Script[0] != wantScript {
		t.Errorf("the script error at its first line, its message and frames on one line:\n%q\nwant %q", s.Script, wantScript)
	}
	want := []Line{{10, `Error: Could not find material "MC/My_Wall".`}, {11, `Error: Could not find xmodel "p7_my_crate".`},
		{16, "ERROR: the map's sun is missing"}, {31, "Com_ERROR: ERROR: Could not find zone 'fr_zm_test'"}}
	if fmt.Sprint(s.Own) != fmt.Sprint(want) {
		t.Errorf("the map's zoned assets, once each, and the Error lines of other kinds; the earlier launch's dropped:\n%v\nwant %v", s.Own, want)
	}
	if got := Kinds(s.Noise); s.NoiseAll != 6 || got != "2 fx, 1 asset, 1 config, 1 material, 1 weapon" {
		t.Errorf("stock noise: %d, %s", s.NoiseAll, got)
	}

	s = Sort(l, "", nil)
	if s.Loaded != 0 || len(s.Own) != 2 || s.NoiseAll != 9 {
		t.Errorf("without a map, no load is awaited and every lookup is noise: %+v", s)
	}
}

func TestSortEndsAScriptErrorWithoutItsClosingLine(t *testing.T) {
	var b strings.Builder
	b.WriteString("******* script error *******\nError: undefined is not an object\n******* Call stack *******\n")
	for i := range 20 {
		fmt.Fprintf(&b, "Error: \tfile 'scripts/zm/f%d.gsc' line %d\n", i, i)
	}
	b.WriteString("ModLoad done: zm_test\n") // the closing line never came
	b.WriteString("******* script error *******\nError: cut short by the end of the log\n")
	path := filepath.Join(t.TempDir(), LogName)
	writeFile(t, path, b.String())
	l, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	s := Sort(l, "zm_test", nil)
	if len(s.Script) != 2 {
		t.Fatalf("two script errors: %q", s.Script)
	}
	if n := strings.Count(s.Script[0].Text, " | "); n != maxSRELines-1 || !strings.HasPrefix(s.Script[0].Text, "script error: undefined is not an object | file 'scripts/zm/f0.gsc'") {
		t.Errorf("a long stack keeps its message and first frames, %d lines: %s", n+1, s.Script[0].Text)
	}
	if s.Loaded != 24 {
		t.Errorf("a line without Error: ends the block and is read as itself: %d", s.Loaded)
	}
	if s.Script[1] != (Line{25, "script error: cut short by the end of the log"}) {
		t.Errorf("the end of the log ends a block: %v", s.Script[1])
	}
}

func TestGrepGivesEachMatchWithItsContextAndCountsThoseOverTheCap(t *testing.T) {
	l, _ := readRunLog(t)
	ms, more := Grep(l, regexp.MustCompile(`(?i)could not find xmodel`), 1, 1)
	if len(ms) != 1 || more != 1 {
		t.Fatalf("one match kept, one over the cap: %+v, %d", ms, more)
	}
	if ms[0].Line != (Line{11, `Error: Could not find xmodel "p7_my_crate".`}) ||
		fmt.Sprint(ms[0].Before) != `[Error: Could not find material "MC/My_Wall".]` || len(ms[0].After) != 1 {
		t.Errorf("the match with a line each side: %+v", ms[0])
	}
	ms, _ = Grep(l, regexp.MustCompile(`^Build 0$|fr_zm_test`), 3, 10)
	if len(ms) != 1 || ms[0].N != 31 || len(ms[0].Before) != 3 || ms[0].After != nil {
		t.Errorf("only the last launch is searched, and the context stops at its ends: %+v", ms)
	}
	if got := Tail(l, 2); len(got) != 2 || got[1].N != 31 {
		t.Errorf("tail: %v", got)
	}
	if got := Tail(l, 100); len(got) != len(l.Lines) {
		t.Errorf("a tail longer than the launch is the launch: %d", len(got))
	}
}

func TestFindIsTheNewestLogWithTheOthersBesideIt(t *testing.T) {
	game := t.TempDir()
	now := time.Now()
	root := filepath.Join(game, LogName)
	mod := filepath.Join(game, "mods", "my_mod", LogName)
	other := filepath.Join(game, "mods", "zm_other", LogName)
	if _, _, ok := Find(game, ""); ok {
		t.Error("no log")
	}
	for p, ago := range map[string]time.Duration{root: time.Hour, mod: time.Minute, other: 2 * time.Hour, filepath.Join(game, "crashes.log"): 3 * time.Hour} {
		writeFile(t, p, "")
		setTime(t, p, now.Add(-ago))
	}
	cur, others, ok := Find(game, "")
	if !ok || cur.Path != mod || len(others) != 3 || others[0].Path != root || others[1].Path != other || others[2].Path != filepath.Join(game, "crashes.log") {
		t.Errorf("the newest of every mod's and the root's, then the others newest first, then the root's other logs: %v %v", cur, others)
	}
	if cur, _, _ := Find(game, "zm_test"); cur.Path != root {
		t.Errorf("a usermap's, which has no mods/ folder, is the root's: %v", cur)
	}
	if cur, _, _ := Find(game, "zm_other"); cur.Path != root {
		t.Errorf("the newer of a mod's and the root's: %v", cur)
	}
	lone := t.TempDir()
	writeFile(t, filepath.Join(lone, "mods", "my_mod", LogName), "")
	if cur, others, ok := Find(lone, "zm_test"); ok || cur.Path != "" || len(others) != 1 {
		t.Errorf("no log of that name, the others still listed: %v %v %v", cur, others, ok)
	}
	if err := os.MkdirAll(filepath.Join(game, "mods", "dir_mod", LogName), 0o755); err != nil {
		t.Fatal(err)
	}
	if cur, _, _ := Find(game, "dir_mod"); cur.Path != root {
		t.Errorf("a folder of that name is no log: %v", cur)
	}
}

// The game writes the log in mods/<fs_game>/ when that folder exists, else
// in its root; the run's is the newer of the two written since it started.
func TestWrittenIsTheNewerSinceTheStart(t *testing.T) {
	game := t.TempDir()
	start := time.Now()
	root := filepath.Join(game, LogName)
	mod := filepath.Join(game, "mods", "zm_test", LogName)
	at := func(p string, d time.Duration) {
		t.Helper()
		writeFile(t, p, "")
		setTime(t, p, start.Add(d))
	}
	if got := Written(game, "zm_test", start); got != "" {
		t.Errorf("no log: %q", got)
	}
	at(root, -time.Hour)
	if got := Written(game, "zm_test", start); got != "" {
		t.Errorf("a log from before the run is a past run's: %q", got)
	}
	at(root, -2*time.Second)
	if got := Written(game, "zm_test", start); got != "" {
		t.Errorf("one written seconds before the start is a past run's: %q", got)
	}
	at(root, -500*time.Millisecond)
	if got := Written(game, "zm_test", start); got != root {
		t.Errorf("a file's time lags the clock by a tick, so one stamped just before the start is the run's: %q", got)
	}
	at(mod, 2*time.Second)
	if got := Written(game, "zm_test", start); got != mod {
		t.Errorf("the newer, in the mod's folder: %q", got)
	}
	at(root, 3*time.Second)
	if got := Written(game, "zm_test", start); got != root {
		t.Errorf("the newer, in the root: %q", got)
	}
}

func TestZonedReadsTheMapsOrTheModsZones(t *testing.T) {
	game := t.TempDir()
	writeFile(t, filepath.Join(game, "usermaps", "zm_test", "zone_source", "zm_test.zone"), strings.ReplaceAll(testZone, "\n", "\r\n"))
	got := Zoned(game, "zm_test", false)
	if !got["p7_my_crate"] || !got["scripts/zm/zm_test.gsc"] || got["p7_commented_out"] || got["zm_test"] || len(got) != 3 {
		t.Errorf("a usermap's zone, its comments and settings left out: %v", got)
	}
	writeFile(t, filepath.Join(game, "mods", "my_mod", "zone_source", "core_mod.zone"), "rawfile,gamedata/x.cfg\n")
	writeFile(t, filepath.Join(game, "mods", "my_mod", "zone_source", "zm_mod.zone"), "XModel,P7_Mod_Crate\n")
	got = Zoned(game, "my_mod", true)
	if !got["gamedata/x.cfg"] || !got["p7_mod_crate"] || len(got) != 2 {
		t.Errorf("each of a mod's zones, in lower case: %v", got)
	}
	if got := Zoned(game, "zm_none", false); len(got) != 0 {
		t.Errorf("no zone: %v", got)
	}
}

func TestKindsLargestFirstThenByName(t *testing.T) {
	got := Kinds(map[string]int{"config": 4, "fx": 98, "asset": 4, "weapon": 44, "material": 71})
	if got != "98 fx, 71 material, 44 weapon, 4 asset, 4 config" {
		t.Errorf("%s", got)
	}
}

// FuzzSort holds the sorting to what its readers rely on for any text: each
// script error is one line of at most maxSRELines parts, each of the map's
// lines is an Error line once, and the noise adds up.
func FuzzSort(f *testing.F) {
	for _, s := range []string{runLog, "", "******* script error *******", "******* script error *******\nError: x\nError: ****\n",
		"Error: Could not find xmodel \"p7_my_crate\".\nError: Could not find xmodel \"p7_my_crate\".", "logfile opened on\nERROR: x\n[ 1][DB]  Error: y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var l Launch
		for i, text := range strings.Split(s, "\n") {
			l.Lines = append(l.Lines, Line{i + 1, text})
		}
		g := Sort(l, "zm_test", map[string]bool{"p7_my_crate": true})
		for _, ln := range g.Script {
			if !strings.HasPrefix(ln.Text, "script error: ") || strings.Count(ln.Text, " | ") >= maxSRELines || strings.Contains(ln.Text, "\n") {
				t.Fatalf("%q: script error %q", s, ln.Text)
			}
		}
		seen := map[string]bool{}
		for _, ln := range g.Own {
			if seen[ln.Text] || !errorLine.MatchString(ln.Text) || ln.N < 1 || ln.N > len(l.Lines) {
				t.Fatalf("%q: own line %v", s, ln)
			}
			seen[ln.Text] = true
		}
		sum := 0
		for _, n := range g.Noise {
			sum += n
		}
		if sum != g.NoiseAll {
			t.Fatalf("%q: noise %v of %d", s, g.Noise, g.NoiseAll)
		}
	})
}
