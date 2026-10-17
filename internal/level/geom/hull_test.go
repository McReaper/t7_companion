package geom

import (
	"fmt"
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

func TestHullsOfRepeatedAndInLinePoints(t *testing.T) {
	want := "[{0 0} {4 0} {4 2} {0 2}]"
	for _, pts := range [][]P2{
		{{0, 0}, {2, 0}, {4, 0}, {4, 1}, {4, 2}, {2, 2}, {0, 2}, {0, 1}, {2, 1}},
		{{4, 2}, {0, 0}, {0, 2}, {4, 0}, {0, 0}, {4, 2}, {2, 0}, {0, 1}},
		{{0, 2}, {0, 1}, {0, 0}, {4, 2}, {4, 1}, {4, 0}},
	} {
		if got := fmt.Sprint(ConvexHull(pts)); got != want {
			t.Errorf("hull of %v = %s, want %s", pts, got, want)
		}
	}
}

func TestSideIsAPointsDistanceOffALine(t *testing.T) {
	for _, c := range []struct {
		p, q, r P2
		want    float64
	}{
		{P2{10, 20}, P2{13, 24}, P2{14, 17}, -5}, // a 3-4-5 line, a point 5 to its right
		{P2{10, 20}, P2{13, 24}, P2{6, 23}, 5},   // and 5 to its left
		{P2{13, 24}, P2{10, 20}, P2{6, 23}, -5},  // the line the other way
		{P2{100, 50}, P2{100, 150}, P2{97, 80}, 3},
		{P2{7, 7}, P2{7, 7}, P2{9, 9}, 0}, // a line of no length
	} {
		if got := Side(c.p, c.q, c.r); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("side(%v, %v, %v) = %v, want %v", c.p, c.q, c.r, got, c.want)
		}
	}
}

func TestProperlyCrossNeedsBothSegmentsCutInTheirMiddles(t *testing.T) {
	for _, c := range []struct {
		p, q, r, s P2
		want       bool
	}{
		{P2{100, 100}, P2{200, 200}, P2{100, 200}, P2{200, 100}, true},      // an X
		{P2{200, 200}, P2{100, 100}, P2{100, 200}, P2{200, 100}, true},      // the first the other way
		{P2{100, 100}, P2{200, 200}, P2{200, 100}, P2{100, 200}, true},      // the second the other way
		{P2{200, 200}, P2{100, 100}, P2{200, 100}, P2{100, 200}, true},      // both
		{P2{100, 100}, P2{200, 200}, P2{150, 150}, P2{200, 100}, false},     // a T: one ends on the other
		{P2{150, 150}, P2{200, 200}, P2{100, 200}, P2{200, 100}, false},     // a T: the first starts on the second
		{P2{200, 200}, P2{150, 150}, P2{200, 100}, P2{100, 200}, false},     // and ends on it, the other way round
		{P2{100, 100}, P2{200, 200}, P2{100, 200}, P2{150, 150}, false},     // a T: the second ends on the first
		{P2{100, 100}, P2{200, 200}, P2{200, 200}, P2{300, 100}, false},     // a V: they meet at an end
		{P2{100, 100}, P2{200, 200}, P2{150, 150}, P2{250, 250}, false},     // on one line
		{P2{100, 100}, P2{200, 100}, P2{100, 101}, P2{200, 101}, false},     // side by side
		{P2{100, 100}, P2{200, 200}, P2{100, 200}, P2{140, 160.005}, false}, // stops a hair short
	} {
		if got := ProperlyCross(c.p, c.q, c.r, c.s); got != c.want {
			t.Errorf("properlyCross(%v, %v, %v, %v) = %v", c.p, c.q, c.r, c.s, got)
		}
		if got := ProperlyCross(c.r, c.s, c.p, c.q); got != c.want {
			t.Errorf("properlyCross(%v, %v, %v, %v) = %v: the pair swapped", c.r, c.s, c.p, c.q, got)
		}
	}
}

func TestAlongsideNeedsOneLineAndMoreThanAPointInCommon(t *testing.T) {
	for _, c := range []struct {
		p, q, r, s P2
		want       bool
	}{
		{P2{100, 50}, P2{200, 50}, P2{150, 50}, P2{300, 50}, true},         // overlapping by 50
		{P2{100, 50}, P2{200, 50}, P2{300, 50}, P2{150, 50}, true},         // the second the other way
		{P2{200, 50}, P2{100, 50}, P2{150, 50}, P2{300, 50}, true},         // the first the other way
		{P2{100, 50}, P2{200, 50}, P2{120, 50.005}, P2{180, 50.005}, true}, // within the tolerance
		{P2{100, 50}, P2{200, 50}, P2{120, 50.02}, P2{180, 50.02}, false},  // past it
		{P2{100, 50}, P2{200, 50}, P2{120, 50}, P2{180, 50.02}, false},     // one end off the line
		{P2{100, 50}, P2{200, 50}, P2{200, 50}, P2{300, 50}, false},        // end to end
		{P2{100, 50}, P2{200, 50}, P2{200.005, 50}, P2{300, 50}, false},    // a hair apart
		{P2{100, 50}, P2{200, 50}, P2{199.98, 50}, P2{300, 50}, true},      // a hair over
		{P2{100, 50}, P2{200, 50}, P2{-300, 50}, P2{0, 50}, false},         // far apart on the line
		{P2{50, 100}, P2{50, 200}, P2{50, 150}, P2{50, 400}, true},         // upright
	} {
		if got := Alongside(c.p, c.q, c.r, c.s); got != c.want {
			t.Errorf("alongside(%v, %v, %v, %v) = %v", c.p, c.q, c.r, c.s, got)
		}
	}
}

func TestInHullTakesItsEdgesWithinTheTolerance(t *testing.T) {
	sq := ConvexHull([]P2{{100, 200}, {300, 200}, {300, 400}, {100, 400}})
	for p, want := range map[P2]bool{
		{200, 300}: true, {300, 300}: true, {300.005, 300}: true, {300.02, 300}: false,
		{99.995, 300}: true, {99.98, 300}: false, {200, 199.995}: true, {200, 199.98}: false,
		{200, 400.005}: true, {200, 400.02}: false, {100, 200}: true, {50, 50}: false,
	} {
		if got := InHull(sq, p); got != want {
			t.Errorf("inHull(square, %v) = %v", p, got)
		}
	}
	for p, want := range map[P2]bool{{200, 300}: true, {299.995, 300}: false, {300, 300}: false, {150, 250}: true} {
		if got := StrictlyIn(sq, p); got != want {
			t.Errorf("strictlyIn(square, %v) = %v", p, got)
		}
	}
}

func TestBoxesOverlapWithinTheTolerance(t *testing.T) {
	a := box(mapfile.Vec3{100, 200, 300}, mapfile.Vec3{150, 250, 350})
	for name, c := range map[string]struct {
		b    Box
		want bool
	}{
		"inside":          {box(mapfile.Vec3{110, 210, 310}, mapfile.Vec3{120, 220, 320}), true},
		"touching":        {box(mapfile.Vec3{150, 200, 300}, mapfile.Vec3{180, 250, 350}), true},
		"a hair apart":    {box(mapfile.Vec3{150.005, 200, 300}, mapfile.Vec3{180, 250, 350}), true},
		"apart":           {box(mapfile.Vec3{150.02, 200, 300}, mapfile.Vec3{180, 250, 350}), false},
		"below":           {box(mapfile.Vec3{100, 200, 250}, mapfile.Vec3{150, 250, 299.98}), false},
		"below, touching": {box(mapfile.Vec3{100, 200, 250}, mapfile.Vec3{150, 250, 300}), true},
		"beside on y":     {box(mapfile.Vec3{100, 250.02, 300}, mapfile.Vec3{150, 260, 350}), false},
		"beside on y, -y": {box(mapfile.Vec3{100, 150, 300}, mapfile.Vec3{150, 199.98, 350}), false},
		"empty":           {Box{}, false},
	} {
		if got := a.Overlaps(c.b); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
		if got := c.b.Overlaps(a); got != c.want {
			t.Errorf("%s, the other way: %v", name, got)
		}
	}
}

func TestHullsCoverAndTouch(t *testing.T) {
	sq := ConvexHull([]P2{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {1, 1}, {2, 2}})
	if len(sq) != 4 {
		t.Fatalf("hull %v: the square's four corners, a point inside and a repeat dropped", sq)
	}
	inner := ConvexHull([]P2{{0.5, 0.5}, {1, 0.5}, {1, 1}})
	if !Covers(sq, inner) || Covers(inner, sq) || !Covers(sq, sq) {
		t.Error("covers")
	}
	cross := ConvexHull([]P2{{-1, 0.5}, {3, 0.5}, {3, 1.5}, {-1, 1.5}})
	far := ConvexHull([]P2{{3, 3}, {4, 3}, {4, 4}})
	edge := ConvexHull([]P2{{2, 0}, {3, 0}, {2, 2}})
	corner := ConvexHull([]P2{{2, 0}, {3, 0}, {3, 2}})
	if !Touch(sq, cross) || !Touch(sq, inner) || Touch(sq, far) || !Touch(sq, edge) || Touch(sq, corner) || !Touch(sq, sq) {
		t.Error("touch: a band across, one inside, one apart, one sharing an edge, one a corner only, itself")
	}
	if ConvexHull([]P2{{0, 0}, {1, 1}}) != nil || len(ConvexHull([]P2{{0, 0}, {1, 1}, {2, 2}})) > 2 {
		t.Error("fewer than three points, or three in a line, is no hull")
	}
}
