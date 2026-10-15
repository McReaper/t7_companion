package level

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// RoomOp builds a room around an interior box, the space a player walks in: a
// floor slab under it, a ceiling slab over it, a wall on each side not left
// open, its doors and windows left as holes the walls are built around (CSG
// Subtract would carve the map's volumes too). The faces its own pieces hide
// of each other (walls' tops and bottoms, butted ends) are caulk.
type RoomOp struct {
	Min      [3]float64 `json:"min"`
	Max      [3]float64 `json:"max"`
	Material string     `json:"material"`          // the walls'
	Floor    string     `json:"floor,omitempty"`   // the floor's; none, the walls'
	Ceiling  string     `json:"ceiling,omitempty"` // the ceiling's; none, the walls'
	Wall     float64    `json:"wall,omitempty"`    // wall thickness; none, Treyarch's 8
	Slab     float64    `json:"slab,omitempty"`    // floor and ceiling thickness; none, Treyarch's 16
	Open     []string   `json:"open,omitempty"`    // sides with no wall, +x … -y, top (no ceiling), bottom (no floor)
	Holes    []HoleOp   `json:"holes,omitempty"`
	Probe    *bool      `json:"probe,omitempty"` // a reflection_probe in its middle; none, true
}

// HoleOp is a door or a window in one of a room's walls.
type HoleOp struct {
	Side   string  `json:"side"`             // its wall: +x, -x, +y or -y
	At     float64 `json:"at"`               // its middle, in units along the wall from the interior's lower corner
	Width  float64 `json:"width,omitempty"`  // none: 56, Treyarch's door
	Height float64 `json:"height,omitempty"` // none: 96
	Sill   float64 `json:"sill,omitempty"`   // its bottom above the floor: 0 a door, 32 a window
}

// piece is one box of a room.
type piece struct {
	lo, hi   mapfile.Vec3
	material string
}

// wallSides are the sides a room has walls on, and the axis each runs along.
var wallSides = map[string]int{"-x": 1, "+x": 1, "-y": 0, "+y": 0}

// room runs a room op: its pieces, each a world brush of op i.
func (b *batch) room(i int, o *RoomOp) error {
	pieces, err := roomPieces(o)
	if err != nil {
		return fmt.Errorf("room: %w", err)
	}
	brushes := make([]*mapfile.Brush, len(pieces))
	for k, p := range pieces {
		t := b.tile(p.material)
		m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, boxBrush(Box{Min: p.lo, Max: p.hi, Ok: true}, p.material, "", texNumbers(t.W, t.H)))))
		if err != nil {
			return fmt.Errorf("room: %w", err)
		}
		brushes[k] = m.Entities[0].Brushes()[0]
	}
	for k, dirs := range selfHidden(brushes) {
		tx, err := newTexturing(&TextureOp{Faces: strings.Join(dirs, ","), Material: "caulk"}, b.tile)
		if err == nil {
			err = tx.brush(brushes[k])
		}
		if err != nil {
			return fmt.Errorf("room: %w", err)
		}
	}
	for _, br := range brushes {
		b.units = append(b.units, &unit{brush: br, world: true, state: created, op: i})
	}
	if o.Probe != nil && !*o.Probe {
		return nil
	}
	return b.entity(i, roomProbe(o))
}

// roomProbe is a room's reflection probe: in the middle of its inside, its
// radius reaching the corners, so the room's surfaces take their reflections
// and bounce light from inside it, not from the sky beyond its walls (a space
// with no probe of its own shows the sun through them: community, t7kb modme
// 0.60).
func roomProbe(o *RoomOp) *EntityOp {
	var c mapfile.Vec3
	var r float64
	for k := range 3 {
		c[k] = (o.Min[k] + o.Max[k]) / 2
		r += (o.Max[k] - o.Min[k]) * (o.Max[k] - o.Min[k]) / 4
	}
	return &EntityOp{Class: "reflection_probe", Origin: c, Keys: map[string]string{"radius": strconv.Itoa(int(math.Ceil(math.Sqrt(r)/8) * 8))}}
}

// roomPieces are a room's boxes: floor, ceiling, then each wall's.
func roomPieces(o *RoomOp) ([]piece, error) {
	r := *o
	if err := r.check(); err != nil {
		return nil, err
	}
	open := map[string]bool{}
	for _, s := range r.Open {
		open[s] = true
	}
	lo, hi, w := r.Min, r.Max, r.Wall
	ext := func(side string) float64 {
		if open[side] {
			return 0
		}
		return w
	}
	x0, x1, y0, y1 := lo[0]-ext("-x"), hi[0]+ext("+x"), lo[1]-ext("-y"), hi[1]+ext("+y")
	var out []piece
	if !open["bottom"] {
		out = append(out, piece{mapfile.Vec3{x0, y0, lo[2] - r.Slab}, mapfile.Vec3{x1, y1, lo[2]}, r.Floor})
	}
	if !open["top"] {
		out = append(out, piece{mapfile.Vec3{x0, y0, hi[2]}, mapfile.Vec3{x1, y1, hi[2] + r.Slab}, r.Ceiling})
	}
	walls := map[string][2]mapfile.Vec3{ // the x walls run the room's whole length, corners in; the y walls between them
		"-x": {{lo[0] - w, y0, lo[2]}, {lo[0], y1, hi[2]}},
		"+x": {{hi[0], y0, lo[2]}, {hi[0] + w, y1, hi[2]}},
		"-y": {{lo[0], lo[1] - w, lo[2]}, {hi[0], lo[1], hi[2]}},
		"+y": {{lo[0], hi[1], lo[2]}, {hi[0], hi[1] + w, hi[2]}},
	}
	for _, side := range []string{"-x", "+x", "-y", "+y"} {
		if !open[side] {
			out = append(out, r.wall(side, walls[side][0], walls[side][1])...)
		}
	}
	return out, nil
}

// check fills a room's defaults and refuses what can't be built.
func (r *RoomOp) check() error {
	for k := range 3 {
		if r.Max[k] <= r.Min[k] {
			return fmt.Errorf("max %v must exceed min %v on every axis", r.Max, r.Min)
		}
	}
	r.Floor, r.Ceiling = cmp.Or(r.Floor, r.Material), cmp.Or(r.Ceiling, r.Material)
	for _, m := range []string{r.Material, r.Floor, r.Ceiling} {
		if !word(m) {
			return fmt.Errorf("material %q: one word, a material's name", m)
		}
	}
	if r.Wall, r.Slab = cmp.Or(r.Wall, 8), cmp.Or(r.Slab, 16); r.Wall < 0 || r.Slab < 0 {
		return fmt.Errorf("wall %v, slab %v: thicknesses above 0", r.Wall, r.Slab)
	}
	for _, s := range r.Open {
		if _, ok := wallSides[s]; !ok && s != "top" && s != "bottom" {
			return fmt.Errorf("open %q: +x, -x, +y, -y, top or bottom", s)
		}
	}
	return r.checkHoles()
}

// checkHoles fills the holes' defaults and refuses one off its wall, or two
// side by side that meet.
func (r *RoomOp) checkHoles() error {
	bySide := map[string][]*HoleOp{}
	for k := range r.Holes {
		h := &r.Holes[k]
		h.Width, h.Height = cmp.Or(h.Width, 56), cmp.Or(h.Height, 96)
		ax, ok := wallSides[h.Side]
		switch {
		case !ok:
			return fmt.Errorf("hole %d: side %q: +x, -x, +y or -y", k, h.Side)
		case slices.Contains(r.Open, h.Side):
			return fmt.Errorf("hole %d: the %s side is open", k, h.Side)
		case h.Width < 0 || h.Height < 0 || h.Sill < 0:
			return fmt.Errorf("hole %d: width, height and sill can't be below 0", k)
		case h.At-h.Width/2 < -eps || h.At+h.Width/2 > r.Max[ax]-r.Min[ax]+eps:
			return fmt.Errorf("hole %d: %v wide at %v runs past its wall's %v of inside", k, h.Width, h.At, r.Max[ax]-r.Min[ax])
		case h.Sill+h.Height > r.Max[2]-r.Min[2]+eps:
			return fmt.Errorf("hole %d: a sill of %v and %v high is taller than the room's %v", k, h.Sill, h.Height, r.Max[2]-r.Min[2])
		}
		bySide[h.Side] = append(bySide[h.Side], h)
	}
	for side, hs := range bySide {
		sort.Slice(hs, func(i, j int) bool { return hs[i].At < hs[j].At })
		for k := 1; k < len(hs); k++ {
			if hs[k].At-hs[k].Width/2 < hs[k-1].At+hs[k-1].Width/2+eps {
				return fmt.Errorf("two holes in the %s wall meet: leave wall between them, or make one hole", side)
			}
		}
	}
	return nil
}

// wall is one wall's pieces: whole, or in strips around its holes, a piece
// under each hole (a sill) and over it (a lintel).
func (r *RoomOp) wall(side string, lo, hi mapfile.Vec3) []piece {
	ax := wallSides[side]
	var hs []HoleOp
	for _, h := range r.Holes {
		if h.Side == side {
			hs = append(hs, h)
		}
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].At < hs[j].At })
	span := func(a, b, z0, z1 float64) (piece, bool) {
		p := piece{lo, hi, r.Material}
		p.lo[ax], p.hi[ax], p.lo[2], p.hi[2] = a, b, z0, z1
		return p, b-a > eps && z1-z0 > eps
	}
	var out []piece
	add := func(p piece, ok bool) {
		if ok {
			out = append(out, p)
		}
	}
	at := lo[ax]
	for _, h := range hs {
		u0, u1 := r.Min[ax]+h.At-h.Width/2, r.Min[ax]+h.At+h.Width/2
		z0, z1 := r.Min[2]+h.Sill, math.Min(r.Min[2]+h.Sill+h.Height, hi[2])
		add(span(at, u0, lo[2], hi[2]))
		add(span(u0, u1, lo[2], z0))
		add(span(u0, u1, z1, hi[2]))
		at = u1
	}
	add(span(at, hi[ax], lo[2], hi[2]))
	return out
}
