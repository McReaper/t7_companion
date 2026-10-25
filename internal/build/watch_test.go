package build

import (
	"os"
	"path/filepath"
	"reflect"
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
	p := filepath.Join(w.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	at := w.start.Add(after)
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
	want := []string{
		"link of mods/my_mod ok",
		"link of usermaps/zm_x built, with 1 warning(s): a french warning; all in usermaps/zm_x/zone_source/french/assetinfo/fr_zm_x.errorlog, " +
			"but 4 asset(s) it named are in no GDT and it linked on without them: image img_a | image img_b | image img_c | and 1 more",
	}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("each target's link, by name:\n got %q\nwant %q", *said, want)
	}
}

func TestRecordWatcherWaitsForRecordsToSettleAndSaysEachOnce(t *testing.T) {
	w, said := recordWatch(t, nil)
	w.Settle = time.Hour
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n")
	w.Tick()
	if len(*said) != 0 {
		t.Fatalf("nothing before the records settle: %q", *said)
	}
	w.Settle = 0
	w.Tick()
	w.Tick()
	if want := []string{"link of usermaps/zm_x ok"}; !reflect.DeepEqual(*said, want) {
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
	runs[compilerExe] = false
	w.Tick()
	want := []string{"compile of map_source/zm/zm_x.map: the map leaks, and the leakfile map_source/zm/zm_x.lin traces the hole to the void (t7kb:mapping); " +
		"map_source/zm/zm_x.errlog places 4 problem(s): entity 0 brush 1 at 1 2 3: Brush has a bad plane | entity 0 brush 2 at 1 2 3: Brush has a bad plane | " +
		"entity 0 brush 3 at 1 2 3: Brush has a bad plane | and 1 more"}
	if !reflect.DeepEqual(*said, want) {
		t.Errorf("a compile's problems, and nothing for one with none:\n got %q\nwant %q", *said, want)
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
