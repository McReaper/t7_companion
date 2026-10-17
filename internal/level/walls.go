package level

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// WallsOp draws walls along lines on the ground, as a plan draws them, and
// cuts doors in them. Corners snap to the grid; a line within a few units
// of an axis runs exactly on it (a tracing error), and the two sides of a
// gap on nearly one line share it; an end within reach of another wall lands
// on it. Where walls meet nothing overlaps: a corner is cut at the mitre, a
// wall ending on another's side stops on its face, walls crossing split the
// second. Each door is the same: Width clear for the AI, nonColliding jambs
// each doubled by a clip_player brush narrowing it to Seen for the eye and
// players only, a lintel from Height to the top, a 2-unit weaponClip detail
// trim (zm_giant's doors: an opening with a lintel and a trim).
type WallsOp struct {
	Lines     []WallLine  `json:"lines"`
	Doors     []WallDoor  `json:"doors,omitempty"`
	Material  string      `json:"material"`
	Z         *[2]float64 `json:"z,omitempty"`         // bottom and top; none, 0 and Treyarch's 112
	Thickness float64     `json:"thickness,omitempty"` // none, Treyarch's 8
	Grid      *float64    `json:"grid,omitempty"`      // corners snap to it; none, 8; 0 leaves them
	Trim      string      `json:"trim,omitempty"`      // the doors' trim; none, a white painted metal
}

// WallLine is one wall's centreline: its corners, and whether it closes on
// its first (a kiosk's four walls).
type WallLine struct {
	Points [][2]float64 `json:"points"`
	Closed bool         `json:"closed,omitempty"`
}

// WallDoor is a door, cut in the wall nearest a point.
type WallDoor struct {
	At     [2]float64 `json:"at"`
	Width  float64    `json:"width,omitempty"`  // clear, the AI's way through; none, 72 (two abreast)
	Height float64    `json:"height,omitempty"` // none, 96
	Seen   float64    `json:"seen,omitempty"`   // what players see and pass; none, 56 (Treyarch's door)
}

const (
	wallOff   = 16  // how far off an axis, end to end, a line may stray and still run on it
	wallReach = 12  // how near another wall an end lands on it
	wallGap   = 150 // how wide a gap the two sides of which align
	doorReach = 48  // how near a wall a door's point must be
	trimDepth = 2
)

// wline is a wall's centreline as the op works it: its corners, closed or
// not, and each segment's kind: on an axis ('h', 'v') or not ('d').
type wline struct {
	pts    []pt2
	closed bool
	kinds  []byte
}

// wend names an end of a line: the line, and 0 its first point, 1 its last.
type wend struct{ line, end int }

func (o *WallsOp) check() (z0, z1, thick, grid float64, err error) {
	z0, z1, thick, grid = 0, 112, o.Thickness, 8
	if o.Z != nil {
		z0, z1 = o.Z[0], o.Z[1]
	}
	if thick == 0 {
		thick = 8
	}
	if o.Grid != nil {
		grid = *o.Grid
	}
	switch {
	case !word(o.Material):
		return 0, 0, 0, 0, fmt.Errorf("walls: material %q: one word, a material's name", o.Material)
	case o.Trim != "" && !word(o.Trim):
		return 0, 0, 0, 0, fmt.Errorf("walls: trim %q: one word, a material's name", o.Trim)
	case z1 <= z0:
		return 0, 0, 0, 0, fmt.Errorf("walls: z %v: its top above its bottom", [2]float64{z0, z1})
	case thick < 0 || grid < 0:
		return 0, 0, 0, 0, fmt.Errorf("walls: thickness %g, grid %g: neither below 0", thick, grid)
	case len(o.Lines) == 0:
		return 0, 0, 0, 0, fmt.Errorf("walls: no lines")
	}
	return z0, z1, thick, grid, nil
}

// walls runs a walls op: its pieces and doors, each a world brush of op i.
func (b *batch) walls(i int, o *WallsOp) error {
	z0, z1, thick, grid, err := o.check()
	if err != nil {
		return err
	}
	lines, err := wallLines(o.Lines)
	if err != nil {
		return err
	}
	joints := findJoints(lines)
	straighten(lines, grid)
	landJoints(lines, joints)
	pieces, cuts, err := cutDoors(lines, o.Doors, grid, z1-z0)
	if err != nil {
		return err
	}
	pieces = splitCrossings(pieces, thick)
	polys := wallPolys(pieces, thick)
	trim := o.Trim
	if trim == "" {
		trim = "t7_metal_painted_clean_white"
	}
	var parts []wpart
	for _, p := range polys {
		parts = append(parts, wpart{poly: p, z0: z0, z1: z1, material: o.Material, part: "wall"})
	}
	for k, c := range cuts {
		parts = append(parts, doorParts(c, k, thick, z0, z1, o.Material, trim)...)
	}
	return b.wallBrushes(i, parts)
}

func wallLines(in []WallLine) ([]*wline, error) {
	var out []*wline
	for k, l := range in {
		var pts []pt2
		for _, p := range l.Points {
			q := pt2{p[0], p[1]}
			if len(pts) == 0 || math.Hypot(q.x-pts[len(pts)-1].x, q.y-pts[len(pts)-1].y) > eps {
				pts = append(pts, q)
			}
		}
		if l.Closed && len(pts) > 2 && dist2(pts[0], pts[len(pts)-1]) > eps {
			pts = append(pts, pts[0])
		}
		if len(pts) < 2 || (l.Closed && len(pts) < 4) {
			return nil, fmt.Errorf("walls: line %d: two points or more (three, closed)", k)
		}
		out = append(out, &wline{pts: pts, closed: l.Closed && dist2(pts[0], pts[len(pts)-1]) <= eps})
	}
	return out, nil
}

func dist2(a, b pt2) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }

// nearSeg is how far p is from segment ab, the nearest point and its share of ab.
func nearSeg(p, a, b pt2) (float64, pt2, float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := dx*dx + dy*dy
	t := 0.0
	if l > 0 {
		t = math.Max(0, math.Min(1, ((p.x-a.x)*dx+(p.y-a.y)*dy)/l))
	}
	q := pt2{a.x + t*dx, a.y + t*dy}
	return dist2(p, q), q, t
}

// findJoints are the ends lying on another wall, read before anything moves.
func findJoints(lines []*wline) map[wend]int {
	out := map[wend]int{}
	for n, l := range lines {
		if l.closed {
			continue
		}
		for e, p := range []pt2{l.pts[0], l.pts[len(l.pts)-1]} {
			best, at := float64(wallReach), -1
			for m, o := range lines {
				if m == n {
					continue
				}
				for s := range len(o.pts) - 1 {
					if d, _, _ := nearSeg(p, o.pts[s], o.pts[s+1]); d < best {
						best, at = d, m
					}
				}
			}
			if at >= 0 {
				out[wend{n, e}] = at
			}
		}
	}
	return out
}

func segKind(a, b pt2) byte {
	dx, dy := math.Abs(b.x-a.x), math.Abs(b.y-a.y)
	switch {
	case dy <= wallOff && dx > dy:
		return 'h'
	case dx <= wallOff && dy > dx:
		return 'v'
	}
	return 'd'
}

// wrun is a stretch of one line's consecutive segments on one axis: the
// coordinate it keeps (1, y, for 'h'), its points, their mean and extent.
type wrun struct {
	line, c int
	idx     []int
	v       float64
	lo, hi  float64
}

func runsOf(n int, l *wline) []wrun {
	var out []wrun
	for s := 0; s < len(l.kinds); {
		e := s
		for e+1 < len(l.kinds) && l.kinds[e+1] == l.kinds[s] {
			e++
		}
		if k := l.kinds[s]; k != 'd' {
			r := wrun{line: n, c: 0}
			if k == 'h' {
				r.c = 1
			}
			r.lo, r.hi = math.Inf(1), math.Inf(-1)
			for j := s; j <= e+1; j++ {
				r.idx = append(r.idx, j)
				r.v += coord(l.pts[j], r.c) / float64(e-s+2)
				r.lo, r.hi = math.Min(r.lo, coord(l.pts[j], 1-r.c)), math.Max(r.hi, coord(l.pts[j], 1-r.c))
			}
			out = append(out, r)
		}
		s = e + 1
	}
	return out
}

func coord(p pt2, c int) float64 {
	if c == 0 {
		return p.x
	}
	return p.y
}

func setCoord(p *pt2, c int, v float64) {
	if c == 0 {
		p.x = v
	} else {
		p.y = v
	}
}

func snapTo(v, grid float64) float64 {
	if grid == 0 {
		return v
	}
	return grid * math.Round(v/grid)
}

// straighten puts each straight run exactly on one grid line, the runs
// facing each other across a gap on the same one, and every other corner
// coordinate on the grid.
func straighten(lines []*wline, grid float64) {
	var runs []wrun
	for n, l := range lines {
		l.kinds = l.kinds[:0]
		for s := range len(l.pts) - 1 {
			l.kinds = append(l.kinds, segKind(l.pts[s], l.pts[s+1]))
		}
		runs = append(runs, runsOf(n, l)...)
	}
	fixed := map[[3]int]bool{}
	for _, g := range runGroups(runs) {
		sum, n := 0.0, 0
		for _, r := range g {
			for _, j := range r.idx {
				sum += coord(lines[r.line].pts[j], r.c)
				n++
			}
		}
		v := snapTo(sum/float64(n), grid)
		for _, r := range g {
			for _, j := range r.idx {
				setCoord(&lines[r.line].pts[j], r.c, v)
				fixed[[3]int{r.line, j, r.c}] = true
			}
		}
	}
	for n, l := range lines {
		for j := range l.pts {
			for c := range 2 {
				if !fixed[[3]int{n, j, c}] {
					setCoord(&l.pts[j], c, snapTo(coord(l.pts[j], c), grid))
				}
			}
		}
	}
}

// runGroups gathers the runs that share a line: the same axis, within
// wallOff of each other, end to end across a gap of at most wallGap.
func runGroups(runs []wrun) [][]wrun {
	parent := make([]int, len(runs))
	for k := range parent {
		parent[k] = k
	}
	find := func(k int) int {
		for parent[k] != k {
			parent[k] = parent[parent[k]]
			k = parent[k]
		}
		return k
	}
	for x, r := range runs {
		for y := x + 1; y < len(runs); y++ {
			t := runs[y]
			gap := math.Max(r.lo, t.lo) - math.Min(r.hi, t.hi)
			if r.c == t.c && math.Abs(r.v-t.v) <= wallOff && gap >= 0 && gap <= wallGap {
				parent[find(x)] = find(y)
			}
		}
	}
	byRoot := map[int]int{}
	var out [][]wrun
	for k, r := range runs {
		root := find(k)
		at, ok := byRoot[root]
		if !ok {
			at = len(out)
			byRoot[root] = at
			out = append(out, nil)
		}
		out[at] = append(out[at], r)
	}
	return out
}

// landJoints puts each joint back on its wall: an end of a straight run
// slides along its own axis (so the run stays straight and on the grid),
// any other end goes to the nearest point. Three passes, as a joint moving
// drags its run, which may carry another.
func landJoints(lines []*wline, joints map[wend]int) {
	keys := make([]wend, 0, len(joints))
	for k := range joints {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b wend) int { return (a.line-b.line)*2 + a.end - b.end })
	for range 3 {
		for _, k := range keys {
			landJoint(lines, k, lines[joints[k]])
		}
	}
}

func landJoint(lines []*wline, k wend, host *wline) {
	l := lines[k.line]
	j, own := 0, 0
	if k.end == 1 {
		j, own = len(l.pts)-1, len(l.kinds)-1
	}
	p := l.pts[j]
	q, ok := pt2{}, false
	if kind := l.kinds[own]; kind != 'd' {
		keep := 0
		if kind == 'h' {
			keep = 1
		}
		q, ok = alongAxis(p, keep, host)
	}
	if !ok {
		best := math.Inf(1)
		for s := range len(host.pts) - 1 {
			if d, r, _ := nearSeg(p, host.pts[s], host.pts[s+1]); d < best {
				best, q = d, r
			}
		}
	}
	for c := range 2 {
		if v := coord(q, c); math.Abs(v-coord(p, c)) > 1e-9 {
			for _, idx := range runIndices(lines, k.line, j, c) {
				setCoord(&l.pts[idx], c, v)
			}
		}
	}
}

// alongAxis is where the line through p keeping coordinate c meets the
// host, nearest p and within reach.
func alongAxis(p pt2, c int, host *wline) (pt2, bool) {
	best, out := 3.0*wallReach, pt2{}
	for s := range len(host.pts) - 1 {
		a, b := host.pts[s], host.pts[s+1]
		den := coord(b, c) - coord(a, c)
		if math.Abs(den) < 1e-9 {
			continue
		}
		t := (coord(p, c) - coord(a, c)) / den
		if t < -1e-9 || t > 1+1e-9 {
			continue
		}
		q := pt2{a.x + t*(b.x-a.x), a.y + t*(b.y-a.y)}
		setCoord(&q, c, coord(p, c))
		if d := dist2(p, q); d < best {
			best, out = d, q
		}
	}
	return out, !math.IsInf(best, 1) && best < 3*wallReach
}

// runIndices are the points of line n's straight run along coordinate c
// through point j: they move together.
func runIndices(lines []*wline, n, j, c int) []int {
	for _, r := range runsOf(n, lines[n]) {
		if r.c == c && slices.Contains(r.idx, j) {
			return r.idx
		}
	}
	return []int{j}
}

// wpiece is a stretch of wall between cuts: its points, the line it is of,
// and whether it is a whole closed line (no free ends).
type wpiece struct {
	pts  []pt2
	line int
	ring bool
}

// doorCut is a door cut out of a wall: its first jamb's point, the wall's
// direction there, its width and height, and how wide players see it.
type doorCut struct {
	at, dir             pt2
	width, height, seen float64
}

// cutDoors cuts each door out of the segment nearest its point, its middle
// moved so its edges land on the grid on an axis wall.
func cutDoors(lines []*wline, doors []WallDoor, grid, height float64) ([]wpiece, []doorCut, error) {
	var pieces []wpiece
	for n, l := range lines {
		pieces = append(pieces, wpiece{pts: slices.Clone(l.pts), line: n, ring: l.closed})
	}
	var cuts []doorCut
	for k, d := range doors {
		w, h, seen := d.Width, d.Height, d.Seen
		if w == 0 {
			w = 72
		}
		if h == 0 {
			h = 96
		}
		if seen == 0 {
			seen = math.Min(56, w)
		}
		switch {
		case seen > w || w < 0 || seen < 0:
			return nil, nil, fmt.Errorf("walls: door %d: seen %g wider than its width %g", k, seen, w)
		case h <= 0 || h >= height:
			return nil, nil, fmt.Errorf("walls: door %d: height %g: under the walls' %g", k, h, height)
		}
		c, rest, err := cutOne(pieces, pt2{d.At[0], d.At[1]}, w, grid)
		if err != nil {
			return nil, nil, fmt.Errorf("walls: door %d at %v: %w", k, d.At, err)
		}
		pieces = rest
		c.height, c.seen = h, seen
		cuts = append(cuts, c)
	}
	return pieces, cuts, nil
}

func cutOne(pieces []wpiece, at pt2, w, grid float64) (doorCut, []wpiece, error) {
	best, bk, bs := math.Inf(1), -1, -1
	for k, p := range pieces {
		for s := range len(p.pts) - 1 {
			if d, _, _ := nearSeg(at, p.pts[s], p.pts[s+1]); d < best {
				best, bk, bs = d, k, s
			}
		}
	}
	if best > doorReach {
		return doorCut{}, nil, fmt.Errorf("no wall within %d", doorReach)
	}
	p := pieces[bk]
	a, b := p.pts[bs], p.pts[bs+1]
	L := dist2(a, b)
	u := pt2{(b.x - a.x) / L, (b.y - a.y) / L}
	_, q, _ := nearSeg(at, a, b)
	t := (q.x-a.x)*u.x + (q.y-a.y)*u.y
	if grid > 0 && (math.Abs(u.x) > 0.999 || math.Abs(u.y) > 0.999) { // an axis wall: the door's edges on the grid
		c, sign := 0, u.x
		if math.Abs(u.y) > 0.999 {
			c, sign = 1, u.y
		}
		mid := grid*math.Round((coord(a, c)+sign*t-w/2)/grid) + w/2
		t = (mid - coord(a, c)) * sign
	}
	margin := 4.0
	if L < w+2*margin {
		return doorCut{}, nil, fmt.Errorf("its wall is %.0f long: a %g door needs %g", L, w, w+2*margin)
	}
	t = math.Max(w/2+margin, math.Min(L-w/2-margin, t))
	j1 := pt2{a.x + u.x*(t-w/2), a.y + u.y*(t-w/2)}
	j2 := pt2{a.x + u.x*(t+w/2), a.y + u.y*(t+w/2)}
	var cut []wpiece
	if p.ring { // a ring cut once opens at the door: from the second jamb round to the first
		pts := append([]pt2{j2}, p.pts[bs+1:len(p.pts)-1]...)
		pts = append(pts, p.pts[:bs+1]...)
		cut = []wpiece{{pts: append(pts, j1), line: p.line}}
	} else {
		cut = []wpiece{
			{pts: append(slices.Clone(p.pts[:bs+1]), j1), line: p.line},
			{pts: append([]pt2{j2}, p.pts[bs+1:]...), line: p.line},
		}
	}
	rest := slices.Concat(pieces[:bk], cut, pieces[bk+1:])
	return doorCut{at: j1, dir: u, width: w}, rest, nil
}

// wpart is one brush the op makes: its outline on the ground, from z0 to
// z1, its material and contents, and what it is.
type wpart struct {
	poly               []pt2
	z0, z1             float64
	material, contents string
	part               string
}

// wallBrushes makes the parts world brushes of op i, and caulks the faces
// they hide of each other (a clip_player brush hides nothing: it draws
// nothing).
func (b *batch) wallBrushes(i int, parts []wpart) error {
	var drawn []*mapfile.Brush
	for _, p := range parts {
		poly := slices.Clone(p.poly)
		if signedArea(poly) < 0 {
			slices.Reverse(poly)
		}
		t := b.tile(p.material)
		text := withContents(prismBrush(poly, p.z0, p.z1, p.material, texNumbers(t.W, t.H)), p.contents)
		if err := b.made(i, EntityText("worldspawn", nil, text), true); err != nil {
			return fmt.Errorf("walls: %w", err)
		}
		u := b.units[len(b.units)-1]
		u.part = p.part
		if !strings.HasPrefix(p.material, "clip") {
			drawn = append(drawn, u.brush)
		}
	}
	for k, faces := range selfHiddenFaces(drawn) {
		if err := setFaces(drawn[k], faces, "caulk", b.tile("caulk")); err != nil {
			return fmt.Errorf("walls: %w", err)
		}
	}
	return nil
}
