package level

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// outlineOf is an outline on the ground as a polygon map_edit can build:
// counter-clockwise, its repeated and collinear corners dropped, its edges
// crossing none but their neighbours.
func outlineOf(pts [][2]float64) ([]pt2, error) {
	var poly []pt2
	for _, p := range pts {
		q := pt2{p[0], p[1]}
		if len(poly) == 0 || math.Hypot(q.x-poly[len(poly)-1].x, q.y-poly[len(poly)-1].y) > eps {
			poly = append(poly, q)
		}
	}
	if len(poly) > 1 && math.Hypot(poly[0].x-poly[len(poly)-1].x, poly[0].y-poly[len(poly)-1].y) <= eps {
		poly = poly[:len(poly)-1]
	}
	poly = dropCollinear(poly)
	if len(poly) < 3 {
		return nil, fmt.Errorf("an outline needs three corners or more, not all on a line")
	}
	if signedArea(poly) < 0 {
		for i, j := 0, len(poly)-1; i < j; i, j = i+1, j-1 {
			poly[i], poly[j] = poly[j], poly[i]
		}
	}
	for i := range poly {
		for j := i + 2; j < len(poly); j++ {
			if (j+1)%len(poly) != i && properlyCross(poly[i], poly[(i+1)%len(poly)], poly[j], poly[(j+1)%len(poly)]) {
				return nil, fmt.Errorf("the outline crosses itself: edge %d meets edge %d", i, j)
			}
		}
	}
	return poly, nil
}

// dropCollinear drops the corners on the line between their neighbours.
func dropCollinear(poly []pt2) []pt2 {
	var out []pt2
	for i, p := range poly {
		a, b := poly[(i+len(poly)-1)%len(poly)], poly[(i+1)%len(poly)]
		if math.Abs(cross2(a, p, b)) > eps*math.Hypot(b.x-a.x, b.y-a.y) {
			out = append(out, p)
		}
	}
	return out
}

// signedArea is a polygon's area, positive when it winds counter-clockwise.
func signedArea(poly []pt2) float64 {
	var a float64
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		a += p.x*q.y - q.x*p.y
	}
	return a / 2
}

// convexParts splits a counter-clockwise outline into convex polygons, few
// of them: ear clipping into triangles, then neighbours merged while what
// they make stays convex (Hertel–Mehlhorn). Each is a brush, as a brush
// can only be convex.
func convexParts(poly []pt2) ([][]pt2, error) {
	if convex(poly) {
		return [][]pt2{poly}, nil
	}
	tris, err := earClip(poly)
	if err != nil {
		return nil, err
	}
	parts := tris
	for range len(tris) { // each pass merges a pair, or none and the merging is done
		merged := false
		for i := range parts {
			for j := i + 1; j < len(parts) && !merged; j++ {
				if u, ok := mergeConvex(parts[i], parts[j]); ok {
					parts[i], parts[j] = u, parts[len(parts)-1]
					parts, merged = parts[:len(parts)-1], true
				}
			}
			if merged {
				break
			}
		}
		if !merged {
			break
		}
	}
	return parts, nil
}

// convex tells a counter-clockwise polygon that turns left, or runs
// straight, at every corner.
func convex(poly []pt2) bool {
	for i, p := range poly {
		if cross2(poly[(i+len(poly)-1)%len(poly)], p, poly[(i+1)%len(poly)]) < -eps {
			return false
		}
	}
	return true
}

// earClip cuts a counter-clockwise polygon into triangles, an ear at a
// time: a convex corner whose triangle holds no other corner.
func earClip(poly []pt2) ([][]pt2, error) {
	left := append([]pt2(nil), poly...)
	var tris [][]pt2
	for range len(poly) {
		if len(left) == 3 {
			return append(tris, left), nil
		}
		ear := -1
		for i := range left {
			if isEar(left, i) {
				ear = i
				break
			}
		}
		if ear < 0 {
			return nil, fmt.Errorf("the outline can't be cut into triangles: is it one shape, its corners in order?")
		}
		n := len(left)
		tris = append(tris, []pt2{left[(ear+n-1)%n], left[ear], left[(ear+1)%n]})
		left = append(left[:ear:ear], left[ear+1:]...)
	}
	return nil, fmt.Errorf("the outline can't be cut into triangles")
}

// isEar tells whether corner i cuts off as a triangle inside the polygon.
func isEar(poly []pt2, i int) bool {
	n := len(poly)
	a, b, c := poly[(i+n-1)%n], poly[i], poly[(i+1)%n]
	if cross2(a, b, c) <= eps {
		return false
	}
	tri := []pt2{a, b, c}
	for j, p := range poly {
		if j != i && j != (i+n-1)%n && j != (i+1)%n && inHull(tri, p) {
			return false
		}
	}
	return true
}

// mergeConvex joins two convex polygons that share an edge, when what they
// make is convex too.
func mergeConvex(a, b []pt2) ([]pt2, bool) {
	same := func(p, q pt2) bool { return math.Hypot(p.x-q.x, p.y-q.y) <= eps }
	for i := range a {
		p, q := a[i], a[(i+1)%len(a)]
		for j := range b {
			if !same(b[j], q) || !same(b[(j+1)%len(b)], p) {
				continue
			}
			// a from q round to p, then b on from p back to q, both ends left out
			var u []pt2
			for k := range len(a) {
				u = append(u, a[(i+1+k)%len(a)])
			}
			for k := range len(b) - 2 {
				u = append(u, b[(j+2+k)%len(b)])
			}
			u = dropCollinear(u)
			return u, len(u) >= 3 && convex(u)
		}
	}
	return nil, false
}

// prismBrush is the text of a convex brush standing on a counter-clockwise
// polygon, from z0 to z1, its faces wound as boxBrush winds them.
func prismBrush(poly []pt2, z0, z1 float64, material, tex string) string {
	v := func(p pt2, z float64) string { return "( " + num(p.x) + " " + num(p.y) + " " + num(z) + " )" }
	o := poly[0]
	faces := [][3]string{
		{v(o, z0), v(pt2{o.x + 1, o.y}, z0), v(pt2{o.x, o.y + 1}, z0)}, // bottom
		{v(o, z1), v(pt2{o.x, o.y + 1}, z1), v(pt2{o.x + 1, o.y}, z1)}, // top
	}
	for i, a := range poly { // a side's three points turn into the brush, left of a→b
		faces = append(faces, [3]string{v(a, z0), v(a, z0+1), v(poly[(i+1)%len(poly)], z0)})
	}
	var s strings.Builder
	s.WriteString("{\n")
	for _, f := range faces {
		s.WriteString(" " + f[0] + " " + f[1] + " " + f[2] + " " + material + " " + tex + "\n")
	}
	s.WriteString("}\n")
	return s.String()
}

// withContents gives a brush's text a contents line, as Radiant writes
// one: "contents nonColliding;" draws the brush and lets nothing collide with
// it (Selection > Make Geo > Make Non-Colliding).
func withContents(brush, contents string) string {
	if contents == "" {
		return brush
	}
	return strings.Replace(brush, "{\n", "{\n contents "+strings.TrimSuffix(contents, ";")+";\n", 1)
}

// ShapeOp adds a solid standing on an outline on the ground, from z[0] to
// z[1]: a world brush for each convex piece of it (a brush is convex), or
// the brushes of a new brush entity of class.
type ShapeOp struct {
	Outline  [][2]float64      `json:"outline"`
	Z        [2]float64        `json:"z"`
	Material string            `json:"material"`
	Contents string            `json:"contents,omitempty"` // each brush's contents, as a box's: detail, nonColliding
	Class    string            `json:"class,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`
}

// shape runs a shape op.
func (b *batch) shape(i int, o *ShapeOp) error {
	switch {
	case o.Z[1] <= o.Z[0]:
		return fmt.Errorf("shape: z %v: its top above its bottom", o.Z)
	case !word(o.Material):
		return fmt.Errorf("shape: material %q: one word, a material's name", o.Material)
	case o.Contents != "" && !word(o.Contents):
		return fmt.Errorf("shape: contents %q: one word, as detail or nonColliding", o.Contents)
	case o.Class == "" && len(o.Keys) > 0:
		return fmt.Errorf("shape: keys belong to a brush entity: give its class")
	}
	poly, err := outlineOf(o.Outline)
	if err != nil {
		return fmt.Errorf("shape: %w", err)
	}
	parts, err := convexParts(poly)
	if err != nil {
		return fmt.Errorf("shape: %w", err)
	}
	t := b.tile(o.Material)
	var brushes []string
	for _, p := range parts {
		brushes = append(brushes, withContents(prismBrush(p, o.Z[0], o.Z[1], o.Material, texNumbers(t.W, t.H)), o.Contents))
	}
	var made []*mapfile.Brush
	if o.Class != "" {
		if err := checkKeys(o.Class, o.Keys); err != nil {
			return fmt.Errorf("shape: %w", err)
		}
		if err := b.made(i, EntityText(o.Class, o.Keys, brushes...), false); err != nil {
			return err
		}
		made = b.units[len(b.units)-1].ent.Brushes()
	} else {
		for _, br := range brushes {
			if err := b.made(i, EntityText("worldspawn", nil, br), true); err != nil {
				return err
			}
			made = append(made, b.units[len(b.units)-1].brush)
		}
	}
	for k, faces := range selfHiddenFaces(made) { // where its pieces meet
		if err := setFaces(made[k], faces, "caulk", b.tile("caulk")); err != nil {
			return fmt.Errorf("shape: %w", err)
		}
	}
	return nil
}

// setFaces lays a material on a brush's faces of those indices, at a tile.
func setFaces(br *mapfile.Brush, faces []int, material string, t Tile) error {
	n := -1
	return rewrite(br, func(w []string) error {
		if n++; slices.Contains(faces, n) {
			w[15], w[16], w[17] = material, num(t.W), num(t.H)
		}
		return nil
	}, nil, func([]string) {})
}
