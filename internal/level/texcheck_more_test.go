package level

import (
	"fmt"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// prism is a brush of the faces given, each "p0 p1 p2" in Radiant's winding,
// of one material at a 64-unit tile.
func prism(material string, faces ...[3]mapfile.Vec3) string {
	var s strings.Builder
	s.WriteString("{\n")
	for _, f := range faces {
		s.WriteString(strings.Replace(face(f[0], f[1], f[2]), " caulk ", " "+material+" ", 1) + "\n")
	}
	return s.String() + "}\n"
}

// wedge is a ramp over x 0..128 at y0..y1, 64 high at x = 0 down to 0 at
// x = 128, offset by o: two triangle faces (its ±y ends), a slope, a floor,
// a back.
func wedge(material string, y0, y1 float64, o mapfile.Vec3) string {
	v := func(x, y, z float64) mapfile.Vec3 { return mapfile.Vec3{x + o[0], y + o[1], z + o[2]} }
	return prism(material,
		[3]mapfile.Vec3{v(0, 0, 0), v(1, 0, 0), v(0, 1, 0)},     // bottom
		[3]mapfile.Vec3{v(0, 0, 0), v(0, 1, 0), v(0, 0, 1)},     // -x
		[3]mapfile.Vec3{v(0, y0, 0), v(0, y0, 1), v(1, y0, 0)},  // -y
		[3]mapfile.Vec3{v(0, y1, 0), v(1, y1, 0), v(0, y1, 1)},  // +y
		[3]mapfile.Vec3{v(0, 0, 64), v(0, 1, 64), v(128, 0, 0)}, // the slope
	)
}

func TestTrianglesAndSlopesAreCheckedToo(t *testing.T) {
	// A ramp whose triangle end sits against a wall: that end is hidden.
	ramp := wedge("brick", 0, 64, mapfile.Vec3{100, 50, 20})
	wall := slab(mapfile.Vec3{50, 114, 0}, mapfile.Vec3{300, 130, 200}, "brick", plain)
	got := check(t, world(t, []string{ramp, wall}))
	if len(got) != 1 || got[0] != "hidden 0d000000 +y: against 0d000001 -y" {
		t.Errorf("findings %q: the ramp's triangle end against the wall", got)
	}
	// Two ramps make a box: each slope hides the other's, slanted as they are.
	up := wedge("brick", 0, 64, mapfile.Vec3{10, 20, 30})
	down := prism("wood",
		[3]mapfile.Vec3{{10, 20, 94}, {10, 21, 94}, {138, 20, 94}},   // its top at z 94
		[3]mapfile.Vec3{{138, 20, 30}, {138, 20, 31}, {138, 21, 30}}, // +x
		[3]mapfile.Vec3{{10, 20, 30}, {10, 20, 31}, {11, 20, 30}},    // -y
		[3]mapfile.Vec3{{10, 84, 30}, {11, 84, 30}, {10, 84, 31}},    // +y
		[3]mapfile.Vec3{{10, 20, 94}, {138, 20, 30}, {10, 21, 94}},   // its slope, the other's turned over
	)
	found, err := CheckTextures(world(t, []string{up, down}), nil, func(m string) MaterialInfo { return checkInfo[m] })
	if err != nil {
		t.Fatal(err)
	}
	slopes := 0
	for _, f := range found {
		if f.Kind == "hidden" && strings.HasPrefix(f.Detail, "against ") {
			slopes++
		}
	}
	if slopes != 2 {
		t.Errorf("findings %+v: the two slopes against each other", found)
	}
}

func TestWhatIsAndIsntBuried(t *testing.T) {
	big := slab(mapfile.Vec3{100, 100, 100}, mapfile.Vec3{356, 356, 356}, "brick", plain)
	// Flush with the big one's +x face from inside: that face is on the big
	// one's surface, not in it, and shows; the others are buried.
	flush := slab(mapfile.Vec3{300, 200, 200}, mapfile.Vec3{356, 232, 232}, "wood", "128 32 0 0 0 0")
	got := check(t, world(t, []string{big, flush}))
	if len(got) != 5 || strings.Contains(strings.Join(got, "\n"), "0d000001 +x") {
		t.Errorf("findings %q: five buried faces, not the flush one", got)
	}
	// A tetrahedron of four faces buries what it holds.
	tet := prism("brick",
		[3]mapfile.Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}},
		[3]mapfile.Vec3{{0, 0, 0}, {0, 1, 0}, {0, 0, 1}},
		[3]mapfile.Vec3{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}},
		[3]mapfile.Vec3{{300, 0, 0}, {0, 0, 300}, {0, 300, 0}},
	)
	small := slab(mapfile.Vec3{20, 20, 20}, mapfile.Vec3{40, 40, 40}, "wood", "128 32 0 0 0 0")
	if got := check(t, world(t, []string{tet, small})); len(got) != 6 {
		t.Errorf("in a tetrahedron: %q", got)
	}
}

func TestSeamsAndStretchAtTheirEdges(t *testing.T) {
	a := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{128, 8, 112}, "brick", "64 64 16 8 90 0")
	seams := func(b string) int {
		n := 0
		for _, g := range check(t, world(t, []string{a, b})) {
			if strings.HasPrefix(g, "seam") {
				n++
			}
		}
		return n
	}
	for name, c := range map[string]struct {
		b    string
		want int
	}{
		"the same offsets and turn": {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 16 8 90 0"), 0},
		"turned the other way":      {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 16 8 -90 0"), 4},
		"a turn further":            {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 16 8 450 0"), 0},
		"a hair larger":             {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64.0004 64 16 8 90 0"), 0},
		"a plane 4 off":             {slab(mapfile.Vec3{128, 4, 0}, mapfile.Vec3{256, 12, 112}, "brick", "64 64 0 0 0 0"), 2},
	} {
		if got := seams(c.b); got != c.want {
			t.Errorf("%s: %d seams, want %d", name, got, c.want)
		}
	}
	for tex, want := range map[string]bool{"96 64 0 0 0 0": false, "97 64 0 0 0 0": true, "64 96 0 0 0 0": false, "64 97 0 0 0 0": true} {
		got := check(t, world(t, []string{slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}, "brick", tex)}))
		if (len(got) > 0) != want {
			t.Errorf("a tile %s of a 64x64 material: %q", tex, got)
		}
	}
	// A material whose tile isn't known is never stretched.
	checkInfo["untiled"] = MaterialInfo{Known: true, Opaque: true, Drawn: true}
	defer delete(checkInfo, "untiled")
	if got := check(t, world(t, []string{slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}, "untiled", "64 4 0 0 0 0")})); len(got) != 0 {
		t.Errorf("an untiled material: %q", got)
	}
}

func TestHullsOfRepeatedAndInLinePoints(t *testing.T) {
	want := "[{0 0} {4 0} {4 2} {0 2}]"
	for _, pts := range [][]pt2{
		{{0, 0}, {2, 0}, {4, 0}, {4, 1}, {4, 2}, {2, 2}, {0, 2}, {0, 1}, {2, 1}},
		{{4, 2}, {0, 0}, {0, 2}, {4, 0}, {0, 0}, {4, 2}, {2, 0}, {0, 1}},
		{{0, 2}, {0, 1}, {0, 0}, {4, 2}, {4, 1}, {4, 0}},
	} {
		if got := fmt.Sprint(convexHull(pts)); got != want {
			t.Errorf("hull of %v = %s, want %s", pts, got, want)
		}
	}
}

func TestARoomsRefusalsSayTheRightSizes(t *testing.T) {
	o := RoomOp{Min: [3]float64{100, 200, 16}, Max: [3]float64{356, 584, 128}, Material: "brick", Holes: []HoleOp{{Side: "+x", At: 370}}}
	if _, err := roomPieces(&o); err == nil || !strings.Contains(err.Error(), "runs past its wall's 384 of inside") {
		t.Errorf("a hole past the end: %v", err)
	}
	o.Holes = []HoleOp{{Side: "-y", At: 100, Sill: 32}}
	if _, err := roomPieces(&o); err == nil || !strings.Contains(err.Error(), "taller than the room's 112") {
		t.Errorf("a hole too tall: %v", err)
	}
	// Holes a unit apart leave a unit of wall; touching ones are one hole.
	o.Holes = []HoleOp{{Side: "-y", At: 40, Width: 40}, {Side: "-y", At: 81, Width: 40}}
	ps, err := roomPieces(&o)
	if err != nil {
		t.Fatalf("holes a unit apart: %v", err)
	}
	thin := 0
	for _, p := range ps {
		if p.hi[0]-p.lo[0] == 1 && p.hi[2]-p.lo[2] == 112 {
			thin++
		}
	}
	if thin != 1 {
		t.Errorf("%d strips a unit wide between the holes", thin)
	}
	o.Holes = []HoleOp{{Side: "-y", At: 40, Width: 40}, {Side: "-y", At: 80, Width: 40}}
	if _, err := roomPieces(&o); err == nil || !strings.Contains(err.Error(), "meet") {
		t.Errorf("touching holes: %v", err)
	}
	// A door at the start of a wall off the origin leaves no sliver before it.
	o.Holes = []HoleOp{{Side: "-y", At: 28}}
	ps, _ = roomPieces(&o)
	for _, p := range ps {
		if p.hi[0]-p.lo[0] < 1 || p.hi[1]-p.lo[1] < 1 || p.hi[2]-p.lo[2] < 1 {
			t.Errorf("a sliver %v %v", p.lo, p.hi)
		}
	}
}
