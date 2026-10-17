package geom

import (
	"math"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// Column is the stretch of a vertical line through (x, y) inside a convex
// brush, false when the line misses it: each plane bounds z from below or
// above, or leaves the line out.
func Column(planes []Plane, x, y float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(-1), math.Inf(1)
	for _, p := range planes {
		r := p.D - p.N[0]*x - p.N[1]*y // inside: n·v ≥ d, so n_z·z ≥ r
		switch {
		case p.N[2] > 1e-9:
			lo = math.Max(lo, r/p.N[2])
		case p.N[2] < -1e-9:
			hi = math.Min(hi, r/p.N[2])
		case r > Eps:
			return 0, 0, false
		}
	}
	return lo, hi, hi-lo > Eps
}

// OnTriangle is where a vertical line through (x, y) meets a triangle,
// false when it passes beside it.
func OnTriangle(t [3]mapfile.Vec3, x, y float64) (float64, bool) {
	d := (t[1][1]-t[2][1])*(t[0][0]-t[2][0]) + (t[2][0]-t[1][0])*(t[0][1]-t[2][1])
	if math.Abs(d) < 1e-9 {
		return 0, false
	}
	l0 := ((t[1][1]-t[2][1])*(x-t[2][0]) + (t[2][0]-t[1][0])*(y-t[2][1])) / d
	l1 := ((t[2][1]-t[0][1])*(x-t[2][0]) + (t[0][0]-t[2][0])*(y-t[2][1])) / d
	l2 := 1 - l0 - l1
	if l0 < -1e-6 || l1 < -1e-6 || l2 < -1e-6 {
		return 0, false
	}
	return l0*t[0][2] + l1*t[1][2] + l2*t[2][2], true
}
