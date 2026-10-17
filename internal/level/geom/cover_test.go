package geom

import "testing"

func rect(x0, y0, x1, y1 float64) []P2 { return []P2{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} }

func TestHullsCoverTogetherOnlyWhatTheyLeaveNothingOf(t *testing.T) {
	face := rect(0, 0, 100, 50)
	for name, c := range map[string]struct {
		hulls [][]P2
		want  bool
	}{
		"two halves":                 {[][]P2{rect(0, 0, 60, 50), rect(60, 0, 100, 50)}, true},
		"two overlapping":            {[][]P2{rect(-10, -10, 70, 60), rect(40, 0, 100, 50)}, true},
		"three, an L and its corner": {[][]P2{rect(0, 0, 50, 50), rect(50, 0, 100, 25), rect(50, 25, 100, 50)}, true},
		"a gap between":              {[][]P2{rect(0, 0, 49, 50), rect(51, 0, 100, 50)}, false},
		"a gap thinner than eps":     {[][]P2{rect(0, 0, 50, 50), rect(50.005, 0, 100, 50)}, true},
		"a corner left out":          {[][]P2{rect(0, 0, 100, 40), rect(0, 40, 90, 50)}, false},
		"a triangle's worth short":   {[][]P2{rect(0, 0, 50, 50), {{50, 0}, {100, 0}, {100, 50}}}, false},
		"none":                       {nil, false},
	} {
		if got := CoveredBy(c.hulls, face); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// Far from the origin a gap half a unit wide still shows: a sliver is
	// judged by its own size, not by where it is.
	far := rect(10000, 20000, 10100, 20050)
	if CoveredBy([][]P2{rect(10000, 20000, 10049.75, 20050), rect(10050.25, 20000, 10100, 20050)}, far) {
		t.Error("a gap half a unit wide, far from the origin")
	}
}

func TestSubtractLeavesConvexPiecesOutsideTheHull(t *testing.T) {
	area := func(ps [][]P2) float64 {
		var a float64
		for _, p := range ps {
			for i := range p {
				q, r := p[i], p[(i+1)%len(p)]
				a += (q.X*r.Y - r.X*q.Y) / 2
			}
		}
		return a
	}
	// a 100×50 face with a 20×20 hole in its middle taken out: 4600 left
	left := subtract(rect(0, 0, 100, 50), rect(40, 15, 60, 35))
	if a := area(left); a < 4600-Eps || a > 4600+Eps || len(left) != 4 {
		t.Errorf("%d pieces, %v of area: %v", len(left), a, left)
	}
	if left := subtract(rect(0, 0, 10, 10), rect(-5, -5, 20, 20)); len(left) != 0 {
		t.Errorf("a hull over the whole: %v", left)
	}
	if !sliver(rect(0, 0, 100, 0.01)) || sliver(rect(0, 0, 100, 0.1)) || !sliver([]P2{{0, 0}, {1, 1}}) {
		t.Error("a sliver is no wider than eps")
	}
}
