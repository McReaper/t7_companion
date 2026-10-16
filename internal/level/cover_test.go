package level

import (
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

func rect(x0, y0, x1, y1 float64) []pt2 { return []pt2{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} }

func TestHullsCoverTogetherOnlyWhatTheyLeaveNothingOf(t *testing.T) {
	face := rect(0, 0, 100, 50)
	for name, c := range map[string]struct {
		hulls [][]pt2
		want  bool
	}{
		"two halves":                 {[][]pt2{rect(0, 0, 60, 50), rect(60, 0, 100, 50)}, true},
		"two overlapping":            {[][]pt2{rect(-10, -10, 70, 60), rect(40, 0, 100, 50)}, true},
		"three, an L and its corner": {[][]pt2{rect(0, 0, 50, 50), rect(50, 0, 100, 25), rect(50, 25, 100, 50)}, true},
		"a gap between":              {[][]pt2{rect(0, 0, 49, 50), rect(51, 0, 100, 50)}, false},
		"a gap thinner than eps":     {[][]pt2{rect(0, 0, 50, 50), rect(50.005, 0, 100, 50)}, true},
		"a corner left out":          {[][]pt2{rect(0, 0, 100, 40), rect(0, 40, 90, 50)}, false},
		"a triangle's worth short":   {[][]pt2{rect(0, 0, 50, 50), {{50, 0}, {100, 0}, {100, 50}}}, false},
		"none":                       {nil, false},
	} {
		if got := coveredBy(c.hulls, face); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// Far from the origin a gap half a unit wide still shows: a sliver is
	// judged by its own size, not by where it is.
	far := rect(10000, 20000, 10100, 20050)
	if coveredBy([][]pt2{rect(10000, 20000, 10049.75, 20050), rect(10050.25, 20000, 10100, 20050)}, far) {
		t.Error("a gap half a unit wide, far from the origin")
	}
}

func TestSubtractLeavesConvexPiecesOutsideTheHull(t *testing.T) {
	area := func(ps [][]pt2) float64 {
		var a float64
		for _, p := range ps {
			for i := range p {
				q, r := p[i], p[(i+1)%len(p)]
				a += (q.x*r.y - r.x*q.y) / 2
			}
		}
		return a
	}
	// a 100×50 face with a 20×20 hole in its middle taken out: 4600 left
	left := subtract(rect(0, 0, 100, 50), rect(40, 15, 60, 35))
	if a := area(left); a < 4600-eps || a > 4600+eps || len(left) != 4 {
		t.Errorf("%d pieces, %v of area: %v", len(left), a, left)
	}
	if left := subtract(rect(0, 0, 10, 10), rect(-5, -5, 20, 20)); len(left) != 0 {
		t.Errorf("a hull over the whole: %v", left)
	}
	if !sliver(rect(0, 0, 100, 0.01)) || sliver(rect(0, 0, 100, 0.1)) || !sliver([]pt2{{0, 0}, {1, 1}}) {
		t.Error("a sliver is no wider than eps")
	}
}

// A face two brushes cover together is hidden, map_check naming one and
// counting the others.
func TestAFaceTwoBrushesCoverTogetherIsHidden(t *testing.T) {
	wall := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 128, 112}, "brick", plain)
	left := slab(mapfile.Vec3{-64, 128, 0}, mapfile.Vec3{4, 136, 112}, "brick", plain)
	right := slab(mapfile.Vec3{4, 128, 0}, mapfile.Vec3{64, 136, 112}, "brick", plain)
	got := strings.Join(check(t, world(t, []string{wall, left, right}), "0d000000"), "\n")
	if !strings.Contains(got, "hidden 0d000000 +y: against 0d000001 -y and 1 more") {
		t.Errorf("the wall's end against the two:\n%s", got)
	}
	short := slab(mapfile.Vec3{4, 128, 0}, mapfile.Vec3{64, 136, 100}, "brick", plain) // stops 12 short of the top
	if got := strings.Join(check(t, world(t, []string{wall, left, short}), "0d000000"), "\n"); strings.Contains(got, "+y") {
		t.Errorf("a corner of it shows:\n%s", got)
	}
}

// Faces on one plane, facing one way, overlapping, fight when they draw
// otherwise: another material or tiles that don't meet. The same texels
// show nothing, a window on a wall is meant to be there, and touching edges
// aren't an overlap.
func TestOverlappingFacesThatDrawOtherwiseFight(t *testing.T) {
	base := slab(mapfile.Vec3{0, 0, -16}, mapfile.Vec3{256, 256, 0}, "brick", plain)
	for name, c := range map[string]struct {
		other string
		want  bool
	}{
		"wood laid in it":       {slab(mapfile.Vec3{64, 64, -16}, mapfile.Vec3{128, 128, 0}, "wood", "128 32 0 0 0 0"), true},
		"brick, shifted tiles":  {slab(mapfile.Vec3{64, 64, -16}, mapfile.Vec3{128, 128, 0}, "brick", "64 64 8 0 0 0"), true},
		"brick, the same tiles": {slab(mapfile.Vec3{64, 64, -16}, mapfile.Vec3{128, 128, 0}, "brick", plain), false},
		"glass on it":           {slab(mapfile.Vec3{64, 64, -16}, mapfile.Vec3{128, 128, 0}, "glass", plain), false},
		"wood beside it":        {slab(mapfile.Vec3{256, 0, -16}, mapfile.Vec3{320, 256, 0}, "wood", "128 32 0 0 0 0"), false},
	} {
		got := strings.Join(check(t, world(t, []string{base, c.other})), "\n")
		if fight := strings.Contains(got, "zfight 0d000000 +z: 0d000001 +z"); fight != c.want {
			t.Errorf("%s: %v\n%s", name, fight, got)
		}
	}
	// a slab fighting two floors laid in it: both pairs
	two := []string{base, slab(mapfile.Vec3{0, 0, -16}, mapfile.Vec3{64, 64, 0}, "wood", "128 32 0 0 0 0"), slab(mapfile.Vec3{128, 128, -16}, mapfile.Vec3{192, 192, 0}, "wood", "128 32 0 0 0 0")}
	if got := strings.Join(check(t, world(t, two)), "\n"); !strings.Contains(got, "zfight 0d000000 +z: 0d000001 +z") || !strings.Contains(got, "zfight 0d000000 +z: 0d000002 +z") {
		t.Errorf("two floors in a slab:\n%s", got)
	}
	if !overlapping(rect(0, 0, 10, 10), rect(5, 5, 20, 20)) || overlapping(rect(0, 0, 10, 10), rect(10, 0, 20, 10)) || overlapping(rect(0, 0, 10, 10), rect(10.005, 0, 20, 10)) {
		t.Error("hulls overlap by an area, not an edge")
	}
}
