package level

import (
	"fmt"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// An L-shaped room is two boxes meeting edge to edge: the leg and corner
// first (the larger), then the rest, neither snapping into the other.
func TestProbesCutAnLShapedRoom(t *testing.T) {
	d, ps, err := probesRun(t, 964, `[
		{"walls": {"material": "brick", "z": [0, 112], "lines": [{"points": [[6000, 0], [6512, 0], [6512, 128], [6256, 128], [6256, 384], [6000, 384]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6516, 388, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 112], "max": [6516, 388, 128], "material": "concrete"}},
		{"probes": {"at": [6128, 200, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 4, 0}, mapfile.Vec3{6248, 380, 112})
	wantBox(t, ps, mapfile.Vec3{6248, 4, 0}, mapfile.Vec3{6508, 124, 112})
	noProbeOverlap(t, ps)
	if n := strings.Join(d.Notes, "\n"); strings.Contains(n, "in no box") {
		t.Errorf("notes %q: every cell reached is in a box", n)
	}
}

// A strip under a soffit, too narrow for a box of its own, is in none: the
// room's blend reaches over it, 3 cells and 4, and the note counts it.
func TestProbesBlendOverAStripNoBoxHolds(t *testing.T) {
	d, ps, err := probesRun(t, 965, `[`+room(6000, 0, 6256, 256, 0, "")+`,
		{"box": {"min": [6004, 4, 88], "max": [6252, 28, 112], "material": "concrete"}},
		{"probes": {"at": [6128, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 24, 0}, mapfile.Vec3{6252, 252, 112}) // the soffit ends at 28: the row from 24 is mostly the room
	if p := ps[0]; p.keys["blend_mins"] != "3 28 3" || p.keys["blend_maxs"] != "3 3 3" {
		t.Errorf("blends %s / %s, want 28 over the strip, 3 on the walls", p.keys["blend_mins"], p.keys["blend_maxs"])
	}
	if n := strings.Join(d.Notes, "\n"); !strings.Contains(n, "probes: 93 of the 8-unit cells reached are in no box") {
		t.Errorf("notes %q: the strip's 31 by 3 cells", n)
	}
}

// A corridor 48 wide is a box's narrowest, its faces on the 4s of the grid
// or on the 8s; one 40 wide is left to the blends.
func TestProbesBoxCorridorsFromFortyEightWide(t *testing.T) {
	pid := 966
	for _, c := range []struct {
		lo, width float64 // the corridor's inside, from its south face
		boxes     int
	}{{4, 48, 2}, {4, 40, 1}, {0, 48, 2}, {0, 40, 1}} {
		hi := c.lo + c.width
		hall := fmt.Sprintf(`[{"box": {"min": [6000, -16, 0], "max": [6256, 0, 112], "material": "brick"}},
			{"box": {"min": [6000, 256, 0], "max": [6256, 272, 112], "material": "brick"}},
			{"box": {"min": [5984, -16, 0], "max": [6000, 272, 112], "material": "brick"}},
			{"box": {"min": [6256, -16, 0], "max": [6272, %[1]g, 112], "material": "brick"}},
			{"box": {"min": [6256, %[2]g, 0], "max": [6272, 272, 112], "material": "brick"}},
			{"box": {"min": [6272, -16, 0], "max": [6528, %[1]g, 112], "material": "brick"}},
			{"box": {"min": [6272, %[2]g, 0], "max": [6528, %[3]g, 112], "material": "brick"}},
			{"box": {"min": [6512, %[1]g, 0], "max": [6528, %[2]g, 112], "material": "brick"}},
			{"box": {"min": [5984, -16, -16], "max": [6528, 272, 0], "material": "concrete"}},
			{"box": {"min": [5984, -16, 112], "max": [6528, 272, 128], "material": "concrete"}},
			{"probes": {"at": [6128, 128, 8]}}]`, c.lo, hi, hi+16)
		_, ps, err := probesRun(t, pid, hall)
		pid++
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != c.boxes {
			t.Errorf("corridor from %g, %g wide: %d probes, want %d: %+v", c.lo, c.width, len(ps), c.boxes, ps)
		}
	}
}

// holder finds the box holding a cell at a height: the cells whose middles
// its rectangle holds, ends exclusive, from its floor to under its top.
func TestProbeBoxHolder(t *testing.T) {
	g := &probeGrid{n: 10, m: 10}
	boxes := []probeBox{{rlo: [2]float64{64, 64}, rhi: [2]float64{72, 72}}, {rlo: [2]float64{16, 24}, rhi: [2]float64{40, 48}, lo: mapfile.Vec3{0, 0, 0}, hi: mapfile.Vec3{0, 0, 100}}}
	for _, c := range []struct {
		i, j int
		z    float64
		want int
	}{
		{2, 3, 0, 1}, {4, 5, 99.9, 1}, {2, 3, -0.005, 1},
		{1, 3, 50, -1}, {5, 3, 50, -1}, {2, 2, 50, -1}, {2, 6, 50, -1}, {2, 3, 100, -1}, {2, 3, -0.02, -1},
	} {
		if got := g.holder(c.i+c.j*g.n, c.z, boxes); got != c.want {
			t.Errorf("cell %d,%d at %g: box %d, want %d", c.i, c.j, c.z, got, c.want)
		}
	}
	if a := (probeBox{lo: mapfile.Vec3{1, 2, 0}, hi: mapfile.Vec3{4, 7, 9}}).area(); a != 15 {
		t.Errorf("area %g, want 3 by 5", a)
	}
}

// Strips under soffits on all four sides: the room's blend reaches over
// each, as far as it goes, 32 at most.
func TestProbesBlendEveryWay(t *testing.T) {
	_, ps, err := probesRun(t, 979, `[`+room(6000, 0, 6512, 512, 0, "")+`,
		{"box": {"min": [6004, 4, 88], "max": [6508, 28, 112], "material": "concrete"}},
		{"box": {"min": [6004, 492, 88], "max": [6508, 508, 112], "material": "concrete"}},
		{"box": {"min": [6004, 28, 88], "max": [6012, 492, 112], "material": "concrete"}},
		{"box": {"min": [6468, 28, 88], "max": [6508, 492, 112], "material": "concrete"}},
		{"probes": {"at": [6256, 256, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	if p := ps[0]; p.keys["blend_mins"] != "12 28 3" || p.keys["blend_maxs"] != "32 20 3" {
		t.Errorf("box %v-%v, blends %s / %s: want 12 and 32 over the x strips, 28 and 20 over the y ones", p.lo, p.hi, p.keys["blend_mins"], p.keys["blend_maxs"])
	}
}

// A side snaps onto the wall beyond it when three of its five lines meet
// the wall (or four), not two.
func TestProbesSnapWhereMostOfASideIsWalled(t *testing.T) {
	for _, c := range []struct {
		open0, open1 float64 // an opening in the east wall onto a strip too narrow for a box
		hi           float64
	}{{100, 200, 6252}, {80, 176, 6252}, {60, 200, 6248}} {
		_, ps, err := probesRun(t, 980, fmt.Sprintf(`[{"box": {"min": [5996, -4, 0], "max": [6004, 260, 112], "material": "brick"}},
			{"box": {"min": [6004, -4, 0], "max": [6300, 4, 112], "material": "brick"}},
			{"box": {"min": [6004, 252, 0], "max": [6300, 260, 112], "material": "brick"}},
			{"box": {"min": [6252, 4, 0], "max": [6260, %[1]g, 112], "material": "brick"}},
			{"box": {"min": [6252, %[2]g, 0], "max": [6260, 252, 112], "material": "brick"}},
			{"box": {"min": [6292, 4, 0], "max": [6300, 252, 112], "material": "brick"}},
			{"box": {"min": [5996, -4, -16], "max": [6300, 260, 0], "material": "concrete"}},
			{"box": {"min": [5996, -4, 112], "max": [6300, 260, 128], "material": "concrete"}},
			{"probes": {"at": [6128, 128, 8]}}]`, c.open0, c.open1))
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != 1 || ps[0].hi[0] != c.hi {
			t.Errorf("opening %g to %g: %+v, want the east side at %g", c.open0, c.open1, ps, c.hi)
		}
	}
}
