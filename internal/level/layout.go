package level

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// LayoutOp builds a storey from its plan: named rooms, rectangles on the
// ground that meet along their edges. A wall stands centred on every edge,
// one where two rooms meet; each room gets a floor and a ceiling slab between
// its walls (an outdoor one no ceiling) and a reflection probe; openings are
// holes the walls are built around (CSG Subtract would carve the map's
// volumes too). A wall's sides take the material of the room each looks
// into, or the exterior's; the faces its pieces hide of each other are caulk.
type LayoutOp struct {
	Rooms    []PlanRoom `json:"rooms"`
	Openings []Opening  `json:"openings,omitempty"`
	Material string     `json:"material"`           // the walls'
	Floor    string     `json:"floor,omitempty"`    // the floors'; none, the walls'
	Ceiling  string     `json:"ceiling,omitempty"`  // the ceilings'; none, the walls'
	Exterior string     `json:"exterior,omitempty"` // the outer walls' outside faces; none, the walls'
	Z        float64    `json:"z,omitempty"`        // the floors' top
	Height   float64    `json:"height,omitempty"`   // floor to ceiling; none, Treyarch's 112
	Wall     float64    `json:"wall,omitempty"`     // wall thickness; none, Treyarch's 8
	Slab     float64    `json:"slab,omitempty"`     // floor and ceiling thickness; none, Treyarch's 16
	Probes   *bool      `json:"probes,omitempty"`   // a reflection_probe in each room; none, true
}

// PlanRoom is one room of a layout: its rectangle, from wall centre to wall
// centre, and its own materials (none, the layout's; an outdoor room's walls,
// the exterior's).
type PlanRoom struct {
	Name     string     `json:"name"`
	Min      [2]float64 `json:"min"`
	Max      [2]float64 `json:"max"`
	Material string     `json:"material,omitempty"`
	Floor    string     `json:"floor,omitempty"`
	Ceiling  string     `json:"ceiling,omitempty"`
	Outdoor  bool       `json:"outdoor,omitempty"` // no ceiling: a courtyard
}

// Opening is a door or a window in a room's wall: to the room it opens on,
// or on one of its sides.
type Opening struct {
	Room   string   `json:"room"`
	To     string   `json:"to,omitempty"`
	Side   string   `json:"side,omitempty"`   // +x, -x, +y or -y
	At     *float64 `json:"at,omitempty"`     // its middle, the world x or y along the wall; none, the middle of the wall shared, or of the side
	Width  float64  `json:"width,omitempty"`  // none: 56
	Height float64  `json:"height,omitempty"` // none: 96 a door, 64 a window
	Sill   float64  `json:"sill,omitempty"`   // its bottom above the floor: 0 a door, 32 a window
}

// piece is one box a layout builds: its material, the faces that take
// another (a wall's sides), and what it is ("hall floor", "hall|kitchen wall").
type piece struct {
	lo, hi   mapfile.Vec3
	material string
	faces    [][2]string // a face's direction and its material
	part     string
}

// seg is a stretch of wall on one line of the plan, between two of the
// line's breakpoints (where a room's edge on it starts or ends, so where a
// wall across it meets it), with the same rooms either side.
type seg struct {
	along       int     // the axis it runs along
	c           float64 // the line's coordinate across it
	lo, hi      float64
	minus, plus int  // the rooms on its - and + sides, -1 the outside
	first, last bool // the ends of a wall along the line
	holes       []hole
}

// hole is an opening placed in a seg: along it, and up.
type hole struct {
	u0, u1, z0, z1 float64
	floor          string // a door's threshold takes its room's floor material
}

// layout runs a layout op: its pieces, each a world brush of op i, then a
// probe in each room.
func (b *batch) layout(i int, o *LayoutOp) error {
	l := *o // its defaults filled, the op left as sent
	l.Rooms, l.Openings = slices.Clone(o.Rooms), slices.Clone(o.Openings)
	pieces, err := l.pieces()
	if err != nil {
		return fmt.Errorf("layout: %w", err)
	}
	brushes := make([]*mapfile.Brush, len(pieces))
	for k, p := range pieces {
		if brushes[k], err = b.pieceBrush(p); err != nil {
			return fmt.Errorf("layout: %w", err)
		}
	}
	for k, dirs := range selfHidden(brushes) {
		if err := b.retexture(brushes[k], strings.Join(dirs, ","), "caulk"); err != nil {
			return fmt.Errorf("layout: %w", err)
		}
	}
	for k, br := range brushes {
		b.units = append(b.units, &unit{brush: br, world: true, state: created, op: i, part: pieces[k].part})
	}
	if l.Probes != nil && !*l.Probes {
		return nil
	}
	for _, r := range l.Rooms {
		if err := b.entity(i, l.probe(r)); err != nil {
			return err
		}
		b.units[len(b.units)-1].part = r.Name + " probe"
	}
	return nil
}

// pieceBrush is a piece's box brush, its faces textured.
func (b *batch) pieceBrush(p piece) (*mapfile.Brush, error) {
	t := b.tile(p.material)
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, boxBrush(Box{Min: p.lo, Max: p.hi, Ok: true}, p.material, "", texNumbers(t.W, t.H)))))
	if err != nil {
		return nil, err
	}
	br := m.Entities[0].Brushes()[0]
	for _, f := range p.faces {
		if f[1] != p.material {
			if err := b.retexture(br, f[0], f[1]); err != nil {
				return nil, err
			}
		}
	}
	return br, nil
}

// retexture lays a material on a brush's faces, at its tile.
func (b *batch) retexture(br *mapfile.Brush, faces, material string) error {
	tx, err := newTexturing(&TextureOp{Faces: faces, Material: material}, b.tile)
	if err != nil {
		return err
	}
	return tx.brush(br)
}

// pieces are the layout's boxes: each room's floor and ceiling, then the
// walls, line by line.
func (l *LayoutOp) pieces() ([]piece, error) {
	idx, err := l.check()
	if err != nil {
		return nil, err
	}
	segs := l.segs()
	for k := range l.Openings {
		if err := l.place(k, idx, segs); err != nil {
			return nil, err
		}
	}
	var out []piece
	for _, r := range l.Rooms {
		out = append(out, l.slabs(r)...)
	}
	for _, s := range segs {
		slices.SortFunc(s.holes, func(a, b hole) int { return cmp.Compare(a.u0, b.u0) })
		for k := 1; k < len(s.holes); k++ {
			if s.holes[k].u0 < s.holes[k-1].u1+eps {
				return nil, fmt.Errorf("two openings in the %s meet: leave wall between them, or make one wider", l.part(s))
			}
		}
		out = append(out, l.wall(s)...)
	}
	return out, nil
}

// check fills the layout's defaults, its rooms' too, and refuses what can't
// be built; it answers each room's index by name.
func (l *LayoutOp) check() (map[string]int, error) {
	if len(l.Rooms) == 0 {
		return nil, fmt.Errorf("no rooms: give each a name, min and max")
	}
	l.Floor, l.Ceiling, l.Exterior = cmp.Or(l.Floor, l.Material), cmp.Or(l.Ceiling, l.Material), cmp.Or(l.Exterior, l.Material)
	if l.Height, l.Wall, l.Slab = cmp.Or(l.Height, 112), cmp.Or(l.Wall, 8), cmp.Or(l.Slab, 16); l.Height < 0 || l.Wall < 0 || l.Slab < 0 {
		return nil, fmt.Errorf("height %v, wall %v, slab %v: sizes above 0", l.Height, l.Wall, l.Slab)
	}
	mats := []string{l.Material, l.Floor, l.Ceiling, l.Exterior}
	idx := map[string]int{}
	for i := range l.Rooms {
		r := &l.Rooms[i]
		walls := l.Material
		if r.Outdoor { // a courtyard's walls are facades
			walls = l.Exterior
		}
		r.Material, r.Floor, r.Ceiling = cmp.Or(r.Material, walls), cmp.Or(r.Floor, l.Floor), cmp.Or(r.Ceiling, l.Ceiling)
		mats = append(mats, r.Material, r.Floor, r.Ceiling)
		if _, dup := idx[r.Name]; dup || !word(r.Name) || strings.Contains(r.Name, "|") {
			return nil, fmt.Errorf("room %d: name %q: one word, each room its own", i, r.Name)
		}
		idx[r.Name] = i
		for k := range 2 {
			if r.Max[k]-r.Min[k] <= l.Wall+eps {
				return nil, fmt.Errorf("room %s: %v to %v leaves no inside between walls %v thick", r.Name, r.Min, r.Max, l.Wall)
			}
		}
	}
	for _, m := range mats {
		if !word(m) {
			return nil, fmt.Errorf("material %q: one word, a material's name", m)
		}
	}
	return idx, l.checkOverlaps()
}

// checkOverlaps refuses two rooms that share more than an edge.
func (l *LayoutOp) checkOverlaps() error {
	for i, a := range l.Rooms {
		for _, b := range l.Rooms[i+1:] {
			if math.Min(a.Max[0], b.Max[0])-math.Max(a.Min[0], b.Min[0]) > eps && math.Min(a.Max[1], b.Max[1])-math.Max(a.Min[1], b.Min[1]) > eps {
				return fmt.Errorf("rooms %s and %s overlap: rooms meet along their edges", a.Name, b.Name)
			}
		}
	}
	return nil
}

// segs are the plan's stretches of wall: on every line a room's edge lies
// on, cut where an edge on it starts or ends.
func (l *LayoutOp) segs() []*seg {
	type edge struct {
		lo, hi float64
		room   int
		plus   bool // the room lies on the line's + side
	}
	type line struct {
		along int
		c     float64
	}
	lines := map[line][]edge{}
	for i, r := range l.Rooms {
		for along := range 2 {
			lo, hi := line{along, r.Min[1-along]}, line{along, r.Max[1-along]}
			lines[lo] = append(lines[lo], edge{r.Min[along], r.Max[along], i, true})
			lines[hi] = append(lines[hi], edge{r.Min[along], r.Max[along], i, false})
		}
	}
	keys := slices.SortedFunc(maps.Keys(lines), func(a, b line) int { return cmp.Or(cmp.Compare(a.along, b.along), cmp.Compare(a.c, b.c)) })
	var out []*seg
	for _, k := range keys {
		var pts []float64
		for _, e := range lines[k] {
			pts = append(pts, e.lo, e.hi)
		}
		slices.Sort(pts)
		pts = slices.Compact(pts)
		var prev *seg
		for j := 1; j < len(pts); j++ {
			s := &seg{along: k.along, c: k.c, lo: pts[j-1], hi: pts[j], minus: -1, plus: -1}
			for _, e := range lines[k] {
				switch {
				case e.lo > s.lo+eps || e.hi < s.hi-eps:
				case e.plus:
					s.plus = e.room
				default:
					s.minus = e.room
				}
			}
			if s.minus < 0 && s.plus < 0 {
				prev = closeRun(prev)
				continue
			}
			s.first = prev == nil
			out, prev = append(out, s), s
		}
		closeRun(prev)
	}
	return out
}

// closeRun marks the last stretch of a wall along its line.
func closeRun(s *seg) *seg {
	if s != nil {
		s.last = true
	}
	return nil
}

// wallRef is the line an opening's wall stands on: the room on its + side
// or not, the room across (-1: whichever), and the span to centre it on.
type wallRef struct {
	along  int
	c      float64
	plus   bool
	to     int
	lo, hi float64
}

// layoutSides are a room's sides: the axis its wall runs along, whether it
// is the room's max edge, and so whether the room lies on its + side.
var layoutSides = map[string]struct {
	along     int
	max, plus bool
}{"-x": {1, false, true}, "+x": {1, true, false}, "-y": {0, false, true}, "+y": {0, true, false}}

// wallOf is the wall an opening is in.
func (l *LayoutOp) wallOf(k int, o *Opening, idx map[string]int) (wallRef, error) {
	r := l.Rooms[idx[o.Room]]
	switch {
	case (o.To == "") == (o.Side == ""):
		return wallRef{}, fmt.Errorf("opening %d: to (the room it opens on) or side, one of them", k)
	case o.Side != "":
		s, ok := layoutSides[o.Side]
		if !ok {
			return wallRef{}, fmt.Errorf("opening %d: side %q: +x, -x, +y or -y", k, o.Side)
		}
		c := r.Min[1-s.along]
		if s.max {
			c = r.Max[1-s.along]
		}
		return wallRef{s.along, c, s.plus, -1, r.Min[s.along], r.Max[s.along]}, nil
	}
	t, ok := idx[o.To]
	if !ok {
		return wallRef{}, fmt.Errorf("opening %d: no room %q", k, o.To)
	}
	to := l.Rooms[t]
	for along := range 2 {
		n := 1 - along
		lo, hi := math.Max(r.Min[along], to.Min[along]), math.Min(r.Max[along], to.Max[along])
		switch {
		case hi-lo <= eps:
		case math.Abs(r.Max[n]-to.Min[n]) <= eps:
			return wallRef{along, r.Max[n], false, t, lo, hi}, nil
		case math.Abs(r.Min[n]-to.Max[n]) <= eps:
			return wallRef{along, r.Min[n], true, t, lo, hi}, nil
		}
	}
	return wallRef{}, fmt.Errorf("opening %d: rooms %s and %s share no wall", k, o.Room, o.To)
}

// place fills an opening's defaults and puts it in the stretch of wall it
// lies in, between the walls that meet that one.
func (l *LayoutOp) place(k int, idx map[string]int, segs []*seg) error {
	o := &l.Openings[k]
	r, ok := idx[o.Room]
	switch {
	case !ok:
		return fmt.Errorf("opening %d: no room %q", k, o.Room)
	case o.Width < 0 || o.Height < 0 || o.Sill < 0:
		return fmt.Errorf("opening %d: width, height and sill can't be below 0", k)
	}
	o.Width = cmp.Or(o.Width, 56)
	if o.Height == 0 {
		o.Height = 96
		if o.Sill > 0 {
			o.Height = 64
		}
	}
	if o.Sill+o.Height > l.Height+eps {
		return fmt.Errorf("opening %d: a sill of %v and %v high is taller than the storey's %v", k, o.Sill, o.Height, l.Height)
	}
	w, err := l.wallOf(k, o, idx)
	if err != nil {
		return err
	}
	at := (w.lo + w.hi) / 2
	if o.At != nil {
		at = *o.At
	}
	h := hole{u0: at - o.Width/2, u1: at + o.Width/2, z0: l.Z + o.Sill, z1: l.Z + o.Sill + o.Height}
	if o.Sill == 0 {
		h.floor = l.Rooms[r].Floor
	}
	var stretches []string
	for _, s := range segs {
		mine, across := s.minus, s.plus
		if w.plus {
			mine, across = across, mine
		}
		if s.along != w.along || math.Abs(s.c-w.c) > eps || mine != r || (w.to >= 0 && across != w.to) {
			continue
		}
		if a, b := s.lo+l.Wall/2, s.hi-l.Wall/2; h.u0 < a-eps || h.u1 > b+eps {
			stretches = append(stretches, fmt.Sprintf("%v to %v", a, b))
			continue
		}
		s.holes = append(s.holes, h)
		return nil
	}
	return fmt.Errorf("opening %d: %v wide at %v runs past the stretch of wall it is in, between the walls that meet it: %s", k, o.Width, at, strings.Join(stretches, ", "))
}

// slabs are a room's floor, and its ceiling unless it is outdoor: between
// its walls.
func (l *LayoutOp) slabs(r PlanRoom) []piece {
	in := l.Wall / 2
	lo, hi := mapfile.Vec3{r.Min[0] + in, r.Min[1] + in, l.Z - l.Slab}, mapfile.Vec3{r.Max[0] - in, r.Max[1] - in, l.Z}
	out := []piece{{lo: lo, hi: hi, material: r.Floor, part: r.Name + " floor"}}
	if !r.Outdoor {
		lo[2], hi[2] = l.Z+l.Height, l.Z+l.Height+l.Slab
		out = append(out, piece{lo: lo, hi: hi, material: r.Ceiling, part: r.Name + " ceiling"})
	}
	return out
}

// wall is a stretch's pieces, through the floor and ceiling slabs: whole, or
// in strips around its holes, a sill under each (a door's threshold, of its
// room's floor) and a lintel over it. The walls along x take the corners:
// they run on past a wall's end by half a wall, their end showing the room
// beyond or the outside; the walls along y stop against them. A wall's top
// is the exterior's when it stands by the outside or an outdoor room.
func (l *LayoutOp) wall(s *seg) []piece {
	e0, e1 := s.lo+l.Wall/2, s.hi-l.Wall/2
	if s.along == 0 {
		e0, e1 = s.lo, s.hi
		if s.first {
			e0 -= l.Wall / 2
		}
		if s.last {
			e1 += l.Wall / 2
		}
	}
	bottom, top := l.Z-l.Slab, l.Z+l.Height+l.Slab
	axis := string("yx"[s.along])
	side := func(r int) string {
		if r < 0 {
			return l.Exterior
		}
		return l.Rooms[r].Material
	}
	roof := l.Material
	if l.open(s.minus) || l.open(s.plus) {
		roof = l.Exterior
	}
	part := l.part(s)
	var out []piece
	add := func(a, b, z0, z1 float64, material string) {
		if b-a <= eps || z1-z0 <= eps {
			return
		}
		p := piece{material: material, faces: [][2]string{{"-" + axis, side(s.minus)}, {"+" + axis, side(s.plus)}}, part: part}
		if z1 >= top-eps {
			p.faces = append(p.faces, [2]string{"+z", roof})
		}
		if s.along == 0 && s.first && a <= e0+eps {
			p.faces = append(p.faces, [2]string{"-x", l.beyond(e0-eps, s.c)})
		}
		if s.along == 0 && s.last && b >= e1-eps {
			p.faces = append(p.faces, [2]string{"+x", l.beyond(e1+eps, s.c)})
		}
		p.lo[s.along], p.hi[s.along] = a, b
		p.lo[1-s.along], p.hi[1-s.along] = s.c-l.Wall/2, s.c+l.Wall/2
		p.lo[2], p.hi[2] = z0, z1
		out = append(out, p)
	}
	at := e0
	for _, h := range s.holes {
		add(at, h.u0, bottom, top, l.Material)
		add(h.u0, h.u1, bottom, h.z0, cmp.Or(h.floor, l.Material))
		add(h.u0, h.u1, h.z1, top, l.Material)
		at = h.u1
	}
	add(at, e1, bottom, top, l.Material)
	return out
}

// open tells the outside (-1), or an outdoor room.
func (l *LayoutOp) open(r int) bool { return r < 0 || l.Rooms[r].Outdoor }

// beyond is the material of what a point of the plan is in: a room's walls',
// or the exterior.
func (l *LayoutOp) beyond(x, y float64) string {
	for _, r := range l.Rooms {
		if r.Min[0] < x && x < r.Max[0] && r.Min[1] < y && y < r.Max[1] {
			return r.Material
		}
	}
	return l.Exterior
}

// part names a stretch of wall by the rooms either side of it.
func (l *LayoutOp) part(s *seg) string {
	var names []string
	for _, r := range []int{s.minus, s.plus} {
		if r >= 0 {
			names = append(names, l.Rooms[r].Name)
		}
	}
	return strings.Join(names, "|") + " wall"
}

// probe is a room's reflection probe: in the middle of its inside, its
// radius reaching the corners, so the room's surfaces take their reflections
// and bounce light from inside it, not from the sky beyond its walls (a space
// with no probe of its own shows the sun through them: community, t7kb modme
// 0.60).
func (l *LayoutOp) probe(r PlanRoom) *EntityOp {
	in := l.Wall / 2
	lo, hi := mapfile.Vec3{r.Min[0] + in, r.Min[1] + in, l.Z}, mapfile.Vec3{r.Max[0] - in, r.Max[1] - in, l.Z + l.Height}
	var c mapfile.Vec3
	var reach float64
	for k := range 3 {
		c[k] = (lo[k] + hi[k]) / 2
		reach += (hi[k] - lo[k]) * (hi[k] - lo[k]) / 4
	}
	return &EntityOp{Class: "reflection_probe", Origin: c, Keys: map[string]string{"radius": strconv.Itoa(int(math.Ceil(math.Sqrt(reach)/8) * 8))}}
}
