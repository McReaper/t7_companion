package geom

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

func box(min, max mapfile.Vec3) Box { return Box{min, max, true} }

func TestBrushBoxesOfWhatRadiantWrote(t *testing.T) {
	m := testMap(t)
	world, sun := m.Entities[0].Brushes(), m.Entities[2].Brushes()
	for name, tc := range map[string]struct {
		b    *mapfile.Brush
		want Box
	}{
		"floor":      {world[0], box(mapfile.Vec3{-216, -232, -64}, mapfile.Vec3{248, 256, -16})},
		"arch curve": {world[1], box(mapfile.Vec3{-96, -64, -16}, mapfile.Vec3{96, 64, 200})},
		"sun volume": {sun[0], box(mapfile.Vec3{-272, -264, -144}, mapfile.Vec3{296, 320, 176})},
	} {
		if got := BrushBox(tc.b); got != tc.want {
			t.Errorf("%s: box = %+v, want %+v", name, got, tc.want)
		}
	}
	probe := EntityBox(m.Entities[1])
	if probe != box(mapfile.Vec3{44, 108, 4}, mapfile.Vec3{44, 108, 4}) {
		t.Errorf("a point entity's box is its origin: %+v", probe)
	}
	if b := EntityBox(m.Entities[0]); b != box(mapfile.Vec3{-216, -232, -64}, mapfile.Vec3{248, 256, 200}) {
		t.Errorf("worldspawn's box holds its brushes: %+v", b)
	}
}

// face writes a face line through three points.
func face(p0, p1, p2 mapfile.Vec3) string {
	return fmt.Sprintf(" ( %s ) ( %s ) ( %s ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0", fmtVec(p0), fmtVec(p1), fmtVec(p2))
}

func fmtVec(v mapfile.Vec3) string {
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
	return f(v[0]) + " " + f(v[1]) + " " + f(v[2])
}

func brushOf(t *testing.T, faces ...string) *mapfile.Brush {
	t.Helper()
	text := "{\n\"classname\" \"worldspawn\"\n{\n guid \"{00000000-0000-0000-0000-000000000001}\"\n" + strings.Join(faces, "\n") + "\n}\n}\n"
	m, err := mapfile.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return m.Entities[0].Brushes()[0]
}

// A box [0,64]³ with its +x+y vertical edge cut off by x+y ≤ 96: ten
// vertices, and the cut's two edges where no face's points are.
func TestACutBoxHasTheVerticesWhereItsPlanesMeet(t *testing.T) {
	b := brushOf(t,
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),    // z ≥ 0
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}), // z ≤ 64
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}),    // x ≥ 0
		face(mapfile.Vec3{64, 0, 0}, mapfile.Vec3{64, 0, 1}, mapfile.Vec3{64, 1, 0}), // x ≤ 64
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}),    // y ≥ 0
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}), // y ≤ 64
		face(mapfile.Vec3{96, 0, 0}, mapfile.Vec3{96, 0, 1}, mapfile.Vec3{0, 96, 0})) // x + y ≤ 96
	faces, err := b.Faces()
	if err != nil {
		t.Fatal(err)
	}
	var planes []Plane
	for _, f := range faces {
		planes = append(planes, PlaneOf(f.Points))
	}
	var got []string
	for _, v := range Vertices(planes) {
		got = append(got, fmtVec(mapfile.Vec3{math.Round(v[0]) + 0, math.Round(v[1]) + 0, math.Round(v[2]) + 0}))
	}
	slices.Sort(got)
	got = slices.Compact(got)
	want := []string{"0 0 0", "0 0 64", "0 64 0", "0 64 64", "32 64 0", "32 64 64", "64 0 0", "64 0 64", "64 32 0", "64 32 64"}
	if !slices.Equal(got, want) {
		t.Errorf("vertices = %q\nwant %q", got, want)
	}
	if bb := BrushBox(b); bb != box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}) {
		t.Errorf("box = %+v", bb)
	}
}

func TestABrushThatEnclosesNothingHasNoBox(t *testing.T) {
	open := brushOf(t, // two parallel faces only
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}))
	if b := BrushBox(open); b.Ok {
		t.Errorf("box = %+v", b)
	}
	inverted := brushOf(t, // the faces wound outward: their half-spaces share no point
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{1, 0, 0}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{1, 0, 64}, mapfile.Vec3{0, 1, 64}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{64, 0, 0}, mapfile.Vec3{64, 1, 0}, mapfile.Vec3{64, 0, 1}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 0, 1}),
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{0, 64, 1}, mapfile.Vec3{1, 64, 0}))
	if b := BrushBox(inverted); b.Ok {
		t.Errorf("an inside-out brush: box = %+v", b)
	}
}

func TestBoxDistanceAndCorners(t *testing.T) {
	b := box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{10, 10, 10})
	for p, want := range map[mapfile.Vec3]float64{{5, 5, 5}: 0, {13, 5, 5}: 3, {13, 14, 5}: 5, {-3, -4, 10}: 5} {
		if got := b.Distance(p); math.Abs(got-want) > 1e-9 {
			t.Errorf("Distance(%v) = %v, want %v", p, got, want)
		}
	}
	if c := b.Corners(); len(c) != 8 || c[0] != (mapfile.Vec3{0, 0, 0}) || c[7] != (mapfile.Vec3{10, 10, 10}) || c[5] != (mapfile.Vec3{10, 0, 10}) {
		t.Errorf("corners = %v", c)
	}
	if b.Centre() != (mapfile.Vec3{5, 5, 5}) {
		t.Errorf("centre = %v", b.Centre())
	}
	var empty Box
	empty.Union(Box{})
	if empty.Ok {
		t.Error("the union of nothing holds something")
	}
}

func TestParseVec3(t *testing.T) {
	if v, ok := ParseVec3(" 1 -2.5 3e2 "); !ok || v != (mapfile.Vec3{1, -2.5, 300}) {
		t.Errorf("ParseVec3 = %v, %v", v, ok)
	}
	for _, s := range []string{"1 2", "1 2 x", "1 2 3 4", ""} {
		if _, ok := ParseVec3(s); ok {
			t.Errorf("ParseVec3(%q) is ok", s)
		}
	}
}

func TestABrushWithADegenerateFaceStillHasItsBox(t *testing.T) {
	b := brushOf(t,
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}),
		face(mapfile.Vec3{64, 0, 0}, mapfile.Vec3{64, 0, 1}, mapfile.Vec3{64, 1, 0}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}),
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}),
		face(mapfile.Vec3{1, 1, 1}, mapfile.Vec3{2, 2, 2}, mapfile.Vec3{3, 3, 3})) // three points on a line: no plane
	if bb := BrushBox(b); bb != box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}) {
		t.Errorf("box = %+v", bb)
	}
}

// A box [0,64]³ cut by x + z ≤ 96: a plane leaning in z.
func TestACutThatLeansInZ(t *testing.T) {
	b := brushOf(t,
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}),
		face(mapfile.Vec3{64, 0, 0}, mapfile.Vec3{64, 0, 1}, mapfile.Vec3{64, 1, 0}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}),
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}),
		face(mapfile.Vec3{96, 0, 0}, mapfile.Vec3{32, 0, 64}, mapfile.Vec3{96, 1, 0})) // x + z ≤ 96
	faces, _ := b.Faces()
	var planes []Plane
	for _, f := range faces {
		planes = append(planes, PlaneOf(f.Points))
	}
	var got []string
	for _, v := range Vertices(planes) {
		got = append(got, fmtVec(mapfile.Vec3{math.Round(v[0]) + 0, math.Round(v[1]) + 0, math.Round(v[2]) + 0}))
	}
	slices.Sort(got)
	got = slices.Compact(got)
	want := []string{"0 0 0", "0 0 64", "0 64 0", "0 64 64", "32 0 64", "32 64 64", "64 0 0", "64 0 32", "64 64 0", "64 64 32"}
	if !slices.Equal(got, want) {
		t.Errorf("vertices %q\nwant %q", got, want)
	}
}

// testMap is a small map Radiant wrote: a worldspawn with a floor brush and an
// arch curve, a volume_sun and three reflection probes.
func testMap(t *testing.T) *mapfile.Map {
	t.Helper()
	b, err := os.ReadFile("../testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mapfile.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
