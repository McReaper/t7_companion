package geom

import "math"

// CoveredBy tells whether hulls together cover hull b: nothing of b is left
// once each is taken away from it, as where a wall's end butts against two
// walls that meet behind it.
func CoveredBy(hulls [][]P2, b []P2) bool {
	left := [][]P2{b}
	for _, h := range hulls {
		var next [][]P2
		for _, r := range left {
			next = append(next, subtract(r, h)...)
		}
		if left = next; len(left) == 0 {
			return true
		}
	}
	return false
}

// subtract is what is left of the convex polygon r outside the hull h, in
// convex pieces: the part of r beyond each of h's edges in turn, r cut down
// to the edge's inside before the next. Slivers no wider than eps are
// dropped: they are rounding, not something to see.
func subtract(r, h []P2) [][]P2 {
	var out [][]P2
	for i := range h {
		a, b := h[i], h[(i+1)%len(h)]
		if beyond := clip(r, b, a); !sliver(beyond) {
			out = append(out, beyond)
		}
		if r = clip(r, a, b); sliver(r) {
			return out
		}
	}
	return out
}

// clip is the part of a convex polygon on the left of the line a→b
// (Sutherland–Hodgman).
func clip(poly []P2, a, b P2) []P2 {
	var out []P2
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		dp, dq := Cross2(a, b, p), Cross2(a, b, q)
		if dp >= 0 {
			out = append(out, p)
		}
		if dp > 0 && dq < 0 || dp < 0 && dq > 0 {
			t := dp / (dp - dq)
			out = append(out, P2{p.X + t*(q.X-p.X), p.Y + t*(q.Y-p.Y)})
		}
	}
	return out
}

// sliver tells a polygon with no area, or none wider than eps: its area
// against its perimeter.
func sliver(poly []P2) bool {
	if len(poly) < 3 {
		return true
	}
	var area, perimeter float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		area += p.X*q.Y - q.X*p.Y
		perimeter += math.Hypot(q.X-p.X, q.Y-p.Y)
	}
	return math.Abs(area)/2 <= Eps*perimeter
}

// Overlapping tells whether two convex hulls share an area, not just an edge
// or a corner: what is left of a cut down to inside each of b's edges.
func Overlapping(a, b []P2) bool {
	r := a
	for i := range b {
		if r = clip(r, b[i], b[(i+1)%len(b)]); sliver(r) {
			return false
		}
	}
	return true
}
