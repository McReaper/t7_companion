package level

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
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
			o, _ := Origin(e)
			mn, _ := e.Key("size_min")
			mx, _ := e.Key("size_max")
			a, _ := ParseVec3(mn)
			b, _ := ParseVec3(mx)
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

// prefabMap is a prefab's map: world brushes, then entities.
func prefabMap(t *testing.T, world []string, ents ...string) *mapfile.Map {
	t.Helper()
	var text strings.Builder
	text.WriteString("iwmap 4\n// entity 0\n{\n\"classname\" \"worldspawn\"\n")
	for k, b := range world {
		fmt.Fprintf(&text, "// brush %d\n%s", k, b)
	}
	text.WriteString("}\n")
	for k, e := range ents {
		fmt.Fprintf(&text, "// entity %d\n%s", k+1, e)
	}
	m, err := mapfile.Parse([]byte(text.String()))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func box6(b [6]float64) Box {
	return Box{Min: mapfile.Vec3{b[0], b[1], b[2]}, Max: mapfile.Vec3{b[3], b[4], b[5]}, Ok: true}
}

// A room inside a prefab inside a prefab, the inner one at half size scaled
// 2, the outer one turned: the probe fills the room where the prefabs put it,
// through a pillar of clip_ai and one nonColliding, on a floor in a
// func_group; a prefab whose file isn't there is left out, said. Without
// the files, the walk finds no room, and says why.
func TestProbesStandOnPrefabs(t *testing.T) {
	var world []string
	for _, b := range [][6]float64{ // 128 by 64 inside, 56 high, walls 8 thick
		{-8, -8, 0, 0, 72, 56}, {128, -8, 0, 136, 72, 56}, {0, -8, 0, 128, 0, 56}, {0, 64, 0, 128, 72, 56}, {-8, -8, 56, 136, 72, 64},
	} {
		world = append(world, BoxBrush(box6(b), "brick", ""))
	}
	world = append(world, BoxBrush(box6([6]float64{60, 28, 0, 68, 36, 56}), "clip_ai", ""), BoxBrush(box6([6]float64{30, 28, 0, 38, 36, 56}), "brick", "nonColliding"))
	half := prefabMap(t, world, EntityText("func_group", nil, BoxBrush(box6([6]float64{-8, -8, -8, 136, 72, 0}), "concrete", "")))
	outer := prefabMap(t, nil,
		EntityText("misc_prefab", map[string]string{"model": "_prefabs/test/half.map", "origin": "0 0 0", "modelscale": "2"}),
		EntityText("misc_prefab", map[string]string{"model": "_prefabs/test/missing.map", "origin": "0 0 -1000"}))
	read := func(model string) (*mapfile.Map, error) {
		switch model {
		case "_prefabs/test/half.map":
			return half, nil
		case "_prefabs/test/outer.map":
			return outer, nil
		}
		return nil, fmt.Errorf("no %s", model)
	}
	op := `[{"entity": {"class": "misc_prefab", "origin": [7000, 0, 0], "angles": [0, 90, 0], "keys": {"model": "_prefabs/test/outer.map"}}},
		{"probes": {"at": [6936, 128, 8]}}]`
	d, ps, err := probesRun(t, 953, op, WithPrefabs(read))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6872, 0, 0}, mapfile.Vec3{7000, 256, 112}) // turned 90: its x along the world's y, its y back along -x
	if !strings.Contains(strings.Join(d.Notes, "\n"), "missing.map left out: no _prefabs/test/missing.map") {
		t.Errorf("notes %q: the missing prefab", d.Notes)
	}
	if _, _, err := probesRun(t, 954, op); err == nil || !strings.Contains(err.Error(), "outer.map left out") {
		t.Errorf("unread prefab: %v", err)
	}
}

// Replace deletes the probes in the boxes; without it they stay, said.
func TestProbesReplace(t *testing.T) {
	base := room(6000, 0, 6256, 256, 0, "") + `, {"entity": {"class": "reflection_probe", "origin": [6128, 128, 56]}}`
	before := countClass(&Done{After: fixture(t, 955)}, "reflection_probe")
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
	if n := countClass(d, "reflection_probe"); n != before+2 || !strings.Contains(strings.Join(d.Notes, "\n"), "kept") {
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
			null, _ = Origin(e)
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
	for want, op := range map[string]string{
		"no floor to stand on":       `[` + r + `, {"probes": {"at": [6128, 128, -100]}}]`,
		"off the map":                `[` + r + `, {"probes": {"at": [90000, 0, 0]}}]`,
		"a player's 72":              `[` + r + `, {"probes": {"at": [6128, 128, 8], "height": 40}}]`,
		"no info_player_start":       `[` + r + `, {"probes": {}}]`,
		"not one map_edit can write": `[` + r + `, {"probes": {"at": [6128, 128, 8], "keys": {"a\"b": "1"}}}]`,
	} {
		if _, _, err := probesRun(t, 960, op); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
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
