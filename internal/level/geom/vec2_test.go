package geom

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

func near2(a, b P2) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

func TestVectorsInThePlane(t *testing.T) {
	a, b := P2{X: 1, Y: 2}, P2{X: 4, Y: 6}
	if got := Sub2(b, a); got != (P2{X: 3, Y: 4}) {
		t.Errorf("Sub2 = %v", got)
	}
	if got := CrossV(P2{X: 1, Y: 0}, P2{X: 0, Y: 1}); got != 1 {
		t.Errorf("CrossV, a left turn = %v, want 1", got)
	}
	if got := CrossV(P2{X: 2, Y: 3}, P2{X: 5, Y: 7}); got != -1 {
		t.Errorf("CrossV = %v, want 2*7-3*5", got)
	}
	if got := Dist2(a, b); got != 5 {
		t.Errorf("Dist2 = %v", got)
	}
	if got := Unit2(a, b); !near2(got, P2{X: 0.6, Y: 0.8}) {
		t.Errorf("Unit2 = %v", got)
	}
	if got := Along(a, P2{X: 0.6, Y: 0.8}, 10); !near2(got, P2{X: 7, Y: 10}) {
		t.Errorf("Along = %v", got)
	}
	u := P2{X: 1, Y: 0}
	if got := Aside(a, u, 4, 1); got != (P2{X: 1, Y: 6}) {
		t.Errorf("Aside left = %v", got)
	}
	if got := Aside(a, u, 4, -1); got != (P2{X: 1, Y: -2}) {
		t.Errorf("Aside right = %v", got)
	}
}

func TestLinesMeetAndSegmentsCross(t *testing.T) {
	if p, ok := MeetLines(P2{X: 0, Y: 0}, P2{X: 1, Y: 0}, P2{X: 3, Y: -2}, P2{X: 0, Y: 1}); !ok || !near2(p, P2{X: 3, Y: 0}) {
		t.Errorf("MeetLines = %v %v, want 3 0", p, ok)
	}
	if _, ok := MeetLines(P2{X: 0, Y: 0}, P2{X: 1, Y: 1}, P2{X: 0, Y: 5}, P2{X: 2, Y: 2}); ok {
		t.Error("parallel lines met")
	}
	a, c := P2{X: 0, Y: 0}, P2{X: 10, Y: 0}
	for _, k := range []struct {
		p, q P2
		ok   bool
		at   P2
	}{
		{P2{X: 5, Y: -5}, P2{X: 5, Y: 5}, true, P2{X: 5, Y: 0}},
		{P2{X: 0, Y: -5}, P2{X: 0, Y: 5}, false, P2{}},   // at a, the first's end
		{P2{X: 10, Y: -5}, P2{X: 10, Y: 5}, false, P2{}}, // at c, its other end
		{P2{X: 5, Y: 0}, P2{X: 5, Y: 5}, false, P2{}},    // p on the first
		{P2{X: 5, Y: -5}, P2{X: 5, Y: 0}, false, P2{}},   // q on the first
		{P2{X: 0, Y: 1}, P2{X: 10, Y: 1}, false, P2{}},   // parallel
		{P2{X: 5, Y: 1}, P2{X: 5, Y: 5}, false, P2{}},    // short of the first
	} {
		if at, ok := ProperCrossing(a, c, k.p, k.q); ok != k.ok || !near2(at, k.at) {
			t.Errorf("ProperCrossing(%v-%v, %v-%v) = %v %v, want %v %v", a, c, k.p, k.q, at, ok, k.at, k.ok)
		}
	}
}

func TestAVerticalLineThroughABrushAndATriangle(t *testing.T) {
	b := Box{Min: mapfile.Vec3{0, 0, -16}, Max: mapfile.Vec3{64, 64, 8}, Ok: true}
	var planes []Plane // the box's six faces, normals inward
	for axis := range 3 {
		var lo, hi mapfile.Vec3
		lo[axis], hi[axis] = 1, -1
		planes = append(planes, Plane{N: lo, D: b.Min[axis]}, Plane{N: hi, D: -b.Max[axis]})
	}
	if lo, hi, ok := Column(planes, 32, 32); !ok || lo != -16 || hi != 8 {
		t.Errorf("Column inside = %v %v %v", lo, hi, ok)
	}
	if _, _, ok := Column(planes, 80, 32); ok {
		t.Error("a line beside the brush crossed it")
	}
	if _, _, ok := Column(planes, 64.005, 32); !ok {
		t.Error("a line on the face, within Eps, missed it")
	}
	thin := append(append([]Plane{}, planes[:4]...), Plane{N: mapfile.Vec3{0, 0, 1}, D: 0}, Plane{N: mapfile.Vec3{0, 0, -1}, D: -0.005})
	if _, _, ok := Column(thin, 32, 32); ok {
		t.Error("a brush thinner than Eps has a stretch")
	}
	tri := [3]mapfile.Vec3{{0, 0, 0}, {64, 0, 64}, {0, 64, 0}}
	if z, ok := OnTriangle(tri, 32, 16); !ok || math.Abs(z-32) > 1e-9 {
		t.Errorf("OnTriangle on the slope = %v %v, want 32", z, ok)
	}
	if z, ok := OnTriangle(tri, 0, 0); !ok || z != 0 {
		t.Errorf("OnTriangle at a corner = %v %v", z, ok)
	}
	for _, p := range [][2]float64{{40, 40}, {-1, 10}, {10, -1}} {
		if _, ok := OnTriangle(tri, p[0], p[1]); ok {
			t.Errorf("OnTriangle at %v, beside it", p)
		}
	}
	if _, ok := OnTriangle([3]mapfile.Vec3{{0, 0, 0}, {10, 0, 5}, {20, 0, 9}}, 5, 0); ok {
		t.Error("a triangle seen edge-on has a height")
	}
}

func TestPlanesKeysAxesAndFlattening(t *testing.T) {
	p := Plane{N: mapfile.Vec3{0.70710678, 0.70710678, 0}, D: 12.34}
	if got := KeyOf(p); got != (PlaneKey{707, 707, 0, 123}) {
		t.Errorf("KeyOf = %v", got)
	}
	if KeyOf(p) != KeyOf(Plane{N: mapfile.Vec3{0.70710679, 0.70710677, 0}, D: 12.3401}) {
		t.Error("planes within rounding have different keys")
	}
	for n, want := range map[mapfile.Vec3]int{{0, 0, 1}: 2, {0.5, 0.5, 0.5}: 2, {0.9, 0.1, 0.1}: 0, {0.5, 0.5, 0.1}: 0, {0.1, -0.9, 0.2}: 1} {
		if got := Dominant(n); got != want {
			t.Errorf("Dominant(%v) = %d, want %d", n, got, want)
		}
	}
	pts := []mapfile.Vec3{{1, 2, 3}}
	for axis, want := range []P2{{X: 2, Y: 3}, {X: 3, Y: 1}, {X: 1, Y: 2}} {
		if got := Flatten(pts, axis); len(got) != 1 || got[0] != want {
			t.Errorf("Flatten along %d = %v, want %v", axis, got, want)
		}
	}
}

func TestGrownAndOverlapping(t *testing.T) {
	b := Box{Min: mapfile.Vec3{0, 0, 0}, Max: mapfile.Vec3{100, 10, 300}, Ok: true}
	if g := b.Grown(256); g.Min != (mapfile.Vec3{-78, -123, 0}) || g.Max != (mapfile.Vec3{178, 133, 300}) {
		t.Errorf("Grown = %v %v", g.Min, g.Max)
	}
	square := func(x0, y0, s float64) []P2 {
		return []P2{{X: x0, Y: y0}, {X: x0 + s, Y: y0}, {X: x0 + s, Y: y0 + s}, {X: x0, Y: y0 + s}}
	}
	for _, c := range []struct {
		b    []P2
		want bool
	}{
		{square(5, 5, 10), true}, {square(10, 0, 10), false}, {square(10, 10, 5), false}, {square(9.999, 0, 10), false}, {square(9, 0, 10), true},
	} {
		if got := Overlapping(square(0, 0, 10), c.b); got != c.want {
			t.Errorf("Overlapping with %v = %v, want %v", c.b, got, c.want)
		}
	}
}
