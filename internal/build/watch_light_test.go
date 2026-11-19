package build

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// staleLine is the line a link of usermaps/zm_x says when its map was lit at
// led and compiled at bsp.
func staleLine(led, bsp time.Time) string {
	return "link of usermaps/zm_x used lighting baked " + led.Format(ledStamp) + ", before the map's compile of " + bsp.Format(ledStamp) +
		": lights, emissives and probes changed since show in game only once the map is lit again (Light in the Launcher, or build's light stage)"
}

func TestRecordWatcherSaysOncePerBakeThatALinkUsedLightingOlderThanItsCompile(t *testing.T) {
	w, said := recordWatch(t, nil)
	const led, bsp, deps = "share/raw/maps/zm/zm_x.led", "share/raw/maps/zm/zm_x.d3dbsp", "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps"
	link := func(after time.Duration) []string {
		t.Helper()
		*said = nil
		writeRecordAt(t, w, deps, "version,1,1\n", after)
		w.Tick()
		return *said
	}
	writeRecordAt(t, w, led, "", -2*time.Hour) // a bake from before the watch began
	writeRecordAt(t, w, bsp, "", -time.Hour)
	if got, want := link(time.Second), []string{staleLine(w.start.Add(-2*time.Hour), w.start.Add(-time.Hour))}; !reflect.DeepEqual(got, want) {
		t.Errorf("a link after a compile the bake predates:\n got %q\nwant %q", got, want)
	}
	if got := link(2 * time.Second); len(got) != 0 {
		t.Errorf("said once per bake, not on each link: %q", got)
	}
	writeRecordAt(t, w, led, "", 10*time.Second)
	if got := link(20 * time.Second); len(got) != 0 {
		t.Errorf("a bake after the compile is current: %q", got)
	}
	writeRecordAt(t, w, bsp, "", 30*time.Second)
	if got, want := link(40*time.Second), []string{staleLine(w.start.Add(10*time.Second), w.start.Add(30*time.Second))}; !reflect.DeepEqual(got, want) {
		t.Errorf("a compile after that bake makes it stale again:\n got %q\nwant %q", got, want)
	}
	if err := os.Remove(filepath.Join(w.Root, filepath.FromSlash(led))); err != nil {
		t.Fatal(err)
	}
	if got := link(50 * time.Second); len(got) != 0 {
		t.Errorf("a map with no .led has no bake to call stale: %q", got)
	}
}

func TestRecordWatcherSaysNoStaleLightingWhereThereIsNone(t *testing.T) {
	for _, c := range []struct {
		name    string
		records map[string]time.Duration // each file and when it was written, from the watch's start
		body    string                   // the errorlog the link writes, if any
		want    []string
	}{
		{"a bake within 2 s of the compile", map[string]time.Duration{"share/raw/maps/zm/zm_x.led": -time.Hour, "share/raw/maps/zm/zm_x.d3dbsp": -time.Hour + time.Second, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps": time.Second},
			"", nil},
		{"a map never lit", map[string]time.Duration{"share/raw/maps/zm/zm_x.d3dbsp": -time.Hour, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps": time.Second},
			"", nil},
		{"a link that failed", map[string]time.Duration{"share/raw/maps/zm/zm_x.led": -2 * time.Hour, "share/raw/maps/zm/zm_x.d3dbsp": -time.Hour, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog": time.Second},
			"return 1\n^1ERROR: bad\n", []string{"link of usermaps/zm_x failed with 1 error(s): ERROR: bad; all in usermaps/zm_x/zone_source/all/assetinfo/zm_x.errorlog"}},
		{"a mod", map[string]time.Duration{"share/raw/maps/zm/zm_x.led": -2 * time.Hour, "share/raw/maps/zm/zm_x.d3dbsp": -time.Hour, "mods/zm_x/zone_source/all/assetinfo/zm_x.deps": time.Second},
			"", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, said := recordWatch(t, nil)
			for rel, at := range c.records {
				body := ""
				if strings.HasSuffix(rel, ".errorlog") {
					body = c.body
				}
				writeRecordAt(t, w, rel, body, at)
			}
			w.Tick()
			if !reflect.DeepEqual(*said, c.want) {
				t.Errorf("\n got %q\nwant %q", *said, c.want)
			}
		})
	}
}

func TestRecordWatcherSaysTheStaleLightingOfALinkT7kbRan(t *testing.T) {
	w, said := recordWatch(t, nil)
	w.Builds = func() []State { return []State{{Target: "zm_x", Kind: "usermap", PID: 7, Started: w.start}} }
	writeRecordAt(t, w, "share/raw/maps/zm/zm_x.led", "", -2*time.Hour)
	writeRecordAt(t, w, "share/raw/maps/zm/zm_x.d3dbsp", "", -time.Hour)
	writeRecord(t, w, "usermaps/zm_x/zone_source/all/assetinfo/zm_x.deps", "version,1,1\n")
	w.Tick()
	if want := []string{staleLine(w.start.Add(-2*time.Hour), w.start.Add(-time.Hour))}; !reflect.DeepEqual(*said, want) {
		t.Errorf("the build's line says its link; the lighting is said all the same:\n got %q\nwant %q", *said, want)
	}
}
