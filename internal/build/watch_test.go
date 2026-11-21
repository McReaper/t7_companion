package build

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recordWatch is a RecordWatcher over an empty mod-tools root that has
// ticked once, so what the test writes next is new; it says to the returned
// list, and runs says which tools run.
func recordWatch(t *testing.T, runs map[string]bool) (*RecordWatcher, *[]string) {
	t.Helper()
	var said []string
	w := &RecordWatcher{Root: t.TempDir(), Runs: func(exe string) bool { return runs[exe] }, Out: func(s string) { said = append(said, s) }}
	w.Tick()
	return w, &said
}

// writeRecord writes a record under the root, dated a second after the watch
// began.
func writeRecord(t *testing.T, w *RecordWatcher, rel, body string) {
	t.Helper()
	writeRecordAt(t, w, rel, body, time.Second)
}

// writeRecordAt writes a record under the root, dated that long after the
// watch began.
func writeRecordAt(t *testing.T, w *RecordWatcher, rel, body string, after time.Duration) {
	t.Helper()
	writeFileAt(t, w.Root, rel, body, w.start.Add(after))
}

// writeFileAt writes a file under root, dated at.
func writeFileAt(t *testing.T, root, rel, body string, at time.Time) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func TestRecordWatcherSaysAFailedLinkOnceTheLinkerHasQuit(t *testing.T) {
	runs := map[string]bool{linkerExe: true}
	w, said := recordWatch(t, runs)
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog",
		"return 1\n^1ERROR: no xmodel p7_crate\n  csv:zone_source/zm_x.zone\n^3a warning\n")
	w.Tick()
	if len(*said) != 0 {
		t.Fatalf("nothing while the linker runs: %q", *said)
	}
	runs[linkerExe] = false
	w.Tick()
	w.Tick()
	want := []string{"link of usermaps/zm_x failed with 1 error(s): ERROR: no xmodel p7_crate (csv:zone_source/zm_x.zone); " +
		"all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog"}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("one line once the linker quit:\n got %q\nwant %q", *said, want)
	}
}

func TestRecordWatcherSaysALinkThatBuiltWithWhatItLinkedWithout(t *testing.T) {
	w, said := recordWatch(t, nil)
	deps := "version,1,1\nxmodel,p7_crate\n\tgdt,image,img_a,\n\tgdt,image,img_b,\n\tgdt,image,img_c,\n\tgdt,image,img_d,\n\tgdt,image,img_found,11-22\n"
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", deps, 3*time.Second)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/french/assetinfo/zm_x.deps", "version,1,1\n", 2*time.Second) // a record older than the one before it
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/french/assetinfo/fr_zm_x.errorlog", "return 1000\n^3a french warning\n", 2*time.Second)
	writeRecord(t, w, "mods/my_mod/zone_source/all/assetinfo/zm_mod.deps", "version,1,1\n")
	writeRecordAt(t, w, "mods/my_mod/zone_source/all/assetinfo/core_mod.deps", "version,1,1\nxmodel,x\n\tgdt,image,old_missing,\n", -time.Hour) // an earlier link's
	w.Tick()
	want := []string{ // my_mod's link went fine, and names no earlier link's lookups
		"link of usermaps/zm_x built, with 1 warning(s): a french warning; all in usermaps/zm_x/zone_source/french/assetinfo/fr_zm_x.errorlog, " +
			"but 4 asset(s) it named are in no GDT and it linked on without them: image img_a | image img_b | image img_c | and 1 more",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("each target's link, by name:\n got %q\nwant %q", *said, want)
	}
}

// A link that stops on a fatal error, a crash or a stop leaves its .deps as
// .deps.partial and writes no errorlog: said once the linker has quit, with
// where its error is. An earlier link's .deps doesn't hide it.
func TestRecordWatcherSaysALinkThatStoppedBeforeItFinished(t *testing.T) {
	runs := map[string]bool{linkerExe: true}
	w, said := recordWatch(t, runs)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n", -time.Hour) // the last link that finished
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps.partial", "version,1,1\nscriptparsetree,scripts/zm/_zm_bl")
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1\n^1ERROR: no xmodel p7_crate\n")
	w.Tick()
	if len(*said) != 0 {
		t.Fatalf("nothing while the linker runs: %q", *said)
	}
	runs[linkerExe] = false
	w.Tick()
	w.Tick()
	want := []string{"link of usermaps/zm_x stopped before it finished (a fatal error, a crash or a stop): " +
		"usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps stayed .partial, and its error went only to the Launcher's console: " +
		"ask for it, or link again with build, which reports it; before that it logged 1 error(s): ERROR: no xmodel p7_crate"}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("one line once the linker quit:\n got %q\nwant %q", *said, want)
	}
}

// A .deps.partial its .deps followed, or one left by an earlier link, is not
// a link that stopped.
func TestRecordWatcherSaysALinkThatFinishedAfterItsPartialAsItWent(t *testing.T) {
	w, said := recordWatch(t, nil)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps.partial", "version,1,1\n", time.Second)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n", 3*time.Second)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1000\n^3w\n", 4*time.Second)
	writeRecordAt(t, w, "mods/my_mod/zone_source/all/assetinfo/zm_mod.deps.partial", "version,1,1\n", -time.Hour) // a stopped link before the watch
	writeRecord(t, w, "mods/my_mod/zone_source/all/assetinfo/zm_mod.deps", "version,1,1\n")
	writeRecord(t, w, "mods/my_mod/zone_source/all/assetinfo/zm_mod.errorlog", "return 1000\n^3w\n")
	w.Tick()
	want := []string{
		"link of mods/my_mod built, with 1 warning(s): w; all in mods/my_mod/zone_source/all/assetinfo/zm_mod.errorlog",
		"link of usermaps/zm_x built, with 1 warning(s): w; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("finished links:\n got %q\nwant %q", *said, want)
	}
}

func TestRecordWatcherWaitsForRecordsToSettleAndSaysEachOnce(t *testing.T) {
	w, said := recordWatch(t, nil)
	w.Settle = time.Hour
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n")
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1\n^1e\n")
	w.Tick()
	if len(*said) != 0 {
		t.Fatalf("nothing before the records settle: %q", *said)
	}
	w.Settle = 0
	w.Tick()
	w.Tick()
	if want := []string{"link of usermaps/zm_x failed with 1 error(s): e; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog"}; !reflect.DeepEqual(*said, want) {
		t.Errorf("said once settled: %q", *said)
	}
}

func TestRecordWatcherLeavesRecordsOlderThanTheWatch(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "usermaps", "zm_x", "zone_source", "all", "assetinfo", "zm_x.errorlog")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("^1old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	var said []string
	w := &RecordWatcher{Root: root, Out: func(s string) { said = append(said, s) }}
	w.Tick()
	w.Tick()
	if len(said) != 0 {
		t.Errorf("an earlier link is not news: %q", said)
	}
}

func TestRecordWatcherSaysACompilesLeakAndPlacedProblems(t *testing.T) {
	runs := map[string]bool{compilerExe: true}
	w, said := recordWatch(t, runs)
	writeRecord(t, w, "map_source/zm/zm_x.lin", "0 0 0\n1 1 1\n")
	var errlog string
	for i := range 4 {
		errlog += `"map_source/zm/zm_x.map" 0 ` + string(rune('1'+i)) + ` 1 2 3 0 0 1 "Brush has a bad plane"` + "\n"
	}
	writeRecord(t, w, "map_source/zm/zm_x.errlog", errlog)
	writeRecord(t, w, "map_source/mp/mp_quiet.errlog", "nothing it places\n")
	w.Tick()
	if len(*said) != 0 {
		t.Fatalf("nothing while cod2map runs: %q", *said)
	}
	writeRecordAt(t, w, "share/raw/maps/zm/zm_x.d3dbsp", "bsp", 3*time.Second) // each compile finished: its .d3dbsp comes last
	writeRecordAt(t, w, "share/raw/maps/mp/mp_quiet.d3dbsp", "bsp", 3*time.Second)
	runs[compilerExe] = false
	w.Tick()
	want := []string{"compile of map_source/zm/zm_x.map: the map leaks, and the leakfile map_source/zm/zm_x.lin traces the hole to the void (t7kb:mapping); " +
		"map_source/zm/zm_x.errlog places 4 problem(s): entity 0 brush 1 at 1 2 3: Brush has a bad plane | entity 0 brush 2 at 1 2 3: Brush has a bad plane | " +
		"entity 0 brush 3 at 1 2 3: Brush has a bad plane | and 1 more"}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("a compile's problems, and nothing for one with none:\n got %q\nwant %q", *said, want)
	}
}

// A compile whose process the watch never saw (it began before the watch, or
// a process list missed it) is read from its records: written before its
// .d3dbsp, they date its start. A compile t7kb ran is its build's to say.
func TestRecordWatcherSaysACompileThatStoppedFromItsRecords(t *testing.T) {
	w, said := recordWatch(t, nil)
	w.Builds = func() []State { return []State{{Target: "zm_t7kb", Kind: "usermap", PID: 7, Started: w.start}} }
	for _, name := range []string{"zm_cut", "zm_t7kb", "zm_done"} {
		writeRecordAt(t, w, "share/raw/maps/zm/"+name+".d3dbsp", "bsp", -time.Hour)
		writeRecordAt(t, w, "map_source/zm/"+name+".errlog", `"m" 0 1 1 2 3 0 0 1 "bad plane"`+"\n", 2*time.Second)
	}
	writeRecordAt(t, w, "share/raw/maps/zm/zm_done.d3dbsp", "bsp", 3*time.Second)
	writeRecordAt(t, w, "map_source/zm/zm_quiet.errlog", "nothing it places\n", 2*time.Second) // stopped with nothing placed yet
	w.Tick()
	const problem = "map_source/zm/%s.errlog places 1 problem(s): entity 0 brush 1 at 1 2 3: bad plane"
	const stopped = "compile of map_source/zm/%s.map stopped before it finished (a fatal error, a crash or a stop): it didn't write share/raw/maps/zm/%[1]s.d3dbsp, " +
		"and its error went only to the Launcher's console: ask for it, or compile again with build, which reports it"
	want := []string{
		fmt.Sprintf(stopped, "zm_cut") + "; " + fmt.Sprintf(problem, "zm_cut"),
		"compile of map_source/zm/zm_done.map: " + fmt.Sprintf(problem, "zm_done"),
		fmt.Sprintf(stopped, "zm_quiet"),
		"compile of map_source/zm/zm_t7kb.map: " + fmt.Sprintf(problem, "zm_t7kb"),
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("the compile that stopped, from its records:\n got %q\nwant %q", *said, want)
	}
}

// A link's first record can be up to 2 s younger than the others it wrote, as
// file times are coarse: a .deps or a .lin that much older than the build's
// first new record is the build's, and one older still is an earlier one's.
func TestRecordWatcherTakesRecordsUpTo2sOlderThanTheBuildsFirst(t *testing.T) {
	w, said := recordWatch(t, nil)
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\nxmodel,m\n\tgdt,image,img_a,\n")
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_near.deps", "version,1,1\nxmodel,m\n\tgdt,image,img_b,\n", -time.Second/2)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_far.deps", "version,1,1\nxmodel,m\n\tgdt,image,img_c,\n", -2*time.Second)
	writeRecordAt(t, w, "map_source/zm/zm_near.lin", "0 0 0\n", -time.Second/2)
	writeRecord(t, w, "map_source/zm/zm_near.errlog", `"m" 0 1 1 2 3 0 0 1 "bad plane"`+"\n")
	writeRecordAt(t, w, "map_source/zm/zm_far.lin", "0 0 0\n", -2*time.Second)
	writeRecord(t, w, "map_source/zm/zm_far.errlog", `"m" 0 1 1 2 3 0 0 1 "bad plane"`+"\n")
	writeRecordAt(t, w, "share/raw/maps/zm/zm_near.d3dbsp", "bsp", 2*time.Second) // both compiles finished
	writeRecordAt(t, w, "share/raw/maps/zm/zm_far.d3dbsp", "bsp", 2*time.Second)
	w.Tick()
	want := []string{
		"link of usermaps/zm_x ok, but 2 asset(s) it named are in no GDT and it linked on without them: image img_b | image img_a",
		"compile of map_source/zm/zm_far.map: map_source/zm/zm_far.errlog places 1 problem(s): entity 0 brush 1 at 1 2 3: bad plane",
		"compile of map_source/zm/zm_near.map: the map leaks, and the leakfile map_source/zm/zm_near.lin traces the hole to the void (t7kb:mapping); " +
			"map_source/zm/zm_near.errlog places 1 problem(s): entity 0 brush 1 at 1 2 3: bad plane",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("\n got %q\nwant %q", *said, want)
	}
}

func TestNamedCountsWhatItLeavesOut(t *testing.T) {
	for _, c := range []struct {
		list  []string
		total int
		want  string
	}{
		{[]string{"a", "b", "c"}, 3, "a | b | c"},
		{[]string{"a", "b", "c", "d"}, 4, "a | b | c | and 1 more"},
		{[]string{"a", "b", "c"}, 5, "a | b | c | and 2 more"},
	} {
		if got := named(c.list, c.total); got != c.want {
			t.Errorf("named(%q, %d) = %q, want %q", c.list, c.total, got, c.want)
		}
	}
}

func TestRecordWatcherSaysEachBuildT7kbRanOnceItEnds(t *testing.T) {
	w, said := recordWatch(t, nil)
	at := w.start.Add(-time.Hour)
	running := State{Target: "zm_lost", Kind: "usermap", PID: 7, Started: at, Running: "light"}
	starting := State{Target: "zm_quiet", Kind: "usermap", Started: at}
	states := []State{
		running, starting,
		{Target: "zm_old", Kind: "usermap", Done: true, Started: at, Ended: at.Add(time.Minute)}, // ended before the watch
		{Target: "zm_never_seen", Kind: "usermap", Lost: true, Started: at},                      // lost before the watch
	}
	w.Builds = func() []State { return states }
	w.Tick()
	end := w.start.Add(time.Minute)
	running.Lost, running.Running = true, "light"
	starting.Lost = true
	failed := Report{FailedStage: "link", Stages: []StageResult{{Name: "gdt", OK: true}, {Name: "link", Errors: []string{"e1", "e2", "e3", "e4"}}}}
	states = []State{
		running, starting,
		{Target: "zm_ok", Kind: "usermap", Done: true, Started: end.Add(-2 * time.Minute), Ended: end, Report: Report{OK: true, Stages: []StageResult{{Name: "gdt"}, {Name: "link"}}}},
		{Target: "zm_warned", Kind: "usermap", Done: true, Started: end, Ended: end, Report: Report{OK: true, Stages: []StageResult{
			{Name: "compile", Warnings: []string{"w1"}}, {Name: "link", Warnings: []string{"w2", "w3"}, Missing: []string{"image img_a (named by material m)"}}}}},
		{Target: "zm_bad", Kind: "mod", Done: true, Started: end.Add(-time.Minute), Ended: end, Report: failed},
		{Target: "zm_mute", Kind: "usermap", Done: true, Started: end, Ended: end, Report: Report{FailedStage: "start"}},
		{Target: "zm_dumb", Kind: "usermap", Done: true, Started: end, Ended: end, Report: Report{FailedStage: "light", Stages: []StageResult{{Name: "light"}}}},
	}
	w.Tick()
	w.Tick()
	want := []string{
		"build of zm_lost stopped at light: its process ended before the build did; build's status has what it did",
		"build of zm_quiet stopped at its start: its process ended before the build did; build's status has what it did",
		"build of zm_ok finished ok in 2m0s: gdt, link",
		"build of zm_warned finished ok in 0s: compile, link; its compile warned of w1; its link warned of w2 | w3; " +
			"its link went on without what no GDT defines: image img_a (named by material m)",
		"build of zm_bad failed at link after 1m0s: e1 | e2 | e3 | and 1 more; build's status has the report",
		"build of zm_mute failed at start after 0s: no error said; build's status has the report",
		"build of zm_dumb failed at light after 0s: no error said; build's status has the report",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("each build once, as it ended:\n got %q\nwant %q", *said, want)
	}
}

func TestRecordWatcherLeavesALinkT7kbRanToItsBuildsLine(t *testing.T) {
	w, said := recordWatch(t, nil)
	build := State{Target: "zm_x", Kind: "usermap", PID: 7, Started: w.start}
	w.Builds = func() []State { return []State{build} }
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n")
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1000\n^3w1\n")
	writeRecord(t, w, "mods/zm_x/zone_source/all/assetinfo/zm_mod.deps", "version,1,1\n") // another target of that name
	writeRecord(t, w, "mods/zm_x/zone_source/all/assetinfo/zm_mod.errorlog", "return 1000\n^3w1\n")
	w.Tick()
	if want := []string{"link of mods/zm_x built, with 1 warning(s): w1; all in mods/zm_x/zone_source/all/assetinfo/zm_mod.errorlog"}; !reflect.DeepEqual(*said, want) {
		t.Errorf("the running build's link is its own line's: %q", *said)
	}
	// Records up to 2 s around the build are its own, as file times are coarse.
	build.Started = w.start.Add(2 * time.Second)
	writeRecord(t, w, "usermaps/zm_x/zone_source/french/assetinfo/zm_x.deps", "version,1,1\n") // 1 s before its start
	w.Tick()
	build.Done, build.Ended, build.Report = true, w.start.Add(3*time.Second), Report{OK: true}
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/german/assetinfo/zm_x.deps", "version,1,1\n", 4*time.Second) // 1 s after its end
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/german/assetinfo/ge_zm_x.errorlog", "return 1000\n^3w1\n", 4*time.Second)
	w.Tick()
	if len(*said) != 2 || !strings.HasPrefix((*said)[1], "build of zm_x finished ok") {
		t.Errorf("records within 2 s of the build are its own: %q", *said)
	}
	// A later link, from the Launcher: the build's link was the last one.
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n", time.Minute)
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1000\n^3w1\n^3w2\n", time.Minute)
	w.Tick()
	if len(*said) != 3 || (*said)[2] != "link of usermaps/zm_x built, with 2 warning(s), 1 of them new: w2; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog" {
		t.Errorf("a link after the build ended is the Launcher's: %q", *said)
	}
}

// A link t7kb ran that stopped before it finished is its build's line alone:
// the build read the error from the linker's own output.
func TestRecordWatcherLeavesAStoppedLinkT7kbRanToItsBuildsLine(t *testing.T) {
	w, said := recordWatch(t, nil)
	build := State{Target: "zm_x", Kind: "usermap", PID: 7, Started: w.start}
	w.Builds = func() []State { return []State{build} }
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps.partial", "version,1,1\n")
	w.Tick()
	build.Done, build.Ended = true, w.start.Add(2*time.Second)
	build.Report = Report{FailedStage: "link", Stages: []StageResult{{Name: "link", Errors: []string{"script error"}}}}
	w.Tick()
	writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n", time.Minute) // the fix, linked from the Launcher
	w.Tick()
	want := []string{"build of zm_x failed at link after 2s: script error; build's status has the report", "link of usermaps/zm_x ok again"}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("only the build's line, then the Launcher's link after it:\n got %q\nwant %q", *said, want)
	}
}

// A link that built says only what the last link didn't: each line wakes the
// agent. The last link is the last one, not all of them.
func TestRecordWatcherSaysOnlyWhatALinkChanged(t *testing.T) {
	w, said := recordWatch(t, nil)
	bullets := "Found 3 bad bulletmeshes, dumped to " + w.Root + `\/usermaps\zm_x/zone_source/all/assetinfo/zm_x_bulletreport.csv file.`
	link := func(at time.Duration, warnings []string, missing ...string) {
		t.Helper()
		deps := "version,1,1\nxmodel,m\n"
		for _, m := range missing {
			deps += "\tgdt,image," + m + ",\n"
		}
		writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", deps, at)
		if len(warnings) > 0 {
			writeRecordAt(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog", "return 1000\n^3"+strings.Join(warnings, "\n^3")+"\n", at+time.Second)
		}
		w.Tick()
	}
	link(10*time.Second, []string{bullets}, "img_a")
	link(20*time.Second, []string{bullets}, "img_a")
	link(30*time.Second, []string{bullets, "a new warning"}, "img_a")
	link(40*time.Second, []string{bullets}, "img_a", "img_b")
	link(50*time.Second, nil)
	link(60*time.Second, nil, "img_a")
	const log = "; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog"
	want := []string{
		"link of usermaps/zm_x built, with 1 warning(s): Found 3 bad bulletmeshes, dumped to usermaps/zm_x/zone_source/all/assetinfo/zm_x_bulletreport.csv file." + log +
			", but 1 asset(s) it named are in no GDT and it linked on without them: image img_a",
		"link of usermaps/zm_x built, with 2 warning(s), 1 of them new: a new warning" + log + ", but it linked on without the same 1 asset(s) in no GDT as before",
		"link of usermaps/zm_x built, with the same 1 warning(s) as before, but 2 asset(s) it named are in no GDT and it linked on without them, 1 of them new: image img_b",
		"link of usermaps/zm_x ok, but 1 asset(s) it named are in no GDT and it linked on without them: image img_a",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("what changed, link after link:\n got %q\nwant %q", *said, want)
	}
}

// A link that failed or stopped is said each time, and the next one that
// builds says it built again. One that broke leaves the last built link's
// warnings as they were.
func TestRecordWatcherSaysALinkThatBuiltAgainAfterOneThatBroke(t *testing.T) {
	w, said := recordWatch(t, nil)
	const all = "usermaps/zm_x/zone_source/all/assetinfo/"
	link := func(at time.Duration, errorlog string) {
		t.Helper()
		writeRecordAt(t, w, all+"zm_x.deps", "version,1,1\n", at)
		if errorlog != "" {
			writeRecordAt(t, w, all+"zm_x.errorlog", errorlog, at+time.Second)
		}
		w.Tick()
	}
	link(10*time.Second, "return 1\n^1ERROR: bad script\n")
	link(20*time.Second, "return 1\n^1ERROR: bad script\n")
	link(30*time.Second, "")
	writeRecordAt(t, w, all+"zm_x.deps.partial", "version,1,1\n", 40*time.Second)
	w.Tick()
	link(50*time.Second, "return 1000\n^3w\n")
	link(60*time.Second, "return 1000\n^3w\n")
	link(70*time.Second, "return 1\n^1ERROR: bad script\n^3w\n^3w2\n")
	link(80*time.Second, "return 1000\n^3w\n")
	failed := "link of usermaps/zm_x failed with 1 error(s): ERROR: bad script; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog"
	want := []string{
		failed, failed,
		"link of usermaps/zm_x ok again",
		"link of usermaps/zm_x stopped before it finished (a fatal error, a crash or a stop): usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps stayed .partial, " +
			"and its error went only to the Launcher's console: ask for it, or link again with build, which reports it",
		"link of usermaps/zm_x built again, with 1 warning(s): w; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog",
		failed,
		"link of usermaps/zm_x built again, with the same 1 warning(s) as before",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("each break, and each build after one:\n got %q\nwant %q", *said, want)
	}
}

// The last link before the watch is read from disk: the errorlogs and the
// .deps.partial written since the last link that got through its
// language-neutral pass, which every link runs first. A language pass only an
// earlier link ran keeps its records. Its lookups in no GDT aren't read, so
// the first link names them all.
func TestRecordWatcherTakesTheLastLinkBeforeItFromDisk(t *testing.T) {
	root := t.TempDir()
	before := time.Now().Add(-time.Hour)
	bullets := "Found 3 bad bulletmeshes, dumped to " + root + `\/usermaps\zm_same/zone_source/all/assetinfo/zm_same_bulletreport.csv file.`
	writeFileAt(t, root, "usermaps/zm_same/zone_source/all/assetinfo/zm_same.deps", "version,1,1\n", before)
	writeFileAt(t, root, "usermaps/zm_same/zone_source/all/assetinfo/zm_same.errorlog", "return 1000\n^3"+bullets+"\n", before.Add(13*time.Second))
	writeFileAt(t, root, "usermaps/zm_near/zone_source/all/assetinfo/zm_near.deps", "version,1,1\n", before)
	writeFileAt(t, root, "usermaps/zm_near/zone_source/french/assetinfo/fr_zm_near.errorlog", "return 1000\n^3w\n", before.Add(-time.Second)) // file times are coarse
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/all/assetinfo/zm_stale_old.deps", "version,1,1\n", before.Add(-3*time.Hour))
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/all/assetinfo/zm_stale.deps", "version,1,1\n", before)
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/all/assetinfo/zm_stale.errorlog", "return 1000\n^3w\n", before.Add(-time.Hour)) // an earlier link's
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/japanese/assetinfo/zm_stale.deps", "version,1,1\n", before.Add(-2*time.Hour))
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/japanese/assetinfo/ja_zm_stale.errorlog", "return 1\n^1ERROR: e\n", before.Add(-2*time.Hour+time.Second))
	writeFileAt(t, root, "usermaps/zm_stale/zone_source/japanese/assetinfo/zm_stale.deps.partial", "version,1,1\n", before.Add(-2*time.Hour+time.Second))
	writeFileAt(t, root, "mods/m_failed/zone_source/all/assetinfo/m_failed.deps", "version,1,1\n", before)
	writeFileAt(t, root, "mods/m_failed/zone_source/all/assetinfo/m_failed.errorlog", "return 1\n^1ERROR: e\n", before.Add(time.Second))
	writeFileAt(t, root, "usermaps/zm_stopped/zone_source/all/assetinfo/zm_stopped.deps", "version,1,1\n", before.Add(-time.Hour))
	writeFileAt(t, root, "usermaps/zm_stopped/zone_source/all/assetinfo/zm_stopped.errorlog", "return 1000\n^3w\n", before.Add(-time.Hour+time.Second))
	writeFileAt(t, root, "usermaps/zm_stopped/zone_source/all/assetinfo/zm_stopped.deps.partial", "version,1,1\n", before)
	writeFileAt(t, root, "usermaps/zm_missing/zone_source/all/assetinfo/zm_missing.deps", "version,1,1\nxmodel,m\n\tgdt,image,img_a,\n", before)
	var said []string
	w := &RecordWatcher{Root: root, Out: func(s string) { said = append(said, s) }}
	w.Tick()
	for _, name := range []string{"zm_same", "zm_near", "zm_stale", "zm_stopped"} {
		writeRecord(t, w, "usermaps/"+name+"/zone_source/all/assetinfo/"+name+".deps", "version,1,1\n")
	}
	writeRecord(t, w, "usermaps/zm_same/zone_source/all/assetinfo/zm_same.errorlog", "return 1000\n^3"+bullets+"\n")
	writeRecord(t, w, "usermaps/zm_near/zone_source/all/assetinfo/zm_near.errorlog", "return 1000\n^3w\n")
	writeRecord(t, w, "usermaps/zm_stale/zone_source/all/assetinfo/zm_stale.errorlog", "return 1000\n^3w\n")
	writeRecord(t, w, "usermaps/zm_stopped/zone_source/all/assetinfo/zm_stopped.errorlog", "return 1000\n^3w\n")
	writeRecord(t, w, "mods/m_failed/zone_source/all/assetinfo/m_failed.deps", "version,1,1\n")
	writeRecord(t, w, "usermaps/zm_missing/zone_source/all/assetinfo/zm_missing.deps", "version,1,1\nxmodel,m\n\tgdt,image,img_a,\n")
	w.Tick()
	want := []string{
		"link of mods/m_failed ok again",
		"link of usermaps/zm_missing ok, but 1 asset(s) it named are in no GDT and it linked on without them: image img_a",
		"link of usermaps/zm_stale built, with 1 warning(s): w; all in usermaps/zm_stale/zone_source/all/assetinfo/zm_stale.errorlog",
		"link of usermaps/zm_stopped built again, with the same 1 warning(s) as before",
	}
	if !reflect.DeepEqual(said, want) {
		t.Errorf("each link against the one before the watch:\n got %q\nwant %q", said, want)
	}
}

func TestShortWritesPathsUnderTheRootFromIt(t *testing.T) {
	for _, c := range []struct{ root, msg, want string }{
		{`D:\Games\BO3`, `Found 111 bad bulletmeshes, dumped to D:\Games\BO3\/usermaps\zm_x/zone_source/all/assetinfo/zm_x_bulletreport.csv file.`,
			`Found 111 bad bulletmeshes, dumped to usermaps/zm_x/zone_source/all/assetinfo/zm_x_bulletreport.csv file.`},
		{`D:\Games\BO3\`, `d:/games/bo3/mods\m\a.csv and D:\Games\BO3\mods\m\b.csv`, `mods/m/a.csv and mods/m/b.csv`},
		{`D:\Games\BO3`, `D:\Games\BO3x\a\b.csv stays`, `D:\Games\BO3x\a\b.csv stays`},
		{`D:\Games\BO3`, `in D:\Games\BO3\`, `in `},
		{`D:\Games\BO3`, `D:\Games\BO3\ then a\b`, ` then a\b`},
		{`D:\Games\BO3`, `no path\here`, `no path\here`},
		{``, `a/b`, `a/b`},
		{`/`, `/usr/x`, `/usr/x`},
	} {
		w := &RecordWatcher{Root: c.root}
		if got := w.short(c.msg); got != c.want {
			t.Errorf("root %q: short(%q) = %q, want %q", c.root, c.msg, got, c.want)
		}
	}
}
