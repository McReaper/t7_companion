package level

import (
	"math"
	"slices"
	"sort"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// The probes op's boxes. The cells reached are grouped by their storey (how
// many reached spots lie under theirs) and their ceiling (16-unit steps): a
// room's ceiling is one group, a doorway's lintel another, the floor above a
// third, and a stair that climbs through an opening to it a fourth, as
// nothing stands under the stair; open sky is one group, whatever the ground
// does under it. A cell has one spot a storey, so groups
// share no spot. In each group the largest rectangle of cells not yet taken
// goes first, at least 48 each way (narrower leftovers are the blends'),
// after a box turned along each wall off the axes; its sides then move onto
// the walls' faces, and it reaches from its lowest floor to its ceiling,
// never down into the storey under it.

// cellInfo is a reached cell's floor and ceiling, and the ceiling of the
// reached spot under it (-Inf: none).
type cellInfo struct {
	floor, top, below float64
}

// group is a storey and a ceiling.
type group struct{ storey, ceil int }

func (g *probeGrid) boxes(reached map[pstate]bool) ([]probeBox, int) {
	groups, keys := g.groups(reached)
	var out []probeBox
	for _, k := range keys {
		out = append(out, g.cut(groups[k])...)
	}
	for n := range out {
		for axis := range 2 {
			for _, dir := range []float64{-1, 1} {
				g.snapSide(n, out, axis, dir)
			}
		}
	}
	for n := range out {
		g.blends(n, out, reached)
	}
	left := 0
	for st := range reached {
		if g.holder(st.cell, g.spots(st.cell)[st.k].floor, out) < 0 {
			left++
		}
	}
	return out, left
}

// groups are the reached cells by storey and ceiling, and the groups in the
// order their boxes are cut: storey by storey, lowest ceiling first.
func (g *probeGrid) groups(reached map[pstate]bool) (map[group]map[int]cellInfo, []group) {
	groups := map[group]map[int]cellInfo{}
	for st := range reached {
		sp := g.spots(st.cell)[st.k]
		info, storey := cellInfo{sp.floor, sp.ceil, math.Inf(-1)}, 0
		for k := range st.k {
			if reached[pstate{st.cell, k}] {
				storey++
				info.below = g.spots(st.cell)[k].ceil
			}
		}
		key := group{storey, int(math.Round(sp.ceil / 16))}
		if sp.sky {
			key.ceil = math.MaxInt
		}
		if groups[key] == nil {
			groups[key] = map[int]cellInfo{}
		}
		groups[key][st.cell] = info
	}
	keys := make([]group, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].storey != keys[b].storey {
			return keys[a].storey < keys[b].storey
		}
		return keys[a].ceil < keys[b].ceil
	})
	return groups, keys
}

// cut cuts a group's cells into boxes: first along each face off the axes,
// widest first, a box turned with it; then the largest rectangle on the
// axes, and the next.
func (g *probeGrid) cut(cells map[int]cellInfo) []probeBox {
	free := map[int]bool{}
	for c := range cells {
		free[c] = true
	}
	var out []probeBox
	for _, sl := range g.slants(cells) {
		if bx, took, ok := g.hug(sl, cells, free); ok {
			g.take(bx, free, took)
			out = append(out, g.boxOf(bx, cells))
		}
	}
	for {
		r, ok := largestRect(g.n, g.m, func(i, j int) bool { return free[i+j*g.n] })
		if !ok {
			return out
		}
		bx := probeBox{rlo: [2]float64{g.x0 + float64(r[0])*probeCell, g.y0 + float64(r[1])*probeCell},
			rhi: [2]float64{g.x0 + float64(r[2])*probeCell, g.y0 + float64(r[3])*probeCell}}
		g.take(bx, free, nil)
		out = append(out, g.boxOf(bx, cells))
	}
}

// holder is the box holding a cell at a height, -1 for none.
func (g *probeGrid) holder(cell int, z float64, boxes []probeBox) int {
	x, y := g.cx(cell%g.n), g.cy(cell/g.n)
	for n, bx := range boxes {
		if bx.holds(x, y) && z >= bx.lo[2]-geom.Eps && z < bx.hi[2] {
			return n
		}
	}
	return -1
}

// largestRect is the largest rectangle of free cells of an n by m grid, at
// least minSide each way: the histogram method, row by row.
func largestRect(n, m int, free func(i, j int) bool) ([4]int, bool) {
	h := make([]int, n)
	best, r := 0, [4]int{}
	for j := range m {
		for i := range n {
			if free(i, j) {
				h[i]++
			} else {
				h[i] = 0
			}
		}
		type bar struct{ start, h int }
		var stack []bar
		for i := 0; i <= n; i++ {
			cur := 0
			if i < n {
				cur = h[i]
			}
			start := i
			for len(stack) > 0 && stack[len(stack)-1].h >= cur {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if w := i - top.start; top.h*w > best && min(top.h, w) >= minSide {
					best, r = top.h*w, [4]int{top.start, j - top.h + 1, i, j + 1}
				}
				start = top.start
			}
			stack = append(stack, bar{start, cur})
		}
	}
	return r, best > 0
}

// boxOf is a rectangle of cells as a box: lowest floor to highest ceiling
// of the cells it holds, but not under the ceiling of a storey below.
func (g *probeGrid) boxOf(bx probeBox, cells map[int]cellInfo) probeBox {
	lo := mapfile.Vec3{bx.rlo[0], bx.rlo[1], math.Inf(1)}
	hi := mapfile.Vec3{bx.rhi[0], bx.rhi[1], math.Inf(-1)}
	below := math.Inf(-1)
	for k, c := range cells {
		if bx.holds(g.cx(k%g.n), g.cy(k/g.n)) {
			lo[2], hi[2], below = math.Min(lo[2], c.floor), math.Max(hi[2], c.top), math.Max(below, c.below)
		}
	}
	lo[2] = math.Max(lo[2], below)
	bx.lo, bx.hi, bx.cells = lo, hi, cells
	return bx
}

// snapSide moves one side of a box onto the wall beyond it: from half a cell
// inside, along lines across the side at a body's height over the floor
// there, to where a solid starts; when most lines meet one within 12, the
// side goes to the farthest of those faces (a slanted wall's whole face
// covered: the probe's reach inside a wall costs nothing), but never past the
// middle of a wall it meets (its far face is another room's) nor into another
// box (a stair's steps are a wall to the room beside them). It works in the
// box's own axes.
func (g *probeGrid) snapSide(n int, boxes []probeBox, axis int, dir float64) {
	bx := &boxes[n]
	other := 1 - axis
	side := bx.lo[axis]
	if dir > 0 {
		side = bx.hi[axis]
	}
	hits, reach := g.sideHits(bx, axis, dir, side)
	const lines = 5
	if len(hits)*5 < lines*3 {
		return
	}
	far := math.Min(slices.Max(hits), reach)
	for m, o := range boxes { // the nearest box beyond the side, beside this one, at its height
		if m == n || o.lo[2] >= bx.hi[2] || o.hi[2] <= bx.lo[2] {
			continue
		}
		olo, ohi := o.extentIn(bx.f)
		if olo[other] >= bx.hi[other] || ohi[other] <= bx.lo[other] {
			continue
		}
		if dir > 0 && olo[axis] >= side-geom.Eps {
			far = math.Min(far, olo[axis]-side)
		} else if dir < 0 && ohi[axis] <= side+geom.Eps {
			far = math.Min(far, side-ohi[axis])
		}
	}
	if dir > 0 {
		bx.hi[axis] += far
	} else {
		bx.lo[axis] -= far
	}
}

// sideHits are, along five lines across a box's side, how far past it each
// meets a solid (within 12), and how far the side may go and stay inside the
// walls met: the nearest of their middles.
func (g *probeGrid) sideHits(bx *probeBox, axis int, dir, side float64) (hits []float64, reach float64) {
	const lines = 5
	other := 1 - axis
	reach = math.Inf(1)
	cnt := int(math.Round((bx.rhi[other] - bx.rlo[other]) / probeCell))
	for k := range lines {
		var q [2]float64 // the middle of the cell just inside the side, k fifths across it
		q[other] = bx.rlo[other] + (float64(cnt*(2*k+1)/(2*lines))+0.5)*probeCell
		q[axis] = side - dir*probeCell/2
		cell, _ := g.cellAt(bx.f.world(q[0], q[1]))
		c := bx.cells[cell]
		z := c.floor + math.Min(40, (c.top-c.floor)/2)
		solid := func(d float64) bool {
			var p [2]float64
			p[axis], p[other] = side+dir*d, q[other]
			x, y := bx.f.world(p[0], p[1])
			return g.s.inside(mapfile.Vec3{x, y, z})
		}
		for d := -probeCell / 2; d <= 12; d++ {
			if solid(d) {
				e := d
				for e < d+64 && solid(e) {
					e++
				}
				hits, reach = append(hits, d), math.Min(reach, (d+e)/2)
				break
			}
		}
	}
	return hits, reach
}

// blends sets a box's blend each way: 3 where a wall bounds most of a side,
// or a box holds the floor beyond it; else over that floor as far as no box
// holds it, 32 at most.
func (g *probeGrid) blends(n int, boxes []probeBox, reached map[pstate]bool) {
	bx := &boxes[n]
	open := func(c int, ok bool) bool { // someone stands there at the box's height
		if !ok {
			return false
		}
		for k, sp := range g.spots(c) {
			if reached[pstate{c, k}] && sp.floor >= bx.lo[2]-stepUp && sp.floor < bx.hi[2] {
				return true
			}
		}
		return false
	}
	cellOf := func(q [2]float64, step [2]float64, k float64) (int, bool) {
		return g.cellAt(bx.f.world(q[0]+step[0]*k, q[1]+step[1]*k))
	}
	set := func(lo bool, axis int, along [][2]float64, step [2]float64) {
		best, walled := 0, 0
		for _, q := range along {
			if !open(cellOf(q, step, 1)) {
				walled++
			}
			run := 0
			for ; run < 4; run++ {
				c, ok := cellOf(q, step, float64(run+1))
				if !open(c, ok) || g.holder(c, bx.lo[2], boxes) >= 0 {
					break
				}
			}
			best = max(best, run)
		}
		b := 3.0
		if walled*2 <= len(along) && best > 0 {
			b = math.Min(32, float64(best)*probeCell+4)
		}
		if lo {
			bx.blendLo[axis] = b
		} else {
			bx.blendHi[axis] = b
		}
	}
	var west, east, south, north [][2]float64 // the middles of the cells along each side, inside
	for v := bx.rlo[1] + probeCell/2; v < bx.rhi[1]; v += probeCell {
		west, east = append(west, [2]float64{bx.rlo[0] + probeCell/2, v}), append(east, [2]float64{bx.rhi[0] - probeCell/2, v})
	}
	for u := bx.rlo[0] + probeCell/2; u < bx.rhi[0]; u += probeCell {
		south, north = append(south, [2]float64{u, bx.rlo[1] + probeCell/2}), append(north, [2]float64{u, bx.rhi[1] - probeCell/2})
	}
	set(true, 0, west, [2]float64{-probeCell, 0})
	set(false, 0, east, [2]float64{probeCell, 0})
	set(true, 1, south, [2]float64{0, -probeCell})
	set(false, 1, north, [2]float64{0, probeCell})
	bx.blendLo[2], bx.blendHi[2] = 3, 3
}
