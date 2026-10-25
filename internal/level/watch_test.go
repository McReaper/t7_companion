package level

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// radiantWatch is a Watcher over a machine running one fake Radiant, already
// followed; lines collects what it says.
type radiantWatch struct {
	w     *Watcher
	m     *radianttest.Machine
	f     *radianttest.Fake
	mu    sync.Mutex
	lines []string
}

func newRadiantWatch(t *testing.T, quiet time.Duration) *radiantWatch {
	t.Helper()
	f := radianttest.New(7, `C:\bo3\map_source\zm\zm_a.map`)
	rw := &radiantWatch{f: f, m: radianttest.NewMachine(f)}
	rw.w = &Watcher{Machine: rw.m, Quiet: quiet, Out: func(s string) {
		rw.mu.Lock()
		rw.lines = append(rw.lines, s)
		rw.mu.Unlock()
	}}
	rw.w.Tick()
	if rw.w.follows[7] == nil {
		t.Fatal("the watch follows the Radiant whose bridge listens")
	}
	return rw
}

// said ticks until the watch has said n lines in all, and returns them.
func (rw *radiantWatch) said(t *testing.T, n int) []string {
	t.Helper()
	for i := 0; i < 200; i++ {
		rw.w.Tick()
		rw.mu.Lock()
		got := append([]string(nil), rw.lines...)
		rw.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	t.Fatalf("the watch said %d lines, not %d: %q", len(rw.lines), n, rw.lines)
	return nil
}

func TestWatcherSaysABurstOfRadiantsErrorsInOneLine(t *testing.T) {
	rw := newRadiantWatch(t, 200*time.Millisecond) // the burst may reach the watch over several ticks
	for _, l := range []string{"ERROR: Can't find xmodel 'x' in gdtDB", "Loading prefab _prefabs/a.map...", "ERROR: Can't find xmodel 'x' in gdtDB",
		"ERROR: Can't find material 'm' in gdtDB", "ERROR: a", "ERROR: b"} {
		rw.f.Event("console", l)
	}
	got := rw.said(t, 1)
	if want := "Radiant (pid 7) printed 5 error(s): Can't find xmodel 'x' in gdtDB | Can't find material 'm' in gdtDB | a | and 1 more"; got[0] != want {
		t.Errorf("one line for the burst, each error once:\n got %q\nwant %q", got[0], want)
	}
}

func TestWatcherSaysMapsAndPrefabsButNotSaves(t *testing.T) {
	rw := newRadiantWatch(t, 0)
	rw.f.Event("map", `loaded C:\bo3\map_source\zm\zm_a.map`) // what it has open already
	rw.f.Event("map", `saved C:\bo3\map_source\zm\zm_a.map`)
	rw.f.Event("map", `scope 1 C:\bo3\map_source\_prefabs\zm\door.map`)
	rw.f.Event("map", `scope 1 C:\bo3\map_source\_prefabs\zm\door.map`)
	rw.f.Event("map", `scope x C:\bo3\map_source\_prefabs\zm\door.map`)
	rw.f.Event("map", `scope 0 C:\bo3\map_source\zm\zm_a.map`)
	rw.f.Event("map", `loaded D:\scratch\zm_b.map`)
	rw.f.Event("map", "renamed whatever")
	want := []string{
		"Radiant (pid 7) entered the prefab _prefabs/zm/door.map: the map tools act on it until Radiant leaves it",
		"Radiant (pid 7) left its prefabs, back on zm/zm_a.map",
		"Radiant (pid 7) opened zm_b.map: map_find reads what is in it",
	}
	if got := rw.said(t, 3); !reflect.DeepEqual(got, want) {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

func TestWatcherSaysABakeWithWhatItWarnedOf(t *testing.T) {
	rw := newRadiantWatch(t, 0)
	for _, l := range []string{"Sun bake Done (0.24 s)", "Too many shadowing omni lights at 1 2 3", "Too many shadowing omni lights at 4 5 6",
		"Probe: 0, 94.7 free space skip, 100.0 clear, 5.3 percent valid", "All Probes Done (0.80 s)", "All Probes Done (0.10 s)"} {
		rw.f.Event("console", l)
	}
	want := []string{
		"Radiant (pid 7) finished a bake (All Probes Done (0.80 s)), warning of Too many shadowing omni lights at 1 2 3 (and 1 more like it): map_view under real lighting shows it",
		"Radiant (pid 7) finished a bake (All Probes Done (0.10 s)): map_view under real lighting shows it",
	}
	if got := rw.said(t, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

func TestWatcherSaysARadiantThatClosedAfterItsLastErrors(t *testing.T) {
	rw := newRadiantWatch(t, time.Hour)
	rw.f.Event("console", "ERROR: Can't find xmodel 'x' in gdtDB")
	time.Sleep(20 * time.Millisecond)
	rw.w.Tick()
	if len(rw.lines) != 0 {
		t.Fatalf("errors wait for the console to go quiet: %q", rw.lines)
	}
	rw.m.Exit(7)
	want := []string{"Radiant (pid 7) printed 1 error(s): Can't find xmodel 'x' in gdtDB",
		"Radiant (pid 7), on zm/zm_a.map, closed: radiant_open opens a map again"}
	if got := rw.said(t, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	if rw.w.follows[7] != nil {
		t.Error("a closed Radiant is followed no more")
	}
}

func TestWatcherLeavesABridgeThatRefusedItAlone(t *testing.T) {
	f := radianttest.New(8, `C:\bo3\map_source\zm\zm_a.map`)
	f.Plugin = "1.0.0" // older than the client's minimum: hello fails
	w := &Watcher{Machine: radianttest.NewMachine(f), Out: func(s string) { t.Errorf("said %q", s) }}
	w.Tick()
	w.Tick()
	hellos := 0
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, "hello") {
			hellos++
		}
	}
	if hellos != 1 || w.follows[8] != nil {
		t.Errorf("one try, then a wait: %d hellos", hellos)
	}
	g := radianttest.New(9, `C:\bo3\map_source\zm\zm_a.map`)
	g.Handle("subscribe", func(*radianttest.Fake, string, []byte) radianttest.Reply { return radianttest.Err("refused", "no") })
	w = &Watcher{Machine: radianttest.NewMachine(g), Out: func(s string) { t.Errorf("said %q", s) }}
	w.Tick()
	if w.follows[9] != nil || w.failed[9].IsZero() {
		t.Error("a bridge that refuses the events is left alone too")
	}
}

func TestMapName(t *testing.T) {
	for in, want := range map[string]string{"": "no map", `D:\BO3\Map_Source\zm\x.map`: "zm/x.map", `C:\tmp\y.map`: "y.map", "/map_source/zm/z.map": "zm/z.map"} {
		if got := mapName(in); got != want {
			t.Errorf("mapName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinCappedCountsWhatItLeavesOut(t *testing.T) {
	if got := joinCapped([]string{"a", "b", "c"}, 3); got != "a | b | c" {
		t.Errorf("three, all said: %q", got)
	}
	if got := joinCapped([]string{"a", "b", "c", "d"}, 4); got != "a | b | c | and 1 more" {
		t.Errorf("four: %q", got)
	}
}
