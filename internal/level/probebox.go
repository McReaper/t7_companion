package level

import (
	"math"
	"sort"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// The probes op's boxes. The cells reached are grouped by their storey (how
// many reached spots lie under theirs) and their ceiling (16-unit steps): a
// room's ceiling is one group, a doorway's lintel another, the floor above a
// third, and a stair that climbs through an opening to it a fourth, as
// nothing stands under the stair; open sky is one group, whatever the ground
// does under it. A cell has one spot a storey, so groups
// share no spot. In each group the largest rectangle of cells not yet taken
// goes first, at least 48 each way (narrower leftovers are the blends'); its
// sides then move onto the walls' faces, and it reaches from its lowest
// floor to its ceiling, never down into the storey under it.

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
		cells, free := groups[k], map[int]bool{}
		for c := range cells {
			free[c] = true
		}
		for {
			r, ok := g.largest(free)
			if !ok {
				break
			}
			g.take(r, free)
			out = append(out, g.boxOf(r, cells))
		}
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
		if g.holder(st.cell, g.spots[st.cell][st.k].floor, out) < 0 {
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
		sp := g.spots[st.cell][st.k]
		info, storey := cellInfo{sp.floor, sp.ceil, math.Inf(-1)}, 0
		for k := range st.k {
			if reached[pstate{st.cell, k}] {
				storey++
				info.below = g.spots[st.cell][k].ceil
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

// holder is the box holding a cell at a height, -1 for none.
func (g *probeGrid) holder(cell int, z float64, boxes []probeBox) int {
	i, j := cell%g.n, cell/g.n
	for n, bx := range boxes {
		if i >= bx.i0 && i < bx.i1 && j >= bx.j0 && j < bx.j1 && z >= bx.lo[2]-eps && z < bx.hi[2] {
			return n
		}
	}
	return -1
}

// largest is the largest rectangle of free cells, at least minSide each way:
// the histogram method, row by row.
func (g *probeGrid) largest(free map[int]bool) ([4]int, bool) {
	h := make([]int, g.n)
	best, r := 0, [4]int{}
	for j := range g.m {
		for i := range g.n {
			if free[i+j*g.n] {
				h[i]++
			} else {
				h[i] = 0
			}
		}
		type bar struct{ start, h int }
		var stack []bar
		for i := 0; i <= g.n; i++ {
			cur := 0
			if i < g.n {
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

func (g *probeGrid) take(r [4]int, free map[int]bool) {
	for j := r[1]; j < r[3]; j++ {
		for i := r[0]; i < r[2]; i++ {
			free[i+j*g.n] = false
		}
	}
}

// boxOf is a rectangle of cells as a box: lowest floor to highest ceiling,
// but not under the ceiling of a storey below.
func (g *probeGrid) boxOf(r [4]int, cells map[int]cellInfo) probeBox {
	lo := mapfile.Vec3{g.x0 + float64(r[0])*probeCell, g.y0 + float64(r[1])*probeCell, math.Inf(1)}
	hi := mapfile.Vec3{g.x0 + float64(r[2])*probeCell, g.y0 + float64(r[3])*probeCell, math.Inf(-1)}
	below := math.Inf(-1)
	for j := r[1]; j < r[3]; j++ {
		for i := r[0]; i < r[2]; i++ {
			if c, ok := cells[i+j*g.n]; ok {
				lo[2], hi[2], below = math.Min(lo[2], c.floor), math.Max(hi[2], c.top), math.Max(below, c.below)
			}
		}
	}
	lo[2] = math.Max(lo[2], below)
	return probeBox{i0: r[0], j0: r[1], i1: r[2], j1: r[3], lo: lo, hi: hi, cells: cells}
}

// snapSide moves one side of a box onto the wall beyond it: from half a cell
// inside, along lines across the side at a body's height over the floor
// there, to where a solid starts; when most lines meet one within 12, the
// side goes to the farthest of those faces (a slanted wall's whole face
// covered: the probe's reach inside a wall costs nothing), but never into
// another box (a stair's steps are a wall to the room beside them).
func (g *probeGrid) snapSide(n int, boxes []probeBox, axis int, dir float64) {
	bx := &boxes[n]
	other := 1 - axis
	side := bx.lo[axis]
	if dir > 0 {
		side = bx.hi[axis]
	}
	const lines = 5
	var hits []float64
	rect := [2][2]int{{bx.i0, bx.i1}, {bx.j0, bx.j1}}
	for k := range lines {
		var cell [2]int // the cell just inside the side, k fifths across it
		cell[other] = rect[other][0] + (rect[other][1]-rect[other][0])*(2*k+1)/(2*lines)
		cell[axis] = rect[axis][0]
		if dir > 0 {
			cell[axis] = rect[axis][1] - 1
		}
		c := bx.cells[cell[0]+cell[1]*g.n]
		t := []float64{g.cx(cell[0]), g.cy(cell[1])}[other]
		for d := -probeCell / 2; d <= 12; d++ {
			var p mapfile.Vec3
			p[axis], p[other], p[2] = side+dir*d, t, c.floor+math.Min(40, (c.top-c.floor)/2)
			if g.s.inside(p) {
				hits = append(hits, d)
				break
			}
		}
	}
	if len(hits)*5 < lines*3 {
		return
	}
	far := hits[0]
	for _, h := range hits[1:] {
		far = math.Max(far, h)
	}
	for m, o := range boxes { // the nearest box beyond the side, beside this one, at its height
		if m == n || o.lo[2] >= bx.hi[2] || o.hi[2] <= bx.lo[2] || o.lo[other] >= bx.hi[other] || o.hi[other] <= bx.lo[other] {
			continue
		}
		if dir > 0 && o.lo[axis] >= side-eps {
			far = math.Min(far, o.lo[axis]-side)
		} else if dir < 0 && o.hi[axis] <= side+eps {
			far = math.Min(far, side-o.hi[axis])
		}
	}
	if dir > 0 {
		bx.hi[axis] += far
	} else {
		bx.lo[axis] -= far
	}
}

// blends sets a box's blend each way: 3 where a wall bounds most of a side,
// or a box holds the floor beyond it; else over that floor as far as no box
// holds it, 32 at most.
func (g *probeGrid) blends(n int, boxes []probeBox, reached map[pstate]bool) {
	bx := &boxes[n]
	open := func(i, j int) bool { // someone stands there at the box's height
		for k, sp := range g.spots[i+j*g.n] {
			if reached[pstate{i + j*g.n, k}] && sp.floor >= bx.lo[2]-stepUp && sp.floor < bx.hi[2] {
				return true
			}
		}
		return false
	}
	set := func(lo bool, axis int, along [][2]int, di, dj int) {
		best, walled := 0, 0
		for _, c := range along {
			if !open(c[0]+di, c[1]+dj) {
				walled++
			}
			run := 0
			for i, j := c[0]+di, c[1]+dj; run < 4 && open(i, j) && g.holder(i+j*g.n, bx.lo[2], boxes) < 0; i, j = i+di, j+dj {
				run++
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
	var west, east, south, north [][2]int
	for j := bx.j0; j < bx.j1; j++ {
		west, east = append(west, [2]int{bx.i0, j}), append(east, [2]int{bx.i1 - 1, j})
	}
	for i := bx.i0; i < bx.i1; i++ {
		south, north = append(south, [2]int{i, bx.j0}), append(north, [2]int{i, bx.j1 - 1})
	}
	set(true, 0, west, -1, 0)
	set(false, 0, east, 1, 0)
	set(true, 1, south, 0, -1)
	set(false, 1, north, 0, 1)
	bx.blendLo[2], bx.blendHi[2] = 3, 3
}
