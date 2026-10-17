package level

import (
	"math"
	"sort"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// A probe's box turns about the vertical to lie along walls off the axes, as
// Treyarch's do (zm_giant's light prefab: 40 to 45 degrees, their sizes in
// the turned box's own axes): an axis-aligned box against a slanted wall
// leaves its edge across the wall's face, a seam in the reflections. A turn
// is a box's axes on the ground, yaw degrees from the world's; the zero turn
// is the world's.
type turn struct{ yaw, cos, sin float64 }

const (
	minTurn  = 0.5 // degrees: a wall nearer an axis is on it
	hugDepth = 16  // cells: the widest corridor off the axes a box along its wall crosses, 128
)

func turnOf(yaw float64) turn {
	yaw = math.Round(yaw*1000) / 1000 // as the map writes it
	r := yaw * math.Pi / 180
	return turn{yaw, math.Cos(r), math.Sin(r)}
}

// local is a point of the ground in the turn's axes.
func (f turn) local(x, y float64) (float64, float64) {
	if f.yaw == 0 {
		return x, y
	}
	return x*f.cos + y*f.sin, -x*f.sin + y*f.cos
}

// world is a point in the turn's axes in the world's.
func (f turn) world(u, v float64) (float64, float64) {
	if f.yaw == 0 {
		return u, v
	}
	return u*f.cos - v*f.sin, u*f.sin + v*f.cos
}

// holds tells whether a point of the ground lies in a box's rectangle of
// cells, ends exclusive.
func (bx probeBox) holds(x, y float64) bool {
	u, v := bx.f.local(x, y)
	return u >= bx.rlo[0] && u < bx.rhi[0] && v >= bx.rlo[1] && v < bx.rhi[1]
}

// contains tells whether a point lies in a box, its sides included.
func (bx probeBox) contains(p mapfile.Vec3) bool {
	u, v := bx.f.local(p[0], p[1])
	return u >= bx.lo[0] && u <= bx.hi[0] && v >= bx.lo[1] && v <= bx.hi[1] && p[2] >= bx.lo[2] && p[2] <= bx.hi[2]
}

// at is a point in a box's axes in the world.
func (bx probeBox) at(p mapfile.Vec3) mapfile.Vec3 {
	x, y := bx.f.world(p[0], p[1])
	return mapfile.Vec3{x, y, p[2]}
}

// extentIn is a box's ground extent in a turn's axes: its own sides in its
// own, else the bounds of its corners.
func (bx probeBox) extentIn(f turn) (lo, hi [2]float64) {
	if f.yaw == bx.f.yaw {
		return [2]float64{bx.lo[0], bx.lo[1]}, [2]float64{bx.hi[0], bx.hi[1]}
	}
	lo, hi = [2]float64{math.Inf(1), math.Inf(1)}, [2]float64{math.Inf(-1), math.Inf(-1)}
	for _, c := range [4][2]float64{{bx.lo[0], bx.lo[1]}, {bx.hi[0], bx.lo[1]}, {bx.hi[0], bx.hi[1]}, {bx.lo[0], bx.hi[1]}} {
		u, v := f.local(bx.f.world(c[0], c[1]))
		lo, hi = [2]float64{math.Min(lo[0], u), math.Min(lo[1], v)}, [2]float64{math.Max(hi[0], u), math.Max(hi[1], v)}
	}
	return lo, hi
}

// cellAt is the grid's cell under a point, and whether the grid has one.
func (g *probeGrid) cellAt(x, y float64) (int, bool) {
	i, j := int(math.Floor((x-g.x0)/probeCell)), int(math.Floor((y-g.y0)/probeCell))
	if i < 0 || j < 0 || i >= g.n || j >= g.m {
		return -1, false
	}
	return i + j*g.n, true
}

// slant is an upright face off the axes, a box wide at least: its turn (its
// yaw folded into a quarter turn: a wall and the one square to it bound the
// same box), the axis of the turn pointing from it into the space before it
// and that way's sign, where it lies along that axis, and its ends along the
// other.
type slant struct {
	f      turn
	axis   int
	dir    float64
	at     float64
	t0, t1 float64
}

// slants are the faces off the axes of the solids beside a group's cells, at
// their height, the widest first.
func (g *probeGrid) slants(cells map[int]cellInfo) []slant {
	var out []slant
	for k := range g.near(cells) {
		vs := geom.Vertices(g.s.planes[k])
		for _, p := range g.s.planes[k] {
			if math.Abs(p.N[2]) > 0.01 {
				continue
			}
			a := math.Mod(math.Atan2(p.N[1], p.N[0])*180/math.Pi+405, 90) - 45
			if math.Abs(a) < minTurn {
				continue
			}
			if sl, ok := slantOf(p, vs, turnOf(a)); ok {
				out = append(out, sl)
			}
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if wa, wb := out[a].t1-out[a].t0, out[b].t1-out[b].t0; wa != wb {
			return wa > wb
		}
		if out[a].f.yaw != out[b].f.yaw {
			return out[a].f.yaw < out[b].f.yaw
		}
		return out[a].at*out[a].dir < out[b].at*out[b].dir || out[a].at*out[a].dir == out[b].at*out[b].dir && out[a].t0 < out[b].t0
	})
	return out
}

// slantOf is a face as a slant in a turn, if a box's width.
func slantOf(p geom.Plane, vs []mapfile.Vec3, f turn) (slant, bool) {
	ru, rv := f.local(-p.N[0], -p.N[1]) // out of the solid: the plane's normal points into it
	sl := slant{f: f, dir: 1, t0: math.Inf(1), t1: math.Inf(-1)}
	if math.Abs(rv) > math.Abs(ru) {
		sl.axis = 1
	}
	if (sl.axis == 0 && ru < 0) || (sl.axis == 1 && rv < 0) {
		sl.dir = -1
	}
	n := 0
	for _, v := range vs {
		if math.Abs(geom.Dot(p.N, v)-p.D) >= 0.01 {
			continue
		}
		u, w := f.local(v[0], v[1])
		q := [2]float64{u, w}
		sl.at += q[sl.axis]
		sl.t0, sl.t1 = math.Min(sl.t0, q[1-sl.axis]), math.Max(sl.t1, q[1-sl.axis])
		n++
	}
	if n == 0 || sl.t1-sl.t0 < minSide*probeCell {
		return slant{}, false
	}
	sl.at /= float64(n)
	return sl, true
}

// near are the solids within a cell of a group's cells, at their height.
func (g *probeGrid) near(cells map[int]cellInfo) map[int]bool {
	out := map[int]bool{}
	for c, ci := range cells {
		p := mapfile.Vec3{g.cx(c % g.n), g.cy(c / g.n), ci.floor + math.Min(40, (ci.top-ci.floor)/2)}
		for _, k := range g.s.tiles[[2]int{int(math.Floor(p[0] / solidTile)), int(math.Floor(p[1] / solidTile))}] {
			if !out[k] && g.s.boxes[k].Distance(p) <= probeCell {
				out[k] = true
			}
		}
	}
	return out
}

// hug is a box along a slant, turned with it, on cells free yet: in a grid
// of 8 laid from the face in the turn's axes, as far along as the face goes,
// a cell of it free where the group's cell under its middle is and no solid
// stands there at a body's height (the grid's cells sample only their own
// points, a turned one may fall in a wall's thickness). The box is the
// longest band 48 deep along the face; across to the wall facing it when
// that wall runs parallel within hugDepth (a corridor off the axes): deeper
// against a wall at another angle, its far side would cross that wall's face
// as an axis-aligned box crosses a slanted one. It says the group's cells
// the box's cells sampled.
func (g *probeGrid) hug(sl slant, cells map[int]cellInfo, free map[int]bool) (probeBox, []int, bool) {
	n := int((sl.t1 - sl.t0) / probeCell)
	sampled := make([][]int, n)
	for i := range n {
		for d := range hugDepth {
			var q [2]float64
			q[sl.axis], q[1-sl.axis] = sl.at+sl.dir*(float64(d)+0.5)*probeCell, sl.t0+(float64(i)+0.5)*probeCell
			x, y := sl.f.world(q[0], q[1])
			c, ok := g.cellAt(x, y)
			ci, in := cells[c]
			if !ok || !in || !free[c] || g.s.inside(mapfile.Vec3{x + sampleOff, y + sampleOff, ci.floor + math.Min(40, (ci.top-ci.floor)/2)}) {
				break
			}
			sampled[i] = append(sampled[i], c)
		}
	}
	r, ok := largestRect(n, minSide, func(i, d int) bool { return d < len(sampled[i]) }) // from the face out: a free cell has the ones nearer it free
	if !ok {
		return probeBox{}, nil, false
	}
	lo, hi := hugDepth, 0
	for i := r[0]; i < r[2]; i++ {
		lo, hi = min(lo, len(sampled[i])), max(hi, len(sampled[i]))
	}
	if lo < hugDepth && hi-lo <= 1 { // stopped by a wall parallel to the face
		r[3] = lo
	}
	bx := probeBox{f: sl.f}
	a, b := sl.at, sl.at+sl.dir*float64(r[3])*probeCell
	bx.rlo[sl.axis], bx.rhi[sl.axis] = math.Min(a, b), math.Max(a, b)
	bx.rlo[1-sl.axis], bx.rhi[1-sl.axis] = sl.t0+float64(r[0])*probeCell, sl.t0+float64(r[2])*probeCell
	var took []int
	for i := r[0]; i < r[2]; i++ {
		took = append(took, sampled[i][:r[3]]...)
	}
	return bx, took, true
}

// take marks taken the cells whose middles a box holds, and more.
func (g *probeGrid) take(bx probeBox, free map[int]bool, more []int) {
	for c, ok := range free {
		if ok && bx.holds(g.cx(c%g.n), g.cy(c/g.n)) {
			free[c] = false
		}
	}
	for _, c := range more {
		free[c] = false
	}
}
