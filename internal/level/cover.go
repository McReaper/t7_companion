package level

import "math"

// coveredBy tells whether hulls together cover hull b: nothing of b is left
// once each is taken away from it, as where a wall's end butts against two
// walls that meet behind it.
func coveredBy(hulls [][]pt2, b []pt2) bool {
	left := [][]pt2{b}
	for _, h := range hulls {
		var next [][]pt2
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
func subtract(r, h []pt2) [][]pt2 {
	var out [][]pt2
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
func clip(poly []pt2, a, b pt2) []pt2 {
	var out []pt2
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		dp, dq := cross2(a, b, p), cross2(a, b, q)
		if dp >= 0 {
			out = append(out, p)
		}
		if dp > 0 && dq < 0 || dp < 0 && dq > 0 {
			t := dp / (dp - dq)
			out = append(out, pt2{p.x + t*(q.x-p.x), p.y + t*(q.y-p.y)})
		}
	}
	return out
}

// sliver tells a polygon with no area, or none wider than eps: its area
// against its perimeter.
func sliver(poly []pt2) bool {
	if len(poly) < 3 {
		return true
	}
	var area, perimeter float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		area += p.x*q.y - q.x*p.y
		perimeter += math.Hypot(q.x-p.x, q.y-p.y)
	}
	return math.Abs(area)/2 <= eps*perimeter
}

// overlapping tells whether two convex hulls share an area, not just an edge
// or a corner: what is left of a cut down to inside each of b's edges.
func overlapping(a, b []pt2) bool {
	r := a
	for i := range b {
		if r = clip(r, b[i], b[(i+1)%len(b)]); sliver(r) {
			return false
		}
	}
	return true
}
