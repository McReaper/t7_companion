package level

import (
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

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
	if !geom.Overlapping(rect(0, 0, 10, 10), rect(5, 5, 20, 20)) || geom.Overlapping(rect(0, 0, 10, 10), rect(10, 0, 20, 10)) || geom.Overlapping(rect(0, 0, 10, 10), rect(10.005, 0, 20, 10)) {
		t.Error("hulls overlap by an area, not an edge")
	}
}

// rect is a rectangle as a hull.
func rect(x0, y0, x1, y1 float64) []geom.P2 {
	return []geom.P2{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}}
}
