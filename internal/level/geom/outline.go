package geom

import (
	"fmt"
	"math"
)

// OutlineOf is an outline on the ground as a polygon map_edit can build:
// counter-clockwise, its repeated and collinear corners dropped, its edges
// crossing none but their neighbours.
func OutlineOf(pts [][2]float64) ([]P2, error) {
	var poly []P2
	for _, p := range pts {
		q := P2{p[0], p[1]}
		if len(poly) == 0 || math.Hypot(q.X-poly[len(poly)-1].X, q.Y-poly[len(poly)-1].Y) > Eps {
			poly = append(poly, q)
		}
	}
	if len(poly) > 1 && math.Hypot(poly[0].X-poly[len(poly)-1].X, poly[0].Y-poly[len(poly)-1].Y) <= Eps {
		poly = poly[:len(poly)-1]
	}
	poly = dropCollinear(poly)
	if len(poly) < 3 {
		return nil, fmt.Errorf("an outline needs three corners or more, not all on a line")
	}
	if SignedArea(poly) < 0 {
		for i, j := 0, len(poly)-1; i < j; i, j = i+1, j-1 {
			poly[i], poly[j] = poly[j], poly[i]
		}
	}
	for i := range poly {
		for j := i + 2; j < len(poly); j++ {
			if (j+1)%len(poly) != i && ProperlyCross(poly[i], poly[(i+1)%len(poly)], poly[j], poly[(j+1)%len(poly)]) {
				return nil, fmt.Errorf("the outline crosses itself: edge %d meets edge %d", i, j)
			}
		}
	}
	return poly, nil
}

// dropCollinear drops the corners on the line between their neighbours.
func dropCollinear(poly []P2) []P2 {
	var out []P2
	for i, p := range poly {
		a, b := poly[(i+len(poly)-1)%len(poly)], poly[(i+1)%len(poly)]
		if math.Abs(Cross2(a, p, b)) > Eps*math.Hypot(b.X-a.X, b.Y-a.Y) {
			out = append(out, p)
		}
	}
	return out
}

// SignedArea is a polygon's area, positive when it winds counter-clockwise.
func SignedArea(poly []P2) float64 {
	var a float64
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		a += p.X*q.Y - q.X*p.Y
	}
	return a / 2
}

// ConvexParts splits a counter-clockwise outline into convex polygons, few
// of them: ear clipping into triangles, then neighbours merged while what
// they make stays convex (Hertel–Mehlhorn). Each is a brush, as a brush
// can only be convex.
func ConvexParts(poly []P2) ([][]P2, error) {
	if convex(poly) {
		return [][]P2{poly}, nil
	}
	tris, err := earClip(poly)
	if err != nil {
		return nil, err
	}
	parts := tris
	for range len(tris) { // each pass merges a pair, or none and the merging is done
		merged := false
		for i := range parts {
			for j := i + 1; j < len(parts) && !merged; j++ {
				if u, ok := mergeConvex(parts[i], parts[j]); ok {
					parts[i], parts[j] = u, parts[len(parts)-1]
					parts, merged = parts[:len(parts)-1], true
				}
			}
			if merged {
				break
			}
		}
		if !merged {
			break
		}
	}
	return parts, nil
}

// convex tells a counter-clockwise polygon that turns left, or runs
// straight, at every corner.
func convex(poly []P2) bool {
	for i, p := range poly {
		if Cross2(poly[(i+len(poly)-1)%len(poly)], p, poly[(i+1)%len(poly)]) < -Eps {
			return false
		}
	}
	return true
}

// earClip cuts a counter-clockwise polygon into triangles, an ear at a
// time: a convex corner whose triangle holds no other corner.
func earClip(poly []P2) ([][]P2, error) {
	left := append([]P2(nil), poly...)
	var tris [][]P2
	for range len(poly) {
		if len(left) == 3 {
			return append(tris, left), nil
		}
		ear := -1
		for i := range left {
			if isEar(left, i) {
				ear = i
				break
			}
		}
		if ear < 0 {
			return nil, fmt.Errorf("the outline can't be cut into triangles: is it one shape, its corners in order?")
		}
		n := len(left)
		tris = append(tris, []P2{left[(ear+n-1)%n], left[ear], left[(ear+1)%n]})
		left = append(left[:ear:ear], left[ear+1:]...)
	}
	return nil, fmt.Errorf("the outline can't be cut into triangles")
}

// isEar tells whether corner i cuts off as a triangle inside the polygon.
func isEar(poly []P2, i int) bool {
	n := len(poly)
	a, b, c := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
	if Cross2(a, b, c) <= Eps {
		return false
	}
	tri := []P2{a, b, c}
	for j, p := range poly {
		if j != i && j != (i+n-1)%n && j != (i+1)%n && InHull(tri, p) {
			return false
		}
	}
	return true
}

// mergeConvex joins two convex polygons that share an edge, when what they
// make is convex too.
func mergeConvex(a, b []P2) ([]P2, bool) {
	same := func(p, q P2) bool { return math.Hypot(p.X-q.X, p.Y-q.Y) <= Eps }
	for i := range a {
		p, q := a[i], a[(i+1)%len(a)]
		for j := range b {
			if !same(b[j], q) || !same(b[(j+1)%len(b)], p) {
				continue
			}
			// a from q round to p, then b on from p back to q, both ends left out
			var u []P2
			for k := range len(a) {
				u = append(u, a[(i+1+k)%len(a)])
			}
			for k := range len(b) - 2 {
				u = append(u, b[(j+2+k)%len(b)])
			}
			u = dropCollinear(u)
			return u, len(u) >= 3 && convex(u)
		}
	}
	return nil, false
}
