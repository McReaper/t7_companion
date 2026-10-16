package level

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// layoutPieces are a layout's pieces, its defaults filled as the batch fills
// them.
func layoutPieces(t *testing.T, o LayoutOp) []piece {
	t.Helper()
	ps, err := o.pieces()
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

// layoutBoxes are a layout's pieces as boxes, sorted: bounds, material, part.
func layoutBoxes(t *testing.T, o LayoutOp) []string {
	t.Helper()
	var out []string
	for _, p := range layoutPieces(t, o) {
		out = append(out, fmtVec(p.lo)+" "+fmtVec(p.hi)+" "+p.material+" "+p.part)
	}
	sort.Strings(out)
	return out
}

func f64(v float64) *float64 { return &v }

// two rooms side by side, the hall west of the kitchen
func twoRooms() LayoutOp {
	return LayoutOp{Material: "brick", Floor: "wood", Rooms: []PlanRoom{
		{Name: "hall", Min: [2]float64{0, 0}, Max: [2]float64{256, 256}},
		{Name: "kitchen", Min: [2]float64{256, 0}, Max: [2]float64{512, 256}},
	}}
}

func TestALayoutPutsOneWallWhereTwoRoomsMeet(t *testing.T) {
	got := layoutBoxes(t, twoRooms())
	want := []string{
		"4 4 -16 252 252 0 wood hall floor", // between the walls, centred on the plan's lines
		"4 4 112 252 252 128 brick hall ceiling",
		"260 4 -16 508 252 0 wood kitchen floor",
		"260 4 112 508 252 128 brick kitchen ceiling",
		"-4 -4 -16 256 4 128 brick hall wall", // the walls along x take the corners
		"256 -4 -16 516 4 128 brick kitchen wall",
		"-4 252 -16 256 260 128 brick hall wall",
		"256 252 -16 516 260 128 brick kitchen wall",
		"-4 4 -16 4 252 128 brick hall wall", // the walls along y stop against them
		"252 4 -16 260 252 128 brick hall|kitchen wall",
		"508 4 -16 516 252 128 brick kitchen wall",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("pieces:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A wall's sides take the material of the room each looks into, the
// outside's the exterior's; the pieces keep the layout's walls' material for
// their ends and tops.
func TestALayoutsWallsFaceEachRoomWithItsMaterial(t *testing.T) {
	o := twoRooms()
	o.Exterior, o.Rooms[0].Material, o.Rooms[1].Material, o.Rooms[1].Floor = "stone", "plaster", "tile", "lino"
	faces := map[string]string{}
	for _, p := range layoutPieces(t, o) {
		faces[p.part+" "+fmtVec(p.lo)] = fmt.Sprint(p.faces)
		if strings.HasSuffix(p.part, "wall") && p.material != "brick" {
			t.Errorf("%s: %s", p.part, p.material)
		}
		if p.part == "kitchen floor" && p.material != "lino" {
			t.Errorf("the kitchen's own floor: %s", p.material)
		}
	}
	for at, want := range map[string]string{
		"hall|kitchen wall 252 4 -16": "[[-x plaster] [+x tile] [+z brick]]",             // its top under the roof, the walls'
		"hall wall -4 -4 -16":         "[[-y stone] [+y plaster] [+z stone] [-x stone]]", // the corner's end, outside
		"kitchen wall 256 252 -16":    "[[-y tile] [+y stone] [+z stone] [+x stone]]",
		"hall floor 4 4 -16":          "[]",
	} {
		if faces[at] != want {
			t.Errorf("%s: %s, want %s", at, faces[at], want)
		}
	}
	// A wall along x that ends against a wall across a room shows that room
	// at its end; a door's threshold and a window's sill keep their tops.
	o = LayoutOp{Material: "brick", Exterior: "stone", Rooms: []PlanRoom{
		{Name: "big", Min: [2]float64{0, 0}, Max: [2]float64{256, 512}, Material: "plaster"},
		{Name: "a", Min: [2]float64{256, 0}, Max: [2]float64{512, 256}},
		{Name: "b", Min: [2]float64{256, 256}, Max: [2]float64{512, 512}, Outdoor: true}},
		Openings: []Opening{{Room: "a", To: "big"}, {Room: "a", Side: "+x", Sill: 32}}}
	faces = map[string]string{}
	for _, p := range layoutPieces(t, o) {
		faces[p.part+" "+fmtVec(p.lo)+" "+fmtVec(p.hi)] = fmt.Sprint(p.faces)
	}
	for at, want := range map[string]string{
		"a|b wall 252 252 -16 516 260 128": "[[-y brick] [+y stone] [+z stone] [-x plaster] [+x stone]]", // by an outdoor room: its side and top the exterior
		"big|a wall 252 100 -16 260 156 0": "[[-x plaster] [+x brick]]",                                  // the door's threshold
		"a wall 508 100 -16 516 156 32":    "[[-x brick] [+x stone]]",                                    // the window's sill
		"a wall 508 100 96 516 156 128":    "[[-x brick] [+x stone] [+z stone]]",                         // its lintel, to the top
		"big wall -4 -4 -16 256 4 128":     "[[-y stone] [+y plaster] [+z stone] [-x stone]]",
		"big|a wall 252 4 -16 260 100 128": "[[-x plaster] [+x brick] [+z brick]]",
	} {
		if faces[at] != want {
			t.Errorf("%s: %s, want %s", at, faces[at], want)
		}
	}
}

// Where a room meets two, the walls meet in a T: one wall's end against the
// other's side, the corners filled, no two pieces sharing room.
func TestALayoutsWallsMeetWithoutGapsOrOverlaps(t *testing.T) {
	plans := map[string]LayoutOp{
		"two rooms": twoRooms(),
		"a T": {Material: "brick", Rooms: []PlanRoom{
			{Name: "hall", Min: [2]float64{0, 0}, Max: [2]float64{512, 256}},
			{Name: "a", Min: [2]float64{0, 256}, Max: [2]float64{256, 512}},
			{Name: "b", Min: [2]float64{256, 256}, Max: [2]float64{512, 512}},
		}},
		"an L, apart": {Material: "brick", Rooms: []PlanRoom{
			{Name: "a", Min: [2]float64{0, 0}, Max: [2]float64{384, 128}},
			{Name: "b", Min: [2]float64{256, 128}, Max: [2]float64{384, 512}},
			{Name: "c", Min: [2]float64{1000, 1000}, Max: [2]float64{1128, 1128}, Outdoor: true},
		}},
	}
	for name, o := range plans {
		ps := layoutPieces(t, o)
		for i, a := range ps {
			for _, b := range ps[i+1:] {
				if overlap(a, b) {
					t.Errorf("%s: %s %v %v overlaps %s %v %v", name, a.part, a.lo, a.hi, b.part, b.lo, b.hi)
				}
			}
		}
		for _, r := range o.Rooms { // every corner of every room stands in a wall, at any height
			for _, c := range [][2]float64{r.Min, r.Max, {r.Min[0], r.Max[1]}, {r.Max[0], r.Min[1]}} {
				for _, d := range [][2]float64{{-3.9, -3.9}, {3.9, 3.9}, {-3.9, 3.9}, {3.9, -3.9}} {
					if p := (mapfile.Vec3{c[0] + d[0], c[1] + d[1], 64}); !inPiece(ps, p) {
						t.Errorf("%s: %v is in no wall", name, p)
					}
				}
			}
		}
	}
	got := layoutBoxes(t, plans["a T"])
	for _, want := range []string{
		"-4 252 -16 256 260 128 brick hall|a wall", "256 252 -16 516 260 128 brick hall|b wall", // the T's top, in two
		"252 260 -16 260 508 128 brick a|b wall", // its stem, against it
	} {
		if !strings.Contains(strings.Join(got, "\n"), want) {
			t.Errorf("a T lacks %s:\n%s", want, strings.Join(got, "\n"))
		}
	}
	if strings.Contains(strings.Join(layoutBoxes(t, plans["an L, apart"]), "\n"), "c ceiling") {
		t.Error("an outdoor room has no ceiling")
	}
}

func overlap(a, b piece) bool {
	for k := range 3 {
		if min(a.hi[k], b.hi[k])-max(a.lo[k], b.lo[k]) <= eps {
			return false
		}
	}
	return true
}

func inPiece(ps []piece, p mapfile.Vec3) bool {
	for _, q := range ps {
		if p[0] > q.lo[0] && p[0] < q.hi[0] && p[1] > q.lo[1] && p[1] < q.hi[1] && p[2] > q.lo[2] && p[2] < q.hi[2] {
			return true
		}
	}
	return false
}

func TestALayoutsWallsAreBuiltAroundItsOpenings(t *testing.T) {
	o := twoRooms()
	o.Openings = []Opening{{Room: "hall", To: "kitchen"}, {Room: "hall", Side: "-y", At: f64(128), Sill: 32}}
	got := strings.Join(layoutBoxes(t, o), "\n")
	for _, want := range []string{
		// the door, in the middle of the wall the rooms share: 56 wide, 96 high
		"252 4 -16 260 100 128 brick hall|kitchen wall",
		"252 100 -16 260 156 0 wood hall|kitchen wall", // its threshold, of the hall's floor
		"252 100 96 260 156 128 brick hall|kitchen wall",
		"252 156 -16 260 252 128 brick hall|kitchen wall",
		// the window: 56 by 64 over a sill at 32, its middle at x 128
		"-4 -4 -16 100 4 128 brick hall wall",
		"100 -4 -16 156 4 32 brick hall wall",
		"100 -4 96 156 4 128 brick hall wall",
		"156 -4 -16 256 4 128 brick hall wall",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "252 4 -16 260 252 128") || strings.Contains(got, "-4 -4 -16 256 4 128") {
		t.Errorf("a wall left whole around its opening:\n%s", got)
	}
	// From the kitchen's side, a door up to the ceiling leaves only the slab
	// over it, and one flush with the wall's end leaves no sliver before it.
	o.Openings = []Opening{{Room: "kitchen", To: "hall", At: f64(32), Height: 112}}
	got = strings.Join(layoutBoxes(t, o), "\n")
	for _, want := range []string{"252 4 112 260 60 128 brick hall|kitchen wall", "252 60 -16 260 252 128 brick hall|kitchen wall"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %s:\n%s", want, got)
		}
	}
	for _, p := range layoutPieces(t, o) {
		if p.hi[0]-p.lo[0] < 1 || p.hi[1]-p.lo[1] < 1 || p.hi[2]-p.lo[2] < 1 {
			t.Errorf("a sliver %v %v", p.lo, p.hi)
		}
	}
	// Openings a unit apart leave a unit of wall between them.
	o.Openings = []Opening{{Room: "hall", Side: "+y", At: f64(40), Width: 40}, {Room: "hall", Side: "+y", At: f64(81), Width: 40}}
	thin := 0
	for _, p := range layoutPieces(t, o) {
		if p.hi[0]-p.lo[0] == 1 {
			thin++
		}
	}
	if thin != 1 {
		t.Errorf("%d strips a unit wide between the openings", thin)
	}
}

// A layout off the origin and above it: its openings' at and its refusals
// are in world units.
func TestALayoutsOpeningsAreInWorldUnits(t *testing.T) {
	o := LayoutOp{Material: "brick", Z: 128, Height: 144, Rooms: []PlanRoom{{Name: "attic", Min: [2]float64{1000, 2000}, Max: [2]float64{1256, 2128}}},
		Openings: []Opening{{Room: "attic", Side: "+x", At: f64(2032)}}}
	got := strings.Join(layoutBoxes(t, o), "\n")
	for _, want := range []string{
		"1252 2004 112 1260 2004 288", // (none: the door starts flush, no strip before it)
		"1252 2004 112 1260 2060 128 wood",
	} {
		if strings.Contains(got, want) {
			t.Errorf("has %s:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"1252 2004 112 1260 2060 128 brick attic wall", // the threshold: the room's floor is the walls'
		"1252 2004 224 1260 2060 288 brick attic wall", // over a door 96 high, from 128
		"1252 2060 112 1260 2124 288 brick attic wall",
		"1004 2004 272 1252 2124 288 brick attic ceiling",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %s:\n%s", want, got)
		}
	}
	o.Openings[0].At = f64(2110)
	if _, err := o.pieces(); err == nil || !strings.Contains(err.Error(), "56 wide at 2110 runs past the stretch of wall it is in, between the walls that meet it: 2004 to 2124") {
		t.Errorf("an opening past the wall's end: %v", err)
	}
}

func TestALayoutRefusesWhatItCantBuild(t *testing.T) {
	for want, edit := range map[string]func(*LayoutOp){
		"no rooms":                       func(o *LayoutOp) { o.Rooms = nil },
		`material ""`:                    func(o *LayoutOp) { o.Material, o.Floor = "", "wood" },
		`material "two words"`:           func(o *LayoutOp) { o.Rooms[1].Ceiling = "two words" },
		`room 1: name "hall": one word`:  func(o *LayoutOp) { o.Rooms[1].Name = "hall" },
		`room 0: name "the hall"`:        func(o *LayoutOp) { o.Rooms[0].Name = "the hall" },
		`room 0: name "a|b"`:             func(o *LayoutOp) { o.Rooms[0].Name = "a|b" },
		"room hall: [0 0] to [8 256]":    func(o *LayoutOp) { o.Rooms[0].Max[0], o.Rooms[1].Min[0] = 8, 8 },
		"rooms hall and kitchen overlap": func(o *LayoutOp) { o.Rooms[1].Min[0] = 128 },
		"sizes above 0":                  func(o *LayoutOp) { o.Slab = -16 },
		`opening 0: no room "attic"`:     func(o *LayoutOp) { o.Openings = []Opening{{Room: "attic", Side: "-x"}} },
		`opening 0: no room "cellar"`:    func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", To: "cellar"}} },
		"or side, one of them":           func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall"}} },
		"one of them":                    func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", To: "kitchen", Side: "+x"}} },
		`side "up"`:                      func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", Side: "up"}} },
		"rooms hall and kitchen share no": func(o *LayoutOp) {
			o.Rooms[1].Min[0], o.Rooms[1].Max[0] = 300, 600
			o.Openings = []Opening{{Room: "hall", To: "kitchen"}}
		},
		"taller than the storey's 112": func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", Side: "-x", Sill: 32, Height: 96}} },
		"can't be below 0":             func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", Side: "-x", Width: -1}} },
		"two openings in the hall wall": func(o *LayoutOp) {
			o.Openings = []Opening{{Room: "hall", Side: "-x", At: f64(64)}, {Room: "hall", Side: "-x", At: f64(120)}}
		},
		"runs past the stretch": func(o *LayoutOp) { o.Openings = []Opening{{Room: "hall", To: "kitchen", At: f64(20)}} },
		"two openings in the hall|kitchen": func(o *LayoutOp) {
			o.Openings = []Opening{{Room: "hall", To: "kitchen"}, {Room: "kitchen", To: "hall", At: f64(150)}}
		},
	} {
		o := twoRooms()
		edit(&o)
		if _, err := o.pieces(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
	o := twoRooms() // a door from each room's side of the wall, a wall's width apart
	o.Openings = []Opening{{Room: "hall", To: "kitchen", At: f64(60)}, {Room: "kitchen", To: "hall", At: f64(124)}}
	if _, err := o.pieces(); err != nil {
		t.Errorf("two doors in a wall: %v", err)
	}
}

// One undo step: a layout's pieces, its hidden faces caulk, each found back
// under its op and named by its part, a probe in each room, @0 naming them
// all.
func TestALayoutLandsAsOneStepWithItsHiddenFacesCaulked(t *testing.T) {
	_, c, snap := viewing(t, 601)
	tiles := func(m string) (Tile, error) { return Tile{W: 64, H: 64, From: "test"}, nil }
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"layout": {"material": "brick", "floor": "wood", "rooms": [
			{"name": "hall", "min": [1000, 0], "max": [1256, 256], "material": "plaster"},
			{"name": "yard", "min": [1256, 0], "max": [1512, 256], "outdoor": true}],
			"openings": [{"room": "hall", "to": "yard"}]}},
		{"move": {"ids": ["@0"], "by": [0, 0, 8]}}
	]`), "", tiles)
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]int{}
	for _, m := range d.Made {
		parts[m.Part]++
		if m.Op != 0 || m.ID == "" || m.Box.Min[2] < -8 {
			t.Errorf("made %+v: the layout's, moved up 8", m)
		}
	}
	if d.Label != "t7kb: 1 move, 1 layout" || fmt.Sprint(parts) != "map[hall ceiling:1 hall floor:1 hall probe:1 hall wall:3 hall|yard wall:4 yard floor:1 yard probe:1 yard wall:3]" {
		t.Fatalf("done %q, parts %v", d.Label, parts)
	}
	var probes []string
	materials := map[string]int{}
	for _, e := range d.After.Map.Entities {
		for _, m := range d.Made {
			if m.Class == "reflection_probe" && hexOf(e.Guid()) == hexOf(m.ID) {
				origin, _ := e.Key("origin")
				radius, _ := e.Key("radius")
				probes = append(probes, m.Part+" "+origin+" r"+radius)
			}
		}
		for _, br := range e.Brushes() {
			for _, m := range d.Made {
				if hexOf(br.Guid()) == hexOf(m.ID) {
					for _, f := range faceTex(t, br) {
						materials[f.Material]++
					}
				}
			}
		}
	}
	sort.Strings(probes)
	if fmt.Sprint(probes) != "[hall probe 1128 128 64 r192 yard probe 1384 128 64 r192]" {
		t.Errorf("probes %v: each room's middle, moved up 8; reaching its corners", probes)
	}
	if fmt.Sprint(materials) != "map[brick:38 caulk:28 plaster:6 wood:6]" {
		t.Errorf("faces %v", materials)
	}
	if d.Tiles["caulk"].W != 64 || d.Tiles["wood"].From != "test" {
		t.Errorf("tiles %+v", d.Tiles)
	}
}

// "probes": false leaves the rooms without one, for spaces probes already
// cover.
func TestALayoutWithoutItsProbes(t *testing.T) {
	_, c, snap := viewing(t, 602)
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"layout": {"material": "brick", "probes": false,
		"rooms": [{"name": "hall", "min": [1000, 0], "max": [1256, 256]}]}}]`), "", nil)
	if err != nil || len(d.Made) != 6 {
		t.Fatalf("%v, %d made: floor, ceiling, four walls", err, len(d.Made))
	}
	for _, m := range d.Made {
		if m.Class != "" {
			t.Errorf("made %+v", m)
		}
	}
}

// A layout's op as sent is left as it was: its defaults fill a copy.
func TestALayoutLeavesItsOpAsSent(t *testing.T) {
	o := twoRooms()
	o.Openings = []Opening{{Room: "hall", To: "kitchen"}}
	_, _, snap := viewing(t, 604)
	b := newBatch(snap, nil)
	if err := b.layout(0, &o); err != nil {
		t.Fatal(err)
	}
	if o.Rooms[0].Material != "" || o.Openings[0].Width != 0 || o.Height != 0 {
		t.Errorf("the op was changed: %+v", o)
	}
}

// A label the agent already marked keeps one mark in the undo menu.
func TestAnEditsLabelIsMarkedOnce(t *testing.T) {
	_, c, snap := viewing(t, 603)
	for label, want := range map[string]string{"my room": "t7kb: my room", "t7kb: my room": "t7kb: my room", "t7kb:x": "t7kb: t7kb:x"} {
		d, err := Edit(context.Background(), c, snap, ops(t, `[{"box": {"min": [1000, 0, 0], "max": [1064, 64, 64], "material": "brick"}}]`), label, nil)
		if err != nil || d.Label != want {
			t.Errorf("%q: %q, %v, want %q", label, d.Label, err, want)
		}
	}
}
