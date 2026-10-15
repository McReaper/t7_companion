package level

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// boxes are a room op's pieces as boxes, sorted.
func roomBoxes(t *testing.T, o RoomOp) []string {
	t.Helper()
	ps, err := roomPieces(&o)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range ps {
		out = append(out, fmtVec(p.lo)+" "+fmtVec(p.hi)+" "+p.material)
	}
	sort.Strings(out)
	return out
}

func TestARoomIsAFloorACeilingAndFourWallsAroundItsInside(t *testing.T) {
	got := roomBoxes(t, RoomOp{Min: [3]float64{0, 0, 0}, Max: [3]float64{512, 384, 112}, Material: "brick", Floor: "wood"})
	want := []string{
		"-8 -8 -16 520 392 0 wood",    // the floor, under the walls too
		"-8 -8 0 0 392 112 brick",     // -x, the corners in
		"-8 -8 112 520 392 128 brick", // the ceiling, the walls' material
		"0 -8 0 512 0 112 brick",      // -y, between the x walls
		"0 384 0 512 392 112 brick",   // +y
		"512 -8 0 520 392 112 brick",  // +x
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("pieces:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A corridor: open at both ends, no ceiling, thicker walls.
	got = roomBoxes(t, RoomOp{Min: [3]float64{0, 0, 0}, Max: [3]float64{512, 128, 96}, Material: "brick", Open: []string{"-x", "+x", "top"}, Wall: 16, Slab: 8})
	want = []string{"0 -16 -8 512 144 0 brick", "0 -16 0 512 0 96 brick", "0 128 0 512 144 96 brick"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("corridor:\n%s", strings.Join(got, "\n"))
	}
}

func TestARoomsWallsAreBuiltAroundItsHoles(t *testing.T) {
	got := roomBoxes(t, RoomOp{Min: [3]float64{0, 0, 0}, Max: [3]float64{512, 384, 112}, Material: "brick", Open: []string{"top", "bottom", "+x", "-x", "+y"},
		Holes: []HoleOp{{Side: "-y", At: 400, Width: 64, Sill: 32, Height: 48}, {Side: "-y", At: 256}}})
	want := []string{
		"0 -8 0 228 0 112 brick",    // up to the door
		"228 -8 96 284 0 112 brick", // over the door: 56 wide, 96 high from the floor
		"284 -8 0 368 0 112 brick",  // between the door and the window
		"368 -8 0 432 0 32 brick",   // under the window, its sill at 32
		"368 -8 80 432 0 112 brick", // over it
		"432 -8 0 512 0 112 brick",  // the rest
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("pieces:\n%s", strings.Join(got, "\n"))
	}
	// A hole up to the ceiling leaves no lintel; one at the wall's end, no strip.
	got = roomBoxes(t, RoomOp{Min: [3]float64{0, 0, 0}, Max: [3]float64{128, 128, 96}, Material: "brick", Open: []string{"top", "bottom", "-x", "+x", "-y"},
		Holes: []HoleOp{{Side: "+y", At: 28}}})
	if strings.Join(got, "\n") != "56 128 0 128 136 96 brick" {
		t.Errorf("a door 96 high in a room 96 high, at the wall's start:\n%s", strings.Join(got, "\n"))
	}
}

func TestARoomRefusesWhatItCantBuild(t *testing.T) {
	base := func() RoomOp {
		return RoomOp{Min: [3]float64{0, 0, 0}, Max: [3]float64{256, 256, 112}, Material: "brick"}
	}
	for want, edit := range map[string]func(*RoomOp){
		"must exceed min":               func(o *RoomOp) { o.Max[2] = 0 },
		`material "two words"`:          func(o *RoomOp) { o.Floor = "two words" },
		`material ""`:                   func(o *RoomOp) { o.Material = "" },
		"thicknesses above 0":           func(o *RoomOp) { o.Wall = -8 },
		`open "front"`:                  func(o *RoomOp) { o.Open = []string{"front"} },
		`side "up"`:                     func(o *RoomOp) { o.Holes = []HoleOp{{Side: "up", At: 64}} },
		"the -y side is open":           func(o *RoomOp) { o.Open = []string{"-y"}; o.Holes = []HoleOp{{Side: "-y", At: 64}} },
		"can't be below 0":              func(o *RoomOp) { o.Holes = []HoleOp{{Side: "-y", At: 64, Sill: -1}} },
		"runs past its wall's 256":      func(o *RoomOp) { o.Holes = []HoleOp{{Side: "+x", At: 240}} },
		"runs past":                     func(o *RoomOp) { o.Holes = []HoleOp{{Side: "+x", At: 10}} },
		"taller than the room's 112":    func(o *RoomOp) { o.Holes = []HoleOp{{Side: "-x", At: 64, Sill: 32}} },
		"two holes in the +y wall meet": func(o *RoomOp) { o.Holes = []HoleOp{{Side: "+y", At: 64}, {Side: "+y", At: 110}} },
	} {
		o := base()
		edit(&o)
		if _, err := roomPieces(&o); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
	o := base()
	o.Holes = []HoleOp{{Side: "+y", At: 28}, {Side: "+y", At: 228}} // at both ends, a wall's width apart
	if _, err := roomPieces(&o); err != nil {
		t.Errorf("holes at the wall's two ends: %v", err)
	}
}

// One undo step: a room with a door, its hidden faces caulk, every piece
// found back under the room's op, and @0 naming them all.
func TestARoomOpLandsAsOneStepWithItsHiddenFacesCaulked(t *testing.T) {
	_, c, snap := viewing(t, 601)
	tiles := func(m string) (Tile, error) { return Tile{W: 64, H: 64, From: "test"}, nil }
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"room": {"min": [1000, 0, 0], "max": [1512, 384, 112], "material": "brick", "floor": "wood", "holes": [{"side": "-y", "at": 256}]}},
		{"move": {"ids": ["@0"], "by": [0, 0, 8]}}
	]`), "", tiles)
	if err != nil {
		t.Fatal(err)
	}
	if d.Label != "t7kb: 1 move, 1 room" || len(d.Made) != 8 {
		t.Fatalf("done %q, %d made: floor, ceiling, three walls and the door's three pieces", d.Label, len(d.Made))
	}
	caulked, faces := 0, 0
	for _, m := range d.Made {
		if m.Op != 0 || m.ID == "" || m.Box.Min[2] < -8 {
			t.Errorf("made %+v: the room's, moved up 8", m)
		}
		for _, e := range d.After.Map.Entities {
			for _, br := range e.Brushes() {
				if hexOf(br.Guid()) != hexOf(m.ID) {
					continue
				}
				for _, f := range faceTex(t, br) {
					faces++
					if f.Material == "caulk" {
						caulked++
					}
				}
			}
		}
	}
	// what the pieces hide of each other: the x walls' tops and bottoms (4);
	// the +y wall's, and its two ends against the x walls (4); each of the
	// door's sides, its top, bottom and its end against an x wall (6); the
	// lintel's top and its two ends against the door's sides (3). The lintel's
	// underside and the sides' faces at the door show.
	if faces != 48 || caulked != 17 {
		t.Errorf("%d faces, %d caulk", faces, caulked)
	}
	if d.Tiles["caulk"].W != 64 || d.Tiles["wood"].From != "test" {
		t.Errorf("tiles %+v", d.Tiles)
	}
}
