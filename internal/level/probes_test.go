package level

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// A probe the op made: its box and keys.
type madeProbe struct {
	lo, hi, origin mapfile.Vec3
	keys           map[string]string
}

func probesRun(t *testing.T, pid int, opsJSON string, opts ...EditOption) (*Done, []madeProbe, error) {
	t.Helper()
	c, snap := zoning(t, pid)
	d, err := EditWith(context.Background(), c, snap, ops(t, opsJSON), "", nil, nil, opts...)
	if err != nil {
		return nil, nil, err
	}
	var out []madeProbe
	for _, m := range d.Made {
		if !strings.HasPrefix(m.Part, "probe ") {
			continue
		}
		for _, e := range d.After.Map.Entities {
			if hexOf(e.Guid()) != hexOf(m.ID) {
				continue
			}
			o, _ := geom.Origin(e)
			mn, _ := e.Key("size_min")
			mx, _ := e.Key("size_max")
			a, _ := geom.ParseVec3(mn)
			b, _ := geom.ParseVec3(mx)
			p := madeProbe{origin: o, lo: mapfile.Vec3{o[0] - a[0], o[1] - a[1], o[2] - a[2]}, hi: mapfile.Vec3{o[0] + b[0], o[1] + b[1], o[2] + b[2]}, keys: map[string]string{}}
			for _, k := range []string{"resolution", "ao_range", "blend_mins", "blend_maxs", "box", "lighting_state"} {
				p.keys[k], _ = e.Key(k)
			}
			out = append(out, p)
		}
	}
	return d, out, nil
}

// room is a test room's ops: its walls (a closed line, the doors given),
// floor and ceiling slabs.
func room(x0, y0, x1, y1, z float64, doors string) string {
	return fmt.Sprintf(`{"walls": {"material": "brick", "z": [%g, %g], "lines": [{"points": [[%g, %g], [%g, %g], [%g, %g], [%g, %g]], "closed": true}], "doors": [%s]}},
		{"box": {"min": [%g, %g, %g], "max": [%g, %g, %g], "material": "concrete"}},
		{"box": {"min": [%g, %g, %g], "max": [%g, %g, %g], "material": "concrete"}}`,
		z, z+112, x0, y0, x1, y0, x1, y1, x0, y1, doors, x0-4, y0-4, z-16, x1+4, y1+4, z, x0-4, y0-4, z+112, x1+4, y1+4, z+128)
}

func vecNear(a, b mapfile.Vec3) bool {
	for k := range 3 {
		if math.Abs(a[k]-b[k]) > 0.01 {
			return false
		}
	}
	return true
}

func wantBox(t *testing.T, ps []madeProbe, lo, hi mapfile.Vec3) {
	t.Helper()
	for _, p := range ps {
		if vecNear(p.lo, lo) && vecNear(p.hi, hi) {
			return
		}
	}
	var got []string
	for _, p := range ps {
		got = append(got, fmt.Sprintf("%v-%v", p.lo, p.hi))
	}
	t.Errorf("no probe box %v-%v among %s", lo, hi, strings.Join(got, " "))
}

func noProbeOverlap(t *testing.T, ps []madeProbe) {
	t.Helper()
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			a, b := ps[i], ps[j]
			ov := 1.0
			for k := range 3 {
				ov *= math.Max(0, math.Min(a.hi[k], b.hi[k])-math.Max(a.lo[k], b.lo[k]))
			}
			if ov > 1 {
				t.Errorf("probes %v-%v and %v-%v overlap", a.lo, a.hi, b.lo, b.hi)
			}
		}
	}
}

// Two rooms and a door between them: a box each, from wall face to wall
// face, floor to ceiling, Treyarch's defaults, blends of 8 on the walls;
// the doorway, under its lintel, in neither. A sealed closet beside them
// gets none.
func TestProbesFillTheRoomsPlayersReach(t *testing.T) {
	d, ps, err := probesRun(t, 950, `[`+
		room(6000, 0, 6512, 256, 0, "")+`,
		{"walls": {"material": "brick", "lines": [{"points": [[6256, 0], [6256, 256]]}], "doors": [{"at": [6256, 128]}]}},
		`+room(6000, 256, 6128, 384, 0, "")+`,
		{"probes": {"at": [6128, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("%d probes, want one a room: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 4, 0}, mapfile.Vec3{6252, 252, 112})
	wantBox(t, ps, mapfile.Vec3{6260, 4, 0}, mapfile.Vec3{6508, 252, 112})
	for _, p := range ps {
		if p.keys["resolution"] != "4x" || p.keys["ao_range"] != "8" || p.keys["box"] != "1" {
			t.Errorf("keys %v", p.keys)
		}
		if p.keys["blend_mins"] != "3 3 3" || p.keys["blend_maxs"] != "3 3 3" {
			t.Errorf("blends %s / %s, want 3 against every wall", p.keys["blend_mins"], p.keys["blend_maxs"])
		}
		if p.origin[2] != 56 {
			t.Errorf("origin %v: 56 over the floor", p.origin)
		}
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "in no box") {
		t.Errorf("notes %q: the doorway's cells", d.Notes)
	}
	for _, e := range d.After.Map.Entities {
		if v, _ := e.Key("targetname"); v == "global_probe" {
			t.Errorf("the global probe moved without global: %s", e.Bytes())
		}
	}
}

// Two storeys, a stair along a wall climbing through an opening in the floor
// between them: the ground floor's boxes stop at its ceiling, the upper
// floor's start on its floor, the stair's opening is a box of its own from
// the steps to the roof, none overlapping.
func TestProbesKeepStoreysApart(t *testing.T) {
	var steps []string
	for k := range 16 { // 8 up every 16 along x, the last one level with the upper floor
		steps = append(steps, fmt.Sprintf(`{"box": {"min": [%d, 4, 0], "max": [%d, 68, %d], "material": "concrete"}}`, 6252+16*k, 6268+16*k, 8*(k+1)))
	}
	_, ps, err := probesRun(t, 951, `[
		{"walls": {"material": "brick", "z": [0, 240], "lines": [{"points": [[6000, 0], [6512, 0], [6512, 256], [6000, 256]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6516, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, 68, 112], "max": [6516, 260, 128], "material": "concrete"}},
		{"box": {"min": [5996, -4, 112], "max": [6320, 68, 128], "material": "concrete"}},
		{"box": {"min": [5996, -4, 240], "max": [6516, 260, 256], "material": "concrete"}},
		`+strings.Join(steps, ",")+`,
		{"probes": {"at": [6128, 160, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	noProbeOverlap(t, ps)
	var low, high, stair bool
	for _, p := range ps {
		switch {
		case p.lo[2] == 0 && p.hi[2] == 112:
			low = true
		case p.lo[2] == 128 && p.hi[2] == 240:
			high = true
		case p.lo[2] < 112 && p.hi[2] == 240 && p.lo[0] >= 6320 && p.hi[1] <= 72:
			stair = true
		}
	}
	if !low || !high || !stair {
		t.Errorf("probes %+v: want the ground floor's, the upper floor's and the stair's", ps)
	}
}

// Replace deletes the probes in the boxes; without it they stay, said.
func TestProbesReplace(t *testing.T) {
	base := room(6000, 0, 6256, 256, 0, "") + `, {"entity": {"class": "reflection_probe", "origin": [6128, 128, 56]}}`
	for _, o := range []string{"5900 128 56", "6400 128 56", "6128 -100 56", "6128 400 56", "6128 128 -50", "6128 128 300"} { // just out of the box, one way each
		base += `, {"entity": {"class": "reflection_probe", "origin": [` + strings.ReplaceAll(o, " ", ", ") + `]}}`
	}
	before := countClass(&Done{After: fixture(t, 955)}, "reflection_probe") + 6
	d, ps, err := probesRun(t, 956, `[`+base+`, {"probes": {"at": [6128, 128, 8], "replace": true}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if n := countClass(d, "reflection_probe"); len(ps) != 1 || n != before+1 {
		t.Errorf("replace: %d probes in the map, %d made, %d before", n, len(ps), before)
	}
	d, ps, err = probesRun(t, 957, `[`+base+`, {"probes": {"at": [6128, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if n := countClass(d, "reflection_probe"); n != before+2 || !strings.Contains(strings.Join(d.Notes, "\n"), "probes: 1 probes already stand in the boxes, kept") {
		t.Errorf("kept: %d probes, %d made, notes %q", n, len(ps), d.Notes)
	}
}

func countClass(d *Done, class string) int {
	n := 0
	for _, e := range d.After.Map.Entities {
		if e.Class() == class {
			n++
		}
	}
	return n
}

func fixture(t *testing.T, pid int) *Snapshot {
	t.Helper()
	_, snap := zoning(t, pid)
	return snap
}

// global moves the global probe into the largest box, though a smaller room
// with a lower ceiling comes first: an info_null the volume_sun targets. The
// walk starts at the info_player_start; keys go on every probe.
func TestProbesMoveTheGlobalProbe(t *testing.T) {
	rooms := `{"walls": {"material": "brick", "z": [0, 160], "lines": [{"points": [[6000, 0], [6768, 0], [6768, 256], [6000, 256]], "closed": true}, {"points": [[6256, 0], [6256, 256]]}], "doors": [{"at": [6256, 128]}]}},
		{"box": {"min": [5996, -4, -16], "max": [6772, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 112], "max": [6256, 260, 128], "material": "concrete"}},
		{"box": {"min": [6256, -4, 160], "max": [6772, 260, 176], "material": "concrete"}},
		{"entity": {"class": "info_player_start", "origin": [6128, 128, 0]}}`
	d, ps, err := probesRun(t, 958, `[`+rooms+`, {"probes": {"global": true, "keys": {"lighting_state": "1"}}}]`)
	if err != nil {
		t.Fatal(err)
	}
	var null mapfile.Vec3
	found := false
	for _, e := range d.After.Map.Entities {
		if v, _ := e.Key("targetname"); e.Class() == "info_null" && v == "global_probe" {
			null, _ = geom.Origin(e)
			found = true
		}
		if v, _ := e.Key("target"); e.Class() == "volume_sun" && v != "global_probe" {
			t.Errorf("the volume_sun targets %q", v)
		}
	}
	if !found || !vecNear(null, mapfile.Vec3{6512, 128, 56}) {
		t.Errorf("global probe at %v (found %v), boxes %+v", null, found, ps)
	}
	if len(ps) != 2 {
		t.Errorf("%d probes, want one a room: %+v", len(ps), ps)
	}
	for _, p := range ps {
		if p.keys["lighting_state"] != "1" {
			t.Errorf("probe keys %v: the keys given on every probe", p.keys)
		}
	}
	d, _, err = probesRun(t, 959, `[`+rooms+`, {"delete": ["6E73936D"]}, {"probes": {"global": true}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "no volume_sun") {
		t.Errorf("notes %q: no volume_sun to target", d.Notes)
	}
}

func TestProbesRefuse(t *testing.T) {
	r := room(6000, 0, 6256, 256, 0, "")
	for _, c := range [][2]string{
		{"no floor to stand on", `{"at": [6128, 128, -100]}`},
		{"off the map", `{"at": [90000, 128, 0]}`},
		{"off the map", `{"at": [-90000, 128, 0]}`},
		{"off the map", `{"at": [6128, 90000, 0]}`},
		{"off the map", `{"at": [6128, -90000, 0]}`},
		{"a player's 72", `{"at": [6128, 128, 8], "height": 40}`},
		{"no info_player_start", `{}`},
		{"not one map_edit can write", `{"at": [6128, 128, 8], "keys": {"a\"b": "1"}}`},
	} {
		if _, _, err := probesRun(t, 960, `[`+r+`, {"probes": `+c[1]+`}]`); err == nil || !strings.Contains(err.Error(), c[0]) {
			t.Errorf("%s: %v, want %q", c[1], err, c[0])
		}
	}
}

// A map with zones is probed in them: the room no zone holds is left out,
// said, though players walk into it.
func TestProbesKeepToTheZones(t *testing.T) {
	rooms := room(6000, 0, 6512, 256, 0, "") + `,
		{"walls": {"material": "brick", "lines": [{"points": [[6256, 0], [6256, 256]]}], "doors": [{"at": [6256, 128]}]}},
		{"zone": {"name": "west", "areas": [{"min": [6000, 0, 0], "max": [6256, 256, 112]}], "spawners": 0, "pathnode": false}}`
	d, ps, err := probesRun(t, 961, `[`+rooms+`, {"probes": {"at": [6400, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes, want the zoned room's: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 4, 0}, mapfile.Vec3{6252, 252, 112})
	if !strings.Contains(strings.Join(d.Notes, "\n"), "outside every zone") {
		t.Errorf("notes %q: the room left out", d.Notes)
	}
}

// A player drops off a ledge but never climbs one higher than a step: from
// the floor the platform is out of reach, the box stops at its face; from the
// platform, the floor is reached too, and the hall is one box over both.
func TestProbesDropOffLedges(t *testing.T) {
	hall := `{"walls": {"material": "brick", "z": [0, 240], "lines": [{"points": [[6000, 0], [6512, 0], [6512, 256], [6000, 256]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6516, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 240], "max": [6516, 260, 256], "material": "concrete"}},
		{"box": {"min": [6004, 4, 0], "max": [6132, 252, 64], "material": "concrete"}}`
	_, ps, err := probesRun(t, 962, `[`+hall+`, {"probes": {"at": [6300, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("from the floor: %+v", ps)
	}
	wantBox(t, ps, mapfile.Vec3{6132, 4, 0}, mapfile.Vec3{6508, 252, 240})
	_, ps, err = probesRun(t, 963, `[`+hall+`, {"probes": {"at": [6064, 128, 72]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("from the platform: %+v", ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 4, 0}, mapfile.Vec3{6508, 252, 240})
}

// A gap narrower than a body stops the walk; one a body passes doesn't.
func TestProbesPassOnlyWhereABodyFits(t *testing.T) {
	for gap, boxes := range map[float64]int{16: 1, 40: 2} {
		_, ps, err := probesRun(t, 970+int(gap), `[`+room(6000, 0, 6512, 256, 0, "")+fmt.Sprintf(`,
			{"box": {"min": [6252, 4, 0], "max": [6260, %g, 112], "material": "brick"}},
			{"box": {"min": [6252, %g, 0], "max": [6260, 252, 112], "material": "brick"}},
			{"probes": {"at": [6128, 128, 8]}}]`, 128-gap/2, 128+gap/2))
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != boxes {
			t.Errorf("a gap %g wide: %d probes, want %d: %+v", gap, len(ps), boxes, ps)
		}
	}
}

// The capture point stands 56 over the box's floor, half a low space's
// height under 112, and steps out of a solid the middle falls in: 16 one
// way, then the other, then 32.
func TestProbesPlaceTheCapturePoint(t *testing.T) {
	_, ps, err := probesRun(t, 975, `[{"walls": {"material": "brick", "z": [0, 80], "lines": [{"points": [[6000, 0], [6256, 0], [6256, 256], [6000, 256]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6260, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 80], "max": [6260, 260, 96], "material": "concrete"}},
		{"probes": {"at": [6128, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || !vecNear(ps[0].origin, mapfile.Vec3{6128, 128, 40}) {
		t.Errorf("a room 80 high: %+v, want the point at 40", ps)
	}
	var steps []string
	for k := range 16 { // 8 up every 16 along x from 6012, a strip of floor before them
		steps = append(steps, fmt.Sprintf(`{"box": {"min": [%d, 4, 0], "max": [%d, 252, %d], "material": "concrete"}}`, 6012+16*k, 6028+16*k, 8*(k+1)))
	}
	_, ps, err = probesRun(t, 976, `[{"walls": {"material": "brick", "z": [0, 384], "lines": [{"points": [[6000, 0], [6256, 0], [6256, 256], [6000, 256]], "closed": true}]}},
		{"box": {"min": [5996, -4, -16], "max": [6260, 260, 0], "material": "concrete"}},
		{"box": {"min": [5996, -4, 384], "max": [6260, 260, 400], "material": "concrete"}},
		`+strings.Join(steps, ",")+`,
		{"probes": {"at": [6008, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	// the middle, 6128, is in the step 64 high, 6144 in the next, 6112 on top of the one under
	if len(ps) != 1 || !vecNear(ps[0].origin, mapfile.Vec3{6112, 128, 56}) {
		t.Errorf("over a stair: %+v, want the point at 6112 128 56", ps)
	}
}

// The floor kept to a zone reaches 3 cells past its volume, not to the wall
// beyond; a trigger marked player_volume is no zone.
func TestProbesKeepThreeCellsPastAZone(t *testing.T) {
	d, ps, err := probesRun(t, 981, `[`+room(6000, 0, 6256, 256, 0, "")+`,
		{"zone": {"name": "west", "areas": [{"min": [6000, 0, 0], "max": [6160, 256, 112]}], "spawners": 0, "pathnode": false}},
		{"box": {"min": [6160, 0, 0], "max": [6256, 256, 112], "material": "trigger", "class": "trigger_multiple", "keys": {"script_noteworthy": "player_volume"}}},
		{"probes": {"at": [6080, 128, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6004, 4, 0}, mapfile.Vec3{6184, 252, 112})
	if n := strings.Join(d.Notes, "\n"); !strings.Contains(n, "kept to the zones: 248 cells") {
		t.Errorf("notes %q: the 8 by 31 cells past the zone's 3", n)
	}
}

// The walk gives up past maxWalk cells, saying why, rather than run on.
func TestProbesWalkIsBounded(t *testing.T) {
	defer func(n int) { maxWalk = n }(maxWalk)
	maxWalk = 10
	if _, _, err := probesRun(t, 983, `[`+room(6000, 0, 6256, 256, 0, "")+`, {"probes": {"at": [6128, 128, 8]}}]`); err == nil || !strings.Contains(err.Error(), "the walk passed 10 cells") {
		t.Errorf("err = %v", err)
	}
}
