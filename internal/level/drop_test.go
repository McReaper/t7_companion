package level

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// groundOfText is the ground of a worldspawn holding the brushes.
func groundOfText(t *testing.T, brushes ...string) *ground {
	t.Helper()
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, brushes...)))
	if err != nil {
		t.Fatal(err)
	}
	g := &ground{}
	for _, br := range m.Entities[0].Brushes() {
		g.add(br)
	}
	return g
}

func TestTheFloorIsTheHighestTopUnderAPointWithRoomAbove(t *testing.T) {
	floor := slab(mapfile.Vec3{0, 0, -16}, mapfile.Vec3{512, 512, 0}, "brick", plain)
	step := slab(mapfile.Vec3{256, 0, 0}, mapfile.Vec3{512, 512, 16}, "brick", plain)
	wall := slab(mapfile.Vec3{100, 0, 0}, mapfile.Vec3{108, 512, 112}, "brick", plain)
	low := slab(mapfile.Vec3{0, 400, 48}, mapfile.Vec3{100, 512, 64}, "brick", plain) // a shelf too low to stand under
	sky := slab(mapfile.Vec3{0, 0, -64}, mapfile.Vec3{512, 512, -32}, "sky", plain)
	g := groundOfText(t, floor, step, wall, low, sky)
	for _, c := range []struct {
		x, y, lo, hi float64
		want         float64
		ok           bool
	}{
		{50, 50, -18, 18, 0, true},   // the floor
		{300, 50, -18, 18, 16, true}, // the step, a stair's height up
		{300, 50, -18, 8, 0, false},  // under the step's top: inside it
		{104, 50, -18, 18, 0, false}, // in the wall
		{50, 450, -18, 18, 0, false}, // under the shelf
		{120, 50, -18, 18, 0, false}, // beside the wall, its side within half a body
		{50, 50, -60, -20, 0, false}, // the sky's floor is no ground
		{600, 50, -18, 18, 0, false}, // off the map
	} {
		got, ok := g.floor(c.x, c.y, c.lo, c.hi)
		if ok != c.ok || (ok && math.Abs(got-c.want) > eps) {
			t.Errorf("at %v,%v from %v to %v: %v %v, want %v %v", c.x, c.y, c.hi, c.lo, got, ok, c.want, c.ok)
		}
	}
}

// A ramp's top is where a vertical line meets its slope; terrain is the
// patch's triangles.
func TestTheFloorFollowsARampAndTerrain(t *testing.T) {
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil,
		prismBrush([]pt2{{0, 0}, {256, 0}, {256, 128}, {0, 128}}, -16, 0, "brick", texNumbers(64, 64)))))
	if err != nil {
		t.Fatal(err)
	}
	br := m.Entities[0].Brushes()[0]
	faces, _ := br.Faces()
	planes := make([]plane, len(faces))
	for i, f := range faces {
		planes[i] = planeOf(f.Points)
	}
	planes[1] = plane{n: mapfile.Vec3{0.25 / math.Hypot(1, 0.25), 0, -1 / math.Hypot(1, 0.25)}} // the top tilted: z ≤ 0.25·x
	if lo, hi, ok := column(planes, 128, 64); !ok || math.Abs(hi-32) > 1e-6 || math.Abs(lo+16) > 1e-6 {
		t.Errorf("the ramp at x 128: %v to %v, %v", lo, hi, ok)
	}
	if _, _, ok := column(planes, 300, 64); ok {
		t.Error("beside the ramp: the line misses it")
	}
	g := &ground{patches: [][3]mapfile.Vec3{{{0, 0, 0}, {100, 0, 20}, {0, 100, 40}}}}
	if z, ok := g.floor(25, 25, 0, 30); !ok || math.Abs(z-15) > 1e-6 {
		t.Errorf("on the terrain: %v, %v", z, ok)
	}
	if _, ok := g.floor(90, 90, 0, 100); ok {
		t.Error("beside the terrain's triangle")
	}
}

// A mesh patch read from its text is ground: terrain, its control grid in
// triangles.
func TestTerrainFromAPatchIsGround(t *testing.T) {
	mesh := "{\n mesh\n {\n toolFlags;\n  t7_dirt\n  lightmap_gray\n  2 2 16 8\n  (\n" +
		"\tv 0 0 0 t 0 0 0 0\n\tv 0 100 40 t 0 0 0 0\n  )\n  (\n" +
		"\tv 100 0 20 t 0 0 0 0\n\tv 100 100 60 t 0 0 0 0\n  )\n }\n}\n"
	g := groundOfText(t, mesh)
	if len(g.patches) != 2 || len(g.solids) != 0 {
		t.Fatalf("%d triangles, %d solids", len(g.patches), len(g.solids))
	}
	for _, c := range [][3]float64{{25, 25, 15}, {75, 75, 45}, {50, 50, 30}} {
		if z, ok := g.floor(c[0], c[1], c[2]-18, c[2]+18); !ok || math.Abs(z-c[2]) > 1e-6 {
			t.Errorf("at %v,%v: %v %v, want %v", c[0], c[1], z, ok, c[2])
		}
	}
}
