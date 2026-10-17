package geom

import "math"

// Sub2 is a - b.
func Sub2(a, b P2) P2 { return P2{a.X - b.X, a.Y - b.Y} }

// CrossV is a × b: positive when b turns left of a.
func CrossV(a, b P2) float64 { return a.X*b.Y - a.Y*b.X }

// Unit2 is the unit vector from a towards b.
func Unit2(a, b P2) P2 { l := Dist2(a, b); return P2{(b.X - a.X) / l, (b.Y - a.Y) / l} }

// Along is p moved s along u.
func Along(p, u P2, s float64) P2 { return P2{p.X + u.X*s, p.Y + u.Y*s} }

// Aside is p moved h to the left of direction u (s 1) or to its right (s -1).
func Aside(p, u P2, h, s float64) P2 { return P2{p.X - u.Y*h*s, p.Y + u.X*h*s} }

// MeetLines is where p + s d meets q + t e, or false when they are parallel.
func MeetLines(p, d, q, e P2) (P2, bool) {
	den := CrossV(d, e)
	if math.Abs(den) < 1e-9 {
		return P2{}, false
	}
	return Along(p, d, CrossV(Sub2(q, p), e)/den), true
}

// ProperCrossing is where segments a-c and p-q cross, each cut away from its
// ends (more than a thousandth of its length in); false otherwise.
func ProperCrossing(a, c, p, q P2) (P2, bool) {
	r, s := Sub2(c, a), Sub2(q, p)
	den := CrossV(r, s)
	if math.Abs(den) < 1e-9 {
		return P2{}, false
	}
	t := CrossV(Sub2(p, a), s) / den
	v := CrossV(Sub2(p, a), r) / den
	if t <= 1e-3 || t >= 1-1e-3 || v <= 1e-3 || v >= 1-1e-3 {
		return P2{}, false
	}
	return Along(a, r, t), true
}

// Dist2 is the distance between a and b.
func Dist2(a, b P2) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
