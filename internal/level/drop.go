package level

import (
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// ground is what a point can stand on, as the batch leaves the map: its
// world brushes and func_groups' (convex, exact along a vertical line) and
// its mesh and curve patches (their control grids, in triangles: terrain).
// Sky brushes and brush entities (volumes, triggers, doors) are not ground.
type ground struct {
	solids  [][]plane
	boxes   []Box // each solid's bounds
	patches [][3]mapfile.Vec3
}

// groundOf reads the batch's ground.
func (b *batch) groundOf() *ground {
	g := &ground{}
	for _, u := range b.units {
		switch {
		case u.state == deleted:
		case u.world:
			g.add(u.brush)
		case u.ent.Class() == "func_group":
			for _, br := range u.ent.Brushes() {
				g.add(br)
			}
		}
	}
	return g
}

func (g *ground) add(br *mapfile.Brush) {
	switch br.Kind() {
	case mapfile.KindFaces:
		faces, err := br.Faces()
		if err != nil || len(faces) < 4 {
			return
		}
		planes := make([]plane, len(faces))
		for i, f := range faces {
			if strings.EqualFold(f.Material, "sky") {
				return
			}
			planes[i] = planeOf(f.Points)
		}
		var bx Box
		for _, v := range vertices(planes) {
			bx.Add(v)
		}
		g.solids, g.boxes = append(g.solids, planes), append(g.boxes, bx)
	case mapfile.KindCurve, mapfile.KindMesh:
		p, err := br.Patch()
		if err != nil {
			return
		}
		for r := range max(len(p.Rows)-1, 0) {
			for c := range max(min(len(p.Rows[r]), len(p.Rows[r+1]))-1, 0) {
				a, b, d, e := p.Rows[r][c].Pos, p.Rows[r][c+1].Pos, p.Rows[r+1][c].Pos, p.Rows[r+1][c+1].Pos
				g.patches = append(g.patches, [3]mapfile.Vec3{a, b, d}, [3]mapfile.Vec3{b, e, d})
			}
		}
	}
}

// column is the stretch of a vertical line through (x, y) inside a convex
// brush, false when the line misses it: each plane bounds z from below or
// above, or leaves the line out.
func column(planes []plane, x, y float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(-1), math.Inf(1)
	for _, p := range planes {
		r := p.d - p.n[0]*x - p.n[1]*y // inside: n·v ≥ d, so n_z·z ≥ r
		switch {
		case p.n[2] > 1e-9:
			lo = math.Max(lo, r/p.n[2])
		case p.n[2] < -1e-9:
			hi = math.Min(hi, r/p.n[2])
		case r > eps:
			return 0, 0, false
		}
	}
	return lo, hi, hi-lo > eps
}

// onTriangle is where a vertical line through (x, y) meets a triangle,
// false when it passes beside it.
func onTriangle(t [3]mapfile.Vec3, x, y float64) (float64, bool) {
	d := (t[1][1]-t[2][1])*(t[0][0]-t[2][0]) + (t[2][0]-t[1][0])*(t[0][1]-t[2][1])
	if math.Abs(d) < 1e-9 {
		return 0, false
	}
	l0 := ((t[1][1]-t[2][1])*(x-t[2][0]) + (t[2][0]-t[1][0])*(y-t[2][1])) / d
	l1 := ((t[2][1]-t[0][1])*(x-t[2][0]) + (t[0][0]-t[2][0])*(y-t[2][1])) / d
	l2 := 1 - l0 - l1
	if l0 < -1e-6 || l1 < -1e-6 || l2 < -1e-6 {
		return 0, false
	}
	return l0*t[0][2] + l1*t[1][2] + l2*t[2][2], true
}

// clearance is the room a body stands in: 72 high (the player's), 32 wide.
const clearance, halfWidth = 72, 16

// bodyLines are where clear tries a body's box, across each way.
var bodyLines = [...]float64{-halfWidth, -halfWidth / 2, 0, halfWidth / 2, halfWidth}

// floor is the top of the ground under (x, y) between lo and hi, the
// highest, with a body's room clear above it; false when there is none, or
// the line starts inside a solid at hi.
func (g *ground) floor(x, y, lo, hi float64) (float64, bool) {
	z, found := math.Inf(-1), false
	for _, s := range g.solids {
		if a, b, ok := column(s, x, y); ok && b <= hi+eps && b >= lo-eps && b > z {
			z, found = b, true
		} else if ok && a < hi-eps && b > hi+eps {
			return 0, false // inside a wall or a pillar
		}
	}
	for _, t := range g.patches {
		if h, ok := onTriangle(t, x, y); ok && h <= hi+eps && h >= lo-eps && h > z {
			z, found = h, true
		}
	}
	return z, found && g.clear(x, y, z)
}

// clear tells whether a body standing at (x, y, z) has its room: no solid
// reaching into its box, tried along vertical lines every 8 units across it,
// so a wall 8 thick can't slip between them.
func (g *ground) clear(x, y, z float64) bool {
	body := Box{Min: mapfile.Vec3{x - halfWidth, y - halfWidth, z + 1}, Max: mapfile.Vec3{x + halfWidth, y + halfWidth, z + clearance}, Ok: true}
	for i, s := range g.solids {
		if !overlaps(g.boxes[i], body) {
			continue
		}
		for _, dx := range bodyLines {
			for _, dy := range bodyLines {
				if a, b, ok := column(s, x+dx, y+dy); ok && a < body.Max[2]-eps && b > body.Min[2] {
					return false
				}
			}
		}
	}
	return true
}
