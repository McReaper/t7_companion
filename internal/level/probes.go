package level

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// ProbesOp fills the space players reach with reflection probes, as a
// lighter snaps them by hand: from a point players stand (none, the map's
// info_player_start), the floor a player walks to, stairs, ledges dropped
// off and floors above included, and on a map with zones only what lies in
// them (the play space: out of it a player dies), is cut into boxes, the largest first, each from its floor to its
// ceiling, its sides on the walls' faces, no two overlapping (inner boxes
// never overlap: their blends do the joining). A lintel's lower ceiling
// parts two rooms, a storey keeps to itself, a space nobody reaches gets no
// probe. Blends are 3 (the entity's default, what zm_test's lighter kept),
// longer over floor no box holds.
type ProbesOp struct {
	At      *[3]float64       `json:"at,omitempty"`
	Replace bool              `json:"replace,omitempty"` // delete the probes already in the space it fills
	Height  float64           `json:"height,omitempty"`  // a box's top over open sky; none, 384
	Global  bool              `json:"global,omitempty"`  // also move the global probe into the largest box
	Keys    map[string]string `json:"keys,omitempty"`    // more keys on every probe
}

const (
	probeCell = 8.0
	stepUp    = 18.0 // the highest step a player climbs
	minSide   = 6    // cells: a box narrower than 48 is a doorway or a gap, left to the blends
)

// pspot is somewhere to stand in a cell: the floor's top and the ceiling over
// it, or the open-sky cap.
type pspot struct {
	floor, ceil float64
	sky         bool
}

// probeGrid is the ground sampled every 8 units over the solids' bounds, and
// a ring of empty cells around them: a cell where one stands has its eight
// neighbours on the grid.
type probeGrid struct {
	x0, y0 float64
	n, m   int
	spots  [][]pspot // cell (i + j*n) -> where one stands in it
	s      *solids
	height float64
}

func newProbeGrid(s *solids, height float64) *probeGrid {
	b := s.bounds
	g := &probeGrid{x0: math.Floor(b.Min[0]/probeCell)*probeCell - probeCell, y0: math.Floor(b.Min[1]/probeCell)*probeCell - probeCell, s: s, height: height}
	g.n = int((b.Max[0]-g.x0)/probeCell) + 2
	g.m = int((b.Max[1]-g.y0)/probeCell) + 2
	g.spots = make([][]pspot, g.n*g.m)
	for j := range g.m {
		for i := range g.n {
			g.spots[i+j*g.n] = g.standing(g.cx(i), g.cy(j))
		}
	}
	return g
}

func (g *probeGrid) cx(i int) float64 { return g.x0 + (float64(i)+0.5)*probeCell }
func (g *probeGrid) cy(j int) float64 { return g.y0 + (float64(j)+0.5)*probeCell }

// standing is where a player stands on the line through (x, y): on each
// solid's top with a player's room above it; the ceiling the next solid's
// bottom, or the open-sky cap (a roof higher than the cap counts as sky).
func (g *probeGrid) standing(x, y float64) []pspot {
	sp := g.s.spans(x, y)
	var out []pspot
	for k, s := range sp {
		p := pspot{s.hi, s.hi + g.height, true}
		if k+1 < len(sp) && sp[k+1].lo < p.ceil {
			p.ceil, p.sky = sp[k+1].lo, false
		}
		if p.ceil-p.floor >= clearance {
			out = append(out, p)
		}
	}
	return out
}

// pstate is a pspot of a cell reached from the start.
type pstate struct{ cell, k int }

// reach walks from the start to every pspot a player gets to: a step up of
// 18 at most or a drop off a ledge, room overhead, and a body's width (a cell spreads the
// walk only when its four neighbours stand too, so a slit narrower than a
// body stops it, and then to its diagonal ones as well, which a body there
// touches: a room's corners).
func (g *probeGrid) reach(at mapfile.Vec3) (map[pstate]bool, error) {
	i, j := int((at[0]-g.x0)/probeCell), int((at[1]-g.y0)/probeCell)
	if i < 0 || j < 0 || i >= g.n || j >= g.m {
		return nil, fmt.Errorf("probes: at %v is off the map", at)
	}
	start, best := -1, math.Inf(-1)
	for k, sp := range g.spots[i+j*g.n] {
		if sp.floor <= at[2]+stepUp && sp.floor > best {
			start, best = k, sp.floor
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("probes: no floor to stand on under %v", at)
	}
	seen := map[pstate]bool{{i + j*g.n, start}: true}
	queue := []pstate{{i + j*g.n, start}}
	for len(queue) > 0 {
		st := queue[0]
		queue = queue[1:]
		next := g.neighbours(st, sides)
		if len(next) < 4 {
			next = nil // against a wall or an edge: reached, the walk goes on only off a ledge
		} else {
			next = append(next, g.neighbours(st, diagonals)...)
		}
		for _, nb := range append(next, g.drops(st)...) {
			if !seen[nb] {
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return seen, nil
}

// inPlay keeps, of the spots reached, those in a zone's volume (a player
// standing there), and the ones within 3 cells of those (a doorway between
// two zones, a strip along a wall the volume stops short of); it says how
// many it left out.
func (g *probeGrid) inPlay(reached map[pstate]bool) (map[pstate]bool, int) {
	kept := map[pstate]bool{}
	var queue []pstate
	for st := range reached {
		sp := g.spots[st.cell][st.k]
		if g.s.inZone(mapfile.Vec3{g.cx(st.cell % g.n), g.cy(st.cell / g.n), sp.floor + 16}) {
			kept[st] = true
			queue = append(queue, st)
		}
	}
	for range 3 {
		var next []pstate
		for _, st := range queue {
			for _, nb := range append(g.neighbours(st, sides), g.neighbours(st, diagonals)...) {
				if reached[nb] && !kept[nb] {
					kept[nb] = true
					next = append(next, nb)
				}
			}
		}
		queue = next
	}
	return kept, len(reached) - len(kept)
}

var (
	sides     = [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	diagonals = [4][2]int{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}}
)

// neighbours are the spots next to a pstate's, one each way at most, a
// player can step up to or drop to, a body's height of room on the way.
func (g *probeGrid) neighbours(st pstate, ways [4][2]int) []pstate {
	var out []pstate
	for _, d := range ways {
		if nb, ok := g.next(st, d); ok {
			out = append(out, nb)
		}
	}
	return out
}

// drops are where a player lands off a ledge beside a pstate: the spot below
// and the one past it, a body's width clear of the ledge.
func (g *probeGrid) drops(st pstate) []pstate {
	var out []pstate
	for _, d := range sides {
		if nb, ok := g.next(st, d); ok && g.spots[nb.cell][nb.k].floor < g.spots[st.cell][st.k].floor-stepUp {
			out = append(out, nb)
			if past, ok := g.next(nb, d); ok {
				out = append(out, past)
			}
		}
	}
	return out
}

// next is the spot one cell off a pstate's, d away, a player can step up to
// or drop to, a body's height of room on the way.
func (g *probeGrid) next(st pstate, d [2]int) (pstate, bool) {
	here := g.spots[st.cell][st.k]
	c := st.cell + d[0] + d[1]*g.n
	for k, sp := range g.spots[c] {
		if sp.floor-here.floor <= stepUp && math.Min(sp.ceil, here.ceil)-math.Max(sp.floor, here.floor) >= clearance {
			return pstate{c, k}, true
		}
	}
	return pstate{}, false
}

// probeBox is a probe to place: its cells' rectangle, floor to ceiling,
// and its blends.
type probeBox struct {
	i0, j0, i1, j1   int // cells, ends exclusive
	lo, hi           mapfile.Vec3
	blendLo, blendHi mapfile.Vec3
	cells            map[int]cellInfo // its group's cells
}

// probes runs a probes op: the boxes, then a reflection_probe each.
func (b *batch) probes(i int, o *ProbesOp) error {
	height := o.Height
	if height == 0 {
		height = 384
	}
	if height < clearance {
		return fmt.Errorf("probes: height %g: a player's 72 at least", height)
	}
	if err := checkKeys("reflection_probe", o.Keys); err != nil {
		return fmt.Errorf("probes: %w", err)
	}
	at, err := b.probeStart(o)
	if err != nil {
		return err
	}
	s, notes := b.solidsOf(b.prefab)
	g := newProbeGrid(s, height)
	reached, err := g.reach(at)
	if err != nil {
		if len(notes) > 0 {
			return fmt.Errorf("%w (%s)", err, strings.Join(notes, "; "))
		}
		return err
	}
	if len(s.zones) > 0 {
		switch kept, out := g.inPlay(reached); {
		case len(kept) == 0:
			notes = append(notes, "probes: no zone's volume holds the floor players reach from there: all of it probed")
		case out > 0:
			reached = kept
			notes = append(notes, fmt.Sprintf("probes: kept to the zones: %d cells players reach lie outside every zone, where they can't live", out))
		}
	}
	boxes, left := g.boxes(reached)
	b.notes = append(b.notes, notes...)
	if left > 0 {
		b.notes = append(b.notes, fmt.Sprintf("probes: %d of the 8-unit cells reached are in no box: doorways, strips under 48 wide, the edges of walls off the axes (the blends and the global probe light them)", left))
	}
	b.replaceProbes(i, o.Replace, boxes)
	for k, bx := range boxes {
		if err := b.entity(i, probeEntity(s, bx, o.Keys)); err != nil {
			return err
		}
		b.units[len(b.units)-1].part = "probe " + strconv.Itoa(k)
	}
	if o.Global && len(boxes) > 0 {
		return b.moveGlobalProbe(i, boxes)
	}
	return nil
}

// probeStart is where the walk starts: at, or the info_player_start.
func (b *batch) probeStart(o *ProbesOp) (mapfile.Vec3, error) {
	if o.At != nil {
		return mapfile.Vec3(*o.At), nil
	}
	for _, u := range b.units {
		if u.state != deleted && u.ent != nil && u.ent.Class() == "info_player_start" {
			if p, ok := Origin(u.ent); ok {
				return mapfile.Vec3{p[0], p[1], p[2] + stepUp}, nil
			}
		}
	}
	return mapfile.Vec3{}, fmt.Errorf("probes: no info_player_start: give at, a point players stand on")
}

// probeEntity is a box's reflection_probe: its origin over its floor (out
// of any solid), its reach each way, Treyarch's defaults (4x, AO range 8,
// power and strength 1: Multiplayer_Lighting_Workflow.pdf).
func probeEntity(s *solids, bx probeBox, extra map[string]string) *EntityOp {
	c := Box{Min: bx.lo, Max: bx.hi, Ok: true}.Centre()
	c[2] = bx.lo[2] + math.Min(56, (bx.hi[2]-bx.lo[2])/2)
	for _, d := range []float64{0, 16, -16, 32, -32} { // out of a counter or a stair the middle falls in
		if p := (mapfile.Vec3{c[0] + d, c[1], c[2]}); !s.inside(p) {
			c = p
			break
		}
	}
	keys := map[string]string{"resolution": "4x", "ao_range": "8", "ao_strength": "1", "ao_power": "1", "box": "1",
		"size_min":   vecText(mapfile.Vec3{c[0] - bx.lo[0], c[1] - bx.lo[1], c[2] - bx.lo[2]}),
		"size_max":   vecText(mapfile.Vec3{bx.hi[0] - c[0], bx.hi[1] - c[1], bx.hi[2] - c[2]}),
		"blend_mins": vecText(bx.blendLo), "blend_maxs": vecText(bx.blendHi)}
	for k, v := range extra {
		keys[k] = v
	}
	return &EntityOp{Class: "reflection_probe", Origin: c, Keys: keys}
}

// replaceProbes deletes, with replace, the probes whose origin lies in a new
// box; without, it says how many it left there.
func (b *batch) replaceProbes(i int, replace bool, boxes []probeBox) {
	var in []*unit
	for _, u := range b.units {
		if u.state == deleted || u.ent == nil || u.ent.Class() != "reflection_probe" {
			continue
		}
		p, _ := Origin(u.ent)
		for _, bx := range boxes {
			if p[0] >= bx.lo[0] && p[0] <= bx.hi[0] && p[1] >= bx.lo[1] && p[1] <= bx.hi[1] && p[2] >= bx.lo[2] && p[2] <= bx.hi[2] {
				in = append(in, u)
				break
			}
		}
	}
	if len(in) > 0 && !replace {
		b.notes = append(b.notes, fmt.Sprintf("probes: %d probes already stand in the boxes, kept (replace deletes them)", len(in)))
		return
	}
	for _, u := range in { // one an earlier op of the batch made has no GUID yet: it is marked, not named
		u.state, u.op = deleted, i
	}
}

// moveGlobalProbe puts the global probe in the largest box: an info_null the
// volume_sun targets (Lighting_Probe_Workflow.pdf: not buried, not high over
// the map, where it sees only sky).
func (b *batch) moveGlobalProbe(i int, boxes []probeBox) error {
	big := boxes[0]
	for _, bx := range boxes[1:] {
		if (bx.hi[0]-bx.lo[0])*(bx.hi[1]-bx.lo[1]) > (big.hi[0]-big.lo[0])*(big.hi[1]-big.lo[1]) {
			big = bx
		}
	}
	at := Box{Min: big.lo, Max: big.hi, Ok: true}.Centre()
	at[2] = big.lo[2] + 56
	var sun *unit
	for _, u := range b.units {
		if u.state != deleted && u.ent != nil && u.ent.Class() == "volume_sun" {
			sun = u
		}
	}
	if sun == nil {
		b.notes = append(b.notes, "probes: no volume_sun: the global probe isn't moved")
		return nil
	}
	if err := b.entity(i, &EntityOp{Class: "info_null", Origin: at, Keys: map[string]string{"targetname": "global_probe"}}); err != nil {
		return err
	}
	b.units[len(b.units)-1].part = "global probe"
	target := "global_probe"
	return b.set(i, &SetOp{ID: hexOf(sun.ent.Guid()), Keys: map[string]*string{"target": &target}})
}
