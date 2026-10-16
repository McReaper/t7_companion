package level

import (
	"math"
	"slices"
	"strconv"
)

// The walls op's joints. A wall piece is a strip of the wall's thickness
// along each segment of its line; how each strip ends decides whether walls
// overlap where they meet:
//   - a turn inside a line, or two walls' ends meeting, is a corner: both
//     strips end on the line bisecting the turn (the mitre) and meet edge to
//     edge;
//   - an end on another wall's side (a T) stops on that wall's face;
//   - three walls at one point: the two turning widest make the corner, the
//     third is cut by both their faces;
//   - a free end, a door's jamb, a wall running on along another, ends square.
// A turn too sharp to mitre (its point would sit more than three half-walls
// out) keeps the old overlap: the strip runs half a wall past the point.

func sub2(a, b pt2) pt2             { return pt2{a.x - b.x, a.y - b.y} }
func cross2v(a, b pt2) float64      { return a.x*b.y - a.y*b.x }
func unit2(a, b pt2) pt2            { l := dist2(a, b); return pt2{(b.x - a.x) / l, (b.y - a.y) / l} }
func along(p, u pt2, s float64) pt2 { return pt2{p.x + u.x*s, p.y + u.y*s} }

// side is p moved half a wall to the left of direction u (side 1) or right (-1).
func aside(p, u pt2, h, s float64) pt2 { return pt2{p.x - u.y*h*s, p.y + u.x*h*s} }

// meetLines is where p + s d meets q + t e, or false when they are parallel.
func meetLines(p, d, q, e pt2) (pt2, bool) {
	den := cross2v(d, e)
	if math.Abs(den) < 1e-9 {
		return pt2{}, false
	}
	return along(p, d, cross2v(sub2(q, p), e)/den), true
}

// splitCrossings cuts, where two walls cross away from their ends, the
// later one in two; its pieces then stop on the other's faces.
func splitCrossings(pieces []wpiece, thick float64) []wpiece {
	for {
		k, s, x, ok := firstCrossing(pieces, thick)
		if !ok {
			return pieces
		}
		p := pieces[k]
		a := wpiece{pts: append(slices.Clone(p.pts[:s+1]), x), line: p.line}
		b := wpiece{pts: append([]pt2{x}, p.pts[s+1:]...), line: p.line}
		if p.ring {
			pts := append([]pt2{x}, p.pts[s+1:len(p.pts)-1]...)
			pieces = slices.Concat(pieces[:k], []wpiece{{pts: append(append(pts, p.pts[:s+1]...), x), line: p.line}}, pieces[k+1:])
			continue
		}
		pieces = slices.Concat(pieces[:k], []wpiece{a, b}, pieces[k+1:])
	}
}

// firstCrossing is the first piece another wall crosses well inside one of
// its segments: that piece, the segment, and the crossing.
func firstCrossing(pieces []wpiece, thick float64) (int, int, pt2, bool) {
	for i, p := range pieces {
		for j := i + 1; j < len(pieces); j++ {
			q := pieces[j]
			if q.line == p.line {
				continue
			}
			for s1 := range len(p.pts) - 1 {
				for s2 := range len(q.pts) - 1 {
					x, ok := properCrossing(p.pts[s1], p.pts[s1+1], q.pts[s2], q.pts[s2+1])
					if ok && dist2(x, q.pts[s2]) > thick && dist2(x, q.pts[s2+1]) > thick {
						return j, s2, x, true
					}
				}
			}
		}
	}
	return 0, 0, pt2{}, false
}

func properCrossing(a, c, p, q pt2) (pt2, bool) {
	r, s := sub2(c, a), sub2(q, p)
	den := cross2v(r, s)
	if math.Abs(den) < 1e-9 {
		return pt2{}, false
	}
	t := cross2v(sub2(p, a), s) / den
	v := cross2v(sub2(p, a), r) / den
	if t <= 1e-3 || t >= 1-1e-3 || v <= 1e-3 || v >= 1-1e-3 {
		return pt2{}, false
	}
	return along(a, r, t), true
}

// pend names an end of a piece: the piece, and 0 its first point, 1 its last.
type pend struct{ piece, end int }

func endPoint(pieces []wpiece, e pend) pt2 {
	p := pieces[e.piece].pts
	if e.end == 0 {
		return p[0]
	}
	return p[len(p)-1]
}

// endDir is the way an end's segment runs, from the end into the wall.
func endDir(pieces []wpiece, e pend) pt2 {
	p := pieces[e.piece].pts
	if e.end == 0 {
		return unit2(p[0], p[1])
	}
	return unit2(p[len(p)-1], p[len(p)-2])
}

// wcut is how an end is finished; a wface is a face it stops on: a point on
// it, its direction, and which of its sides the wall runs into.
type wcut struct {
	kind  byte // 's' square, 'm' mitre with dir, 'b' butt on faces[0], 'c' clipped by faces, 'o' overlap
	dir   pt2
	faces []wface
}

type wface struct {
	at, dir pt2
	side    float64
}

// endCut decides how an end meets what is there.
func endCut(pieces []wpiece, e pend, h float64) wcut {
	p := endPoint(pieces, e)
	var others []pend
	for k, q := range pieces {
		if q.ring {
			continue
		}
		for end := range 2 {
			o := pend{k, end}
			if o != e && dist2(endPoint(pieces, o), p) < 0.5 {
				others = append(others, o)
			}
		}
	}
	host, hs, ht, near := hostOf(pieces, e, p)
	if near && len(others) > 0 && sameLine(pieces, e, others) { // cut where another wall crosses: stop on its faces
		others = nil
	}
	switch {
	case len(others) == 1:
		return wcut{kind: 'm', dir: endDir(pieces, others[0])}
	case len(others) == 2 && distinctLines(pieces, e, others):
		return threeWay(pieces, e, others, h)
	case len(others) > 1:
		return wcut{kind: 'o'}
	case near:
		return butt(pieces, e, host, hs, ht, h)
	}
	return wcut{kind: 's'}
}

func sameLine(pieces []wpiece, e pend, others []pend) bool {
	for _, o := range others {
		if pieces[o.piece].line != pieces[e.piece].line {
			return false
		}
	}
	return true
}

func distinctLines(pieces []wpiece, e pend, others []pend) bool {
	a, b, c := pieces[e.piece].line, pieces[others[0].piece].line, pieces[others[1].piece].line
	return a != b && b != c && a != c
}

// hostOf is the other wall's segment the end lies on: piece, segment, the
// end's share along it, and whether it lies on it at all.
func hostOf(pieces []wpiece, e pend, p pt2) (int, int, float64, bool) {
	best, bk, bs, bt := 0.5, -1, -1, 0.0
	for k, q := range pieces {
		if q.line == pieces[e.piece].line {
			continue
		}
		for s := range len(q.pts) - 1 {
			if d, _, t := nearSeg(p, q.pts[s], q.pts[s+1]); d < best {
				best, bk, bs, bt = d, k, s, t
			}
		}
	}
	return bk, bs, bt, bk >= 0
}

// butt stops an end on the face of the wall it lies on; at a corner of that
// wall's line the other segment there may be the one it meets.
func butt(pieces []wpiece, e pend, host, s int, t, h float64) wcut {
	line, ring := pieces[host].pts, pieces[host].ring
	last := len(line) - 2
	if !ring && ((s == 0 && t < 1e-6) || (s == last && t > 1-1e-6)) {
		return wcut{kind: 'o'} // on another wall's very end: no joint a plan draws
	}
	segs := []int{s}
	switch { // at a corner of the host's line, the other segment there may be the one it meets
	case t < 1e-6 && s > 0:
		segs = append(segs, s-1)
	case t < 1e-6 && ring:
		segs = append(segs, last)
	case t > 1-1e-6 && s < last:
		segs = append(segs, s+1)
	case t > 1-1e-6 && ring:
		segs = append(segs, 0)
	}
	into := endDir(pieces, e)
	for _, s2 := range segs {
		u := unit2(line[s2], line[s2+1])
		if c := cross2v(u, into); math.Abs(c) > 0.1 {
			sd := math.Copysign(1, c)
			return wcut{kind: 'b', faces: []wface{{at: aside(line[s2], u, h, sd), dir: u, side: sd}}}
		}
	}
	return wcut{kind: 's'} // running on along it
}

// threeWay: the two of three walls at a point turning widest mitre; the
// third is cut by both their faces.
func threeWay(pieces []wpiece, e pend, others []pend, h float64) wcut {
	dirs := []pt2{endDir(pieces, e), endDir(pieces, others[0]), endDir(pieces, others[1])}
	angle := func(a, b pt2) float64 { return math.Acos(math.Max(-1, math.Min(1, a.x*b.x+a.y*b.y))) }
	pair, widest := [2]int{}, -1.0
	for i := range 3 {
		for j := i + 1; j < 3; j++ {
			if a := angle(dirs[i], dirs[j]); a > widest {
				pair, widest = [2]int{i, j}, a
			}
		}
	}
	if pair[0] == 0 {
		return wcut{kind: 'm', dir: dirs[pair[1]]}
	}
	var faces []wface
	p := endPoint(pieces, e)
	for _, i := range pair {
		u := pt2{-dirs[i].x, -dirs[i].y} // the corner wall's direction, into the point
		sd := math.Copysign(1, cross2v(u, dirs[0]))
		faces = append(faces, wface{at: aside(p, u, h, sd), dir: u, side: sd})
	}
	return wcut{kind: 'c', faces: faces}
}

// corner is the mitre of a turn at p from u to v: the left and right points,
// false when too sharp; straight on, the square ends.
func corner(p, u, v pt2, h float64) (pt2, pt2, bool) {
	l, ok1 := meetLines(aside(p, u, h, 1), u, aside(p, v, h, 1), v)
	r, ok2 := meetLines(aside(p, u, h, -1), u, aside(p, v, h, -1), v)
	if !ok1 || !ok2 {
		return aside(p, v, h, 1), aside(p, v, h, -1), true
	}
	if dist2(l, p) > 3*h || dist2(r, p) > 3*h {
		return pt2{}, pt2{}, false
	}
	return l, r, true
}

// wallPolys are the pieces' strips, segment by segment, each a convex
// outline on the ground.
func wallPolys(pieces []wpiece, thick float64) [][]pt2 {
	h := thick / 2
	var out [][]pt2
	for k, p := range pieces {
		n := len(p.pts) - 1
		for s := range n {
			a, c := p.pts[s], p.pts[s+1]
			if dist2(a, c) < 1 {
				continue
			}
			u := unit2(a, c)
			sl, sr, cl1 := stripEnd(pieces, k, s, 0, a, u, h)
			el, er, cl2 := stripEnd(pieces, k, s, 1, c, u, h)
			poly := []pt2{sl, el, er, sr}
			for _, f := range append(cl1, cl2...) {
				poly = clipHalf(poly, f)
			}
			out = append(out, poly)
		}
	}
	return out
}

// stripEnd is the left and right corners of segment s's strip at its start
// (end 0, point a) or its end (1), and any faces that clip it.
func stripEnd(pieces []wpiece, k, s, end int, a, u pt2, h float64) (pt2, pt2, []wface) {
	p := pieces[k]
	n := len(p.pts) - 1
	inner := (end == 0 && s > 0) || (end == 1 && s < n-1) || p.ring
	sign := 1.0
	if end == 0 {
		sign = -1
	}
	past := along(a, u, sign*h) // half a wall past the point: the overlap of a joint left as it was
	if inner {
		l, r := innerEnd(p, s, end, a, u, h)
		return l, r, nil
	}
	c := endCut(pieces, pend{k, end}, h)
	switch c.kind {
	case 'm':
		in, out := pt2{-c.dir.x, -c.dir.y}, u
		if end == 1 {
			in, out = u, c.dir
		}
		if l, r, ok := corner(a, in, out, h); ok {
			return l, r, nil
		}
		return aside(past, u, h, 1), aside(past, u, h, -1), nil
	case 'b':
		f := c.faces[0]
		l, _ := meetLines(aside(a, u, h, 1), u, f.at, f.dir)
		r, _ := meetLines(aside(a, u, h, -1), u, f.at, f.dir)
		return l, r, nil
	case 'c':
		far := along(a, u, sign*4*h)
		return aside(far, u, h, 1), aside(far, u, h, -1), c.faces
	case 'o':
		return aside(past, u, h, 1), aside(past, u, h, -1), nil
	}
	return aside(a, u, h, 1), aside(a, u, h, -1), nil
}

// innerEnd is a strip's corners at a turn inside its piece (a ring's first
// corner too): the mitre, or half a wall past the point when too sharp.
func innerEnd(p wpiece, s, end int, a, u pt2, h float64) (pt2, pt2) {
	n := len(p.pts) - 1
	in, out := u, u
	if end == 0 {
		prev := p.pts[n-1] // a ring's first point: the turn from its last segment
		if s > 0 {
			prev = p.pts[s-1]
		}
		in = unit2(prev, a)
	} else {
		next := p.pts[1] // a ring's last point: the turn into its first segment
		if s < n-1 {
			next = p.pts[s+2]
		}
		out = unit2(a, next)
	}
	if l, r, ok := corner(a, in, out, h); ok {
		return l, r
	}
	sign := 1.0
	if end == 0 {
		sign = -1
	}
	past := along(a, u, sign*h)
	return aside(past, u, h, 1), aside(past, u, h, -1)
}

// clipHalf keeps the part of a convex outline past a face, on the side the
// wall it is cut from runs into.
func clipHalf(poly []pt2, f wface) []pt2 {
	keep := func(p pt2) bool { return cross2v(f.dir, sub2(p, f.at))*f.side >= -1e-9 }
	var out []pt2
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		if keep(p) {
			out = append(out, p)
		}
		if keep(p) != keep(q) {
			if x, ok := meetLines(p, sub2(q, p), f.at, f.dir); ok {
				out = append(out, x)
			}
		}
	}
	return out
}

// quadAlong is the outline from a0 to a1 along u and w0 to w1 across it, from p.
func quadAlong(p, u pt2, a0, a1, w0, w1 float64) []pt2 {
	at := func(a, w float64) pt2 { q := along(p, u, a); return pt2{q.x - u.y*w, q.y + u.x*w} }
	return []pt2{at(a0, w0), at(a1, w0), at(a1, w1), at(a0, w1)}
}

// doorParts are a door's brushes: its lintel, the jambs narrowing it for
// the eye and their player clips, its trim.
func doorParts(c doorCut, k int, thick, z0, z1 float64, material, trim string) []wpart {
	h, t, e := thick/2, thick/2+trimDepth, (c.width-c.seen)/2
	top := z0 + c.height
	name := "door " + strconv.Itoa(k)
	out := []wpart{{poly: quadAlong(c.at, c.dir, 0, c.width, -h, h), z0: top, z1: z1, material: material, part: name + " lintel"}}
	if e > 0.5 {
		for _, a := range [][2]float64{{0, e}, {c.width - e, c.width}} {
			q := quadAlong(c.at, c.dir, a[0], a[1], -h, h)
			out = append(out,
				wpart{poly: q, z0: z0, z1: top, material: material, contents: "nonColliding", part: name + " jamb"},
				wpart{poly: q, z0: z0, z1: top, material: "clip_player", part: name + " player clip"})
		}
	}
	for _, a := range [][2]float64{{e, e + trimDepth}, {c.width - e - trimDepth, c.width - e}} {
		out = append(out, wpart{poly: quadAlong(c.at, c.dir, a[0], a[1], -t, t), z0: z0, z1: top, material: trim, contents: "weaponClip detail", part: name + " trim"})
	}
	return append(out, wpart{poly: quadAlong(c.at, c.dir, e, c.width-e, -t, t), z0: top - trimDepth, z1: top, material: trim, contents: "weaponClip detail", part: name + " trim"})
}
