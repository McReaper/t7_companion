package level

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// The checks' 2D geometry, case by case, away from the origin and both ways
// round, so a sign or a scale wrong anywhere shows.

func TestSideIsAPointsDistanceOffALine(t *testing.T) {
	for _, c := range []struct {
		p, q, r pt2
		want    float64
	}{
		{pt2{10, 20}, pt2{13, 24}, pt2{14, 17}, -5}, // a 3-4-5 line, a point 5 to its right
		{pt2{10, 20}, pt2{13, 24}, pt2{6, 23}, 5},   // and 5 to its left
		{pt2{13, 24}, pt2{10, 20}, pt2{6, 23}, -5},  // the line the other way
		{pt2{100, 50}, pt2{100, 150}, pt2{97, 80}, 3},
		{pt2{7, 7}, pt2{7, 7}, pt2{9, 9}, 0}, // a line of no length
	} {
		if got := side(c.p, c.q, c.r); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("side(%v, %v, %v) = %v, want %v", c.p, c.q, c.r, got, c.want)
		}
	}
}

func TestProperlyCrossNeedsBothSegmentsCutInTheirMiddles(t *testing.T) {
	for _, c := range []struct {
		p, q, r, s pt2
		want       bool
	}{
		{pt2{100, 100}, pt2{200, 200}, pt2{100, 200}, pt2{200, 100}, true},      // an X
		{pt2{200, 200}, pt2{100, 100}, pt2{100, 200}, pt2{200, 100}, true},      // the first the other way
		{pt2{100, 100}, pt2{200, 200}, pt2{200, 100}, pt2{100, 200}, true},      // the second the other way
		{pt2{200, 200}, pt2{100, 100}, pt2{200, 100}, pt2{100, 200}, true},      // both
		{pt2{100, 100}, pt2{200, 200}, pt2{150, 150}, pt2{200, 100}, false},     // a T: one ends on the other
		{pt2{150, 150}, pt2{200, 200}, pt2{100, 200}, pt2{200, 100}, false},     // a T: the first starts on the second
		{pt2{200, 200}, pt2{150, 150}, pt2{200, 100}, pt2{100, 200}, false},     // and ends on it, the other way round
		{pt2{100, 100}, pt2{200, 200}, pt2{100, 200}, pt2{150, 150}, false},     // a T: the second ends on the first
		{pt2{100, 100}, pt2{200, 200}, pt2{200, 200}, pt2{300, 100}, false},     // a V: they meet at an end
		{pt2{100, 100}, pt2{200, 200}, pt2{150, 150}, pt2{250, 250}, false},     // on one line
		{pt2{100, 100}, pt2{200, 100}, pt2{100, 101}, pt2{200, 101}, false},     // side by side
		{pt2{100, 100}, pt2{200, 200}, pt2{100, 200}, pt2{140, 160.005}, false}, // stops a hair short
	} {
		if got := properlyCross(c.p, c.q, c.r, c.s); got != c.want {
			t.Errorf("properlyCross(%v, %v, %v, %v) = %v", c.p, c.q, c.r, c.s, got)
		}
		if got := properlyCross(c.r, c.s, c.p, c.q); got != c.want {
			t.Errorf("properlyCross(%v, %v, %v, %v) = %v: the pair swapped", c.r, c.s, c.p, c.q, got)
		}
	}
}

func TestAlongsideNeedsOneLineAndMoreThanAPointInCommon(t *testing.T) {
	for _, c := range []struct {
		p, q, r, s pt2
		want       bool
	}{
		{pt2{100, 50}, pt2{200, 50}, pt2{150, 50}, pt2{300, 50}, true},         // overlapping by 50
		{pt2{100, 50}, pt2{200, 50}, pt2{300, 50}, pt2{150, 50}, true},         // the second the other way
		{pt2{200, 50}, pt2{100, 50}, pt2{150, 50}, pt2{300, 50}, true},         // the first the other way
		{pt2{100, 50}, pt2{200, 50}, pt2{120, 50.005}, pt2{180, 50.005}, true}, // within the tolerance
		{pt2{100, 50}, pt2{200, 50}, pt2{120, 50.02}, pt2{180, 50.02}, false},  // past it
		{pt2{100, 50}, pt2{200, 50}, pt2{120, 50}, pt2{180, 50.02}, false},     // one end off the line
		{pt2{100, 50}, pt2{200, 50}, pt2{200, 50}, pt2{300, 50}, false},        // end to end
		{pt2{100, 50}, pt2{200, 50}, pt2{200.005, 50}, pt2{300, 50}, false},    // a hair apart
		{pt2{100, 50}, pt2{200, 50}, pt2{199.98, 50}, pt2{300, 50}, true},      // a hair over
		{pt2{100, 50}, pt2{200, 50}, pt2{-300, 50}, pt2{0, 50}, false},         // far apart on the line
		{pt2{50, 100}, pt2{50, 200}, pt2{50, 150}, pt2{50, 400}, true},         // upright
	} {
		if got := alongside(c.p, c.q, c.r, c.s); got != c.want {
			t.Errorf("alongside(%v, %v, %v, %v) = %v", c.p, c.q, c.r, c.s, got)
		}
	}
}

func TestInHullTakesItsEdgesWithinTheTolerance(t *testing.T) {
	sq := convexHull([]pt2{{100, 200}, {300, 200}, {300, 400}, {100, 400}})
	for p, want := range map[pt2]bool{
		{200, 300}: true, {300, 300}: true, {300.005, 300}: true, {300.02, 300}: false,
		{99.995, 300}: true, {99.98, 300}: false, {200, 199.995}: true, {200, 199.98}: false,
		{200, 400.005}: true, {200, 400.02}: false, {100, 200}: true, {50, 50}: false,
	} {
		if got := inHull(sq, p); got != want {
			t.Errorf("inHull(square, %v) = %v", p, got)
		}
	}
	for p, want := range map[pt2]bool{{200, 300}: true, {299.995, 300}: false, {300, 300}: false, {150, 250}: true} {
		if got := strictlyIn(sq, p); got != want {
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
		if got := a.overlaps(c.b); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
		if got := c.b.overlaps(a); got != c.want {
			t.Errorf("%s, the other way: %v", name, got)
		}
	}
}

func TestSameTilesWithinAThousandth(t *testing.T) {
	base := [6]float64{64, 32, 16, 8, 90, 0}
	for name, c := range map[string]struct {
		d    [6]float64
		want bool
	}{
		"the same":                   {[6]float64{}, true},
		"a hair wider":               {[6]float64{0.0005}, true},
		"wider":                      {[6]float64{0.002}, false},
		"a hair taller":              {[6]float64{0, 0.0005}, true},
		"taller":                     {[6]float64{0, 0.002}, false},
		"a tile across further":      {[6]float64{0, 0, 64}, true},
		"a tile down further":        {[6]float64{0, 0, 0, -32}, true},
		"a hair less than a tile on": {[6]float64{0, 0, 63.9995}, true},
		"shifted across":             {[6]float64{0, 0, 1}, false},
		"shifted down":               {[6]float64{0, 0, 0, 1}, false},
		"a turn further":             {[6]float64{0, 0, 0, 0, 360}, true},
		"a hair less than a turn":    {[6]float64{0, 0, 0, 0, 359.9995}, true},
		"turned back":                {[6]float64{0, 0, 0, 0, -180}, false},
		"turned a little":            {[6]float64{0, 0, 0, 0, 0.002}, false},
		"mirrored":                   {[6]float64{-128}, false},
	} {
		b := base
		for i := range b {
			b[i] += c.d[i]
		}
		if got := sameTiles(base, b); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
		if got := sameTiles(b, base); got != c.want {
			t.Errorf("%s, the other way: %v", name, got)
		}
	}
}

func TestFramingFromAboveFramesABoardAwayFromTheOrigin(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	for _, b := range []Box{
		box(mapfile.Vec3{100, -40, 500}, mapfile.Vec3{1748, 60, 508}),     // as wide as the view's aspect
		box(mapfile.Vec3{-40, 200, 500}, mapfile.Vec3{60, 1069, 508}),     // as deep
		box(mapfile.Vec3{3000, 3000, 900}, mapfile.Vec3{3800, 3500, 908}), // far off
	} {
		p := FrameBox(l, above, b)
		x, y, ok := l.Project(p, b.Centre())
		if !ok || math.Abs(x-824) > 1 || math.Abs(y-434.5) > 1 {
			t.Errorf("%v: its middle at %v, %v", b, x, y)
		}
		var lo, hi [2]float64
		for i, c := range b.Corners() {
			cx, cy, _ := l.Project(p, c)
			if i == 0 {
				lo, hi = [2]float64{cx, cy}, [2]float64{cx, cy}
			}
			lo, hi = [2]float64{math.Min(lo[0], cx), math.Min(lo[1], cy)}, [2]float64{math.Max(hi[0], cx), math.Max(hi[1], cy)}
		}
		if fill := math.Max((hi[0]-lo[0])/1648, (hi[1]-lo[1])/869); fill < 0.85 || fill > 1 {
			t.Errorf("%v fills %.2f of the view", b, fill)
		}
	}
}
