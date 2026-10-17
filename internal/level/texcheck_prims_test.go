package level

import (
	"math"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// The checks' 2D geometry, case by case, away from the origin and both ways
// round, so a sign or a scale wrong anywhere shows.

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
	for _, b := range []geom.Box{
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
