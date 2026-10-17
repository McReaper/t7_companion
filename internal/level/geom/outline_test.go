package geom

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestAnOutlineIsCleanedAndWoundCounterClockwise(t *testing.T) {
	poly, err := OutlineOf([][2]float64{{0, 0}, {0, 100}, {0, 100}, {50, 100}, {100, 100}, {100, 0}, {0, 0}})
	if err != nil || fmt.Sprint(poly) != "[{100 0} {100 100} {0 100} {0 0}]" {
		t.Errorf("clockwise, a corner twice, one on a line, the first again at the end: %v, %v", poly, err)
	}
	for want, pts := range map[string][][2]float64{
		"three corners or more":   {{0, 0}, {100, 0}, {200, 0}},
		"crosses itself: edge 0":  {{0, 0}, {100, 100}, {100, 0}, {0, 100}},
		"three corners or more, ": {{0, 0}, {0, 0}},
	} {
		if _, err := OutlineOf(pts); err == nil || !strings.Contains(err.Error(), strings.TrimSuffix(want, ", ")) {
			t.Errorf("%v: %v", pts, err)
		}
	}
}

func TestAnOutlineSplitsIntoFewConvexParts(t *testing.T) {
	area := func(ps [][]P2) float64 {
		var a float64
		for _, p := range ps {
			a += SignedArea(p)
		}
		return a
	}
	for name, c := range map[string]struct {
		pts   [][2]float64
		parts int
		area  float64
	}{
		"a square":               {[][2]float64{{0, 0}, {100, 0}, {100, 100}, {0, 100}}, 1, 10000},
		"a hexagon, at an angle": {[][2]float64{{50, 0}, {100, 25}, {100, 75}, {50, 100}, {0, 75}, {0, 25}}, 1, 7500},
		"an L":                   {[][2]float64{{0, 0}, {200, 0}, {200, 100}, {100, 100}, {100, 200}, {0, 200}}, 2, 30000},
		"a U":                    {[][2]float64{{0, 0}, {300, 0}, {300, 200}, {200, 200}, {200, 100}, {100, 100}, {100, 200}, {0, 200}}, 3, 50000},
		"a bent corridor":        {[][2]float64{{0, 0}, {256, 0}, {400, 144}, {400, 400}, {272, 400}, {272, 196}, {204, 128}, {0, 128}}, 4, 0}, // two reflex corners: 3 at best, merging finds 4
	} {
		poly, err := OutlineOf(c.pts)
		if err != nil {
			t.Fatal(name, err)
		}
		parts, err := ConvexParts(poly)
		if err != nil || len(parts) > c.parts {
			t.Errorf("%s: %d parts, at most %d: %v", name, len(parts), c.parts, err)
		}
		for _, p := range parts {
			if !convex(p) || SignedArea(p) <= 0 {
				t.Errorf("%s: a part not convex and counter-clockwise: %v", name, p)
			}
		}
		if want := SignedArea(poly); math.Abs(area(parts)-want) > 1e-6 || (c.area > 0 && math.Abs(want-c.area) > 1e-6) {
			t.Errorf("%s: parts cover %v, the outline %v", name, area(parts), want)
		}
	}
}

// Outlines away from the origin: a corner given twice and the first given
// again at the end are dropped, and a corner a little off the line between
// its neighbours stays.
func TestOutlinesAwayFromTheOrigin(t *testing.T) {
	poly, err := OutlineOf([][2]float64{{1000, 1000}, {1100, 1000}, {1100, 1100}, {1100, 1100}, {1000, 1100}, {1000, 1000}})
	if err != nil || fmt.Sprint(poly) != "[{1000 1000} {1100 1000} {1100 1100} {1000 1100}]" {
		t.Errorf("a corner twice, the first again: %v, %v", poly, err)
	}
	poly, err = OutlineOf([][2]float64{{5000, 5000}, {5100, 5000.5}, {5200, 5000}, {5200, 5100}, {5000, 5100}})
	if err != nil || len(poly) != 5 {
		t.Errorf("half a unit off a line 200 long is a corner: %v, %v", poly, err)
	}
}
