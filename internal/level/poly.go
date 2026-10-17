package level

import (
	"fmt"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// prismBrush is the text of a convex brush standing on a counter-clockwise
// polygon, from z0 to z1, its faces wound as boxBrush winds them.
func prismBrush(poly []geom.P2, z0, z1 float64, material, tex string) string {
	v := func(p geom.P2, z float64) string { return "( " + num(p.X) + " " + num(p.Y) + " " + num(z) + " )" }
	o := poly[0]
	faces := [][3]string{
		{v(o, z0), v(geom.P2{X: o.X + 1, Y: o.Y}, z0), v(geom.P2{X: o.X, Y: o.Y + 1}, z0)}, // bottom
		{v(o, z1), v(geom.P2{X: o.X, Y: o.Y + 1}, z1), v(geom.P2{X: o.X + 1, Y: o.Y}, z1)}, // top
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

// contentsFlags are the words of a brush's contents line Treyarch's own map
// sources use (zm_giant, mp_combine, mp_sector): nonColliding draws a brush
// nothing collides with, weaponClip one only bullets hit, detail keeps it out
// of the portal structure, ai_nosight blocks the AI's sight.
var contentsFlags = []string{"nonColliding", "weaponClip", "detail", "ai_nosight", "umbraTarget", "triangulatedWeaponClip"}

// checkContents accepts a contents line of those words, in any order.
func checkContents(contents string) error {
	for _, w := range strings.Fields(strings.TrimSuffix(contents, ";")) {
		if !slices.Contains(contentsFlags, w) {
			return fmt.Errorf("contents %q: %q is not one of %s", contents, w, strings.Join(contentsFlags, ", "))
		}
	}
	return nil
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
	case checkContents(o.Contents) != nil:
		return fmt.Errorf("shape: %w", checkContents(o.Contents))
	case o.Class == "" && len(o.Keys) > 0:
		return fmt.Errorf("shape: keys belong to a brush entity: give its class")
	}
	poly, err := geom.OutlineOf(o.Outline)
	if err != nil {
		return fmt.Errorf("shape: %w", err)
	}
	parts, err := geom.ConvexParts(poly)
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
