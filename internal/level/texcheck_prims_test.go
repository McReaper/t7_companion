package level

import (
	"testing"
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
