package level

import (
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// ground is what a point can stand on, as the batch leaves the map: its
// world brushes and func_groups' (convex, exact along a vertical line) and
// its mesh and curve patches (their control grids, in triangles: terrain).
// Sky brushes and brush entities (volumes, triggers, doors) are not ground.
type ground struct {
	solids  [][]geom.Plane
	boxes   []geom.Box // each solid's bounds
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
		planes := make([]geom.Plane, len(faces))
		for i, f := range faces {
			if strings.EqualFold(f.Material, "sky") {
				return
			}
			planes[i] = geom.PlaneOf(f.Points)
		}
		var bx geom.Box
		for _, v := range geom.Vertices(planes) {
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

// clearance is the room a body stands in: 72 high (the player's), 32 wide.
const clearance, halfWidth = 72, 16

// bodyLines are where clear tries a body's box, across each way.
var bodyLines = [...]float64{-halfWidth, -halfWidth / 2, 0, halfWidth / 2, halfWidth}

// floor is the top of the ground under (x, y) between lo and hi, the
// highest, with a body's room clear above it; false when there is none. A
// line that starts inside a wall finds the wall in the body's room.
func (g *ground) floor(x, y, lo, hi float64) (float64, bool) {
	z, found := math.Inf(-1), false
	for _, s := range g.solids {
		if _, b, ok := geom.Column(s, x, y); ok && b <= hi+geom.Eps && b >= lo-geom.Eps && b > z {
			z, found = b, true
		}
	}
	for _, t := range g.patches {
		if h, ok := geom.OnTriangle(t, x, y); ok && h <= hi+geom.Eps && h >= lo-geom.Eps && h > z {
			z, found = h, true
		}
	}
	return z, found && g.clear(x, y, z)
}

// clear tells whether a body standing at (x, y, z) has its room: no solid
// reaching into its box, tried along vertical lines every 8 units across it,
// so a wall 8 thick can't slip between them.
func (g *ground) clear(x, y, z float64) bool {
	body := geom.Box{Min: mapfile.Vec3{x - halfWidth, y - halfWidth, z + 1}, Max: mapfile.Vec3{x + halfWidth, y + halfWidth, z + clearance}, Ok: true}
	for i, s := range g.solids {
		if !g.boxes[i].Overlaps(body) {
			continue
		}
		for _, dx := range bodyLines {
			for _, dy := range bodyLines {
				if a, b, ok := geom.Column(s, x+dx, y+dy); ok && a < body.Max[2]-geom.Eps && b > body.Min[2] {
					return false
				}
			}
		}
	}
	return true
}
