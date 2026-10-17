package geom

import (
	"math"
	"slices"
	"sort"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// P2 is a point in a plane's 2D coordinates.
type P2 struct{ X, Y float64 }

// PlaneKey is a plane rounded, so faces on one plane share it.
type PlaneKey [4]int64

// KeyOf is a plane's key: its normal to a thousandth, its distance to a tenth.
func KeyOf(p Plane) PlaneKey {
	q := func(x float64) int64 { return int64(math.Round(x * 1000)) }
	return PlaneKey{q(p.N[0]), q(p.N[1]), q(p.N[2]), int64(math.Round(p.D * 10))}
}

// Dominant is the axis a normal is closest to, as texAxes picks it.
func Dominant(n mapfile.Vec3) int {
	ax, ay, az := math.Abs(n[0]), math.Abs(n[1]), math.Abs(n[2])
	switch {
	case az >= ax && az >= ay:
		return 2
	case ax >= ay:
		return 0
	}
	return 1
}

// Flatten drops an axis of the points.
func Flatten(pts []mapfile.Vec3, axis int) []P2 {
	out := make([]P2, len(pts))
	u, v := (axis+1)%3, (axis+2)%3
	for i, p := range pts {
		out[i] = P2{p[u], p[v]}
	}
	return out
}

// ConvexHull is the points' hull, counter-clockwise (Andrew's monotone chain).
func ConvexHull(pts []P2) []P2 {
	if len(pts) < 3 {
		return nil
	}
	pts = append([]P2(nil), pts...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].X < pts[j].X || pts[i].X == pts[j].X && pts[i].Y < pts[j].Y })
	lower := chain(pts)
	slices.Reverse(pts)
	return append(lower, chain(pts)...)
}

// chain is one half of a hull, the points taken in order, its last point
// left for the other half to start from. Its loops run over the points, so
// no counter can run away (a mutant that did ran CI's runner out of memory).
func chain(pts []P2) []P2 {
	var h []P2
	for _, p := range pts {
		for len(h) >= 2 && Cross2(h[len(h)-2], h[len(h)-1], p) <= 1e-9 {
			h = h[:len(h)-1]
		}
		h = append(h, p)
	}
	return h[:len(h)-1]
}

// Cross2 is (a - o) × (b - o): positive when o, a, b turn left.
func Cross2(o, a, b P2) float64 { return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X) }

// InHull tells whether a point is inside or on a counter-clockwise hull.
func InHull(h []P2, p P2) bool {
	for i := range h {
		a, b := h[i], h[(i+1)%len(h)]
		if Cross2(a, b, p) < -Eps*math.Hypot(b.X-a.X, b.Y-a.Y) {
			return false
		}
	}
	return true
}

// Covers tells whether hull a holds every corner of hull b.
func Covers(a, b []P2) bool {
	for _, p := range b {
		if !InHull(a, p) {
			return false
		}
	}
	return true
}

// Touch tells whether two hulls overlap or share a stretch of edge, where a
// seam would show: a corner of one strictly inside the other, edges that
// cross, or edges along one line that overlap. Hulls that meet at a corner
// alone don't.
func Touch(a, b []P2) bool {
	for _, p := range a {
		if StrictlyIn(b, p) {
			return true
		}
	}
	for _, p := range b {
		if StrictlyIn(a, p) {
			return true
		}
	}
	for i := range a {
		p, q := a[i], a[(i+1)%len(a)]
		for j := range b {
			if r, s := b[j], b[(j+1)%len(b)]; ProperlyCross(p, q, r, s) || Alongside(p, q, r, s) {
				return true
			}
		}
	}
	return false
}

// Side is where a point lies off a segment's line, in units: positive on its
// left, within eps of 0 on it.
func Side(p, q, r P2) float64 {
	return Cross2(p, q, r) / math.Max(math.Hypot(q.X-p.X, q.Y-p.Y), 1e-12)
}

// StrictlyIn tells whether a point is inside a counter-clockwise hull, off
// its edges.
func StrictlyIn(h []P2, p P2) bool {
	for i := range h {
		if Side(h[i], h[(i+1)%len(h)], p) <= Eps {
			return false
		}
	}
	return true
}

// ProperlyCross tells whether two segments cross each other inside both.
func ProperlyCross(p, q, r, s P2) bool {
	d1, d2, d3, d4 := Side(r, s, p), Side(r, s, q), Side(p, q, r), Side(p, q, s)
	return (d1 > Eps && d2 < -Eps || d1 < -Eps && d2 > Eps) && (d3 > Eps && d4 < -Eps || d3 < -Eps && d4 > Eps)
}

// Alongside tells whether two segments lie on one line and overlap by more
// than a point.
func Alongside(p, q, r, s P2) bool {
	if math.Abs(Side(p, q, r)) > Eps || math.Abs(Side(p, q, s)) > Eps {
		return false
	}
	l := math.Hypot(q.X-p.X, q.Y-p.Y)
	at := func(x P2) float64 { return ((x.X-p.X)*(q.X-p.X) + (x.Y-p.Y)*(q.Y-p.Y)) / math.Max(l, 1e-12) }
	lo, hi := math.Max(0, math.Min(at(r), at(s))), math.Min(l, math.Max(at(r), at(s)))
	return hi-lo > Eps
}

// Overlaps tells whether two boxes share a point, within eps.
func (b Box) Overlaps(o Box) bool {
	for i := range 3 {
		if b.Max[i] < o.Min[i]-Eps || o.Max[i] < b.Min[i]-Eps {
			return false
		}
	}
	return b.Ok && o.Ok
}
