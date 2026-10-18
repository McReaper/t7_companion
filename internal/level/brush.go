package level

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// The elements an agent places as it means them: a brush through its
// corners, a patch through its control points, or the map's own text. Each
// is a world brush, or the brush of a new entity of class.

// BrushOp adds a convex brush through its corners: its faces are the hull of
// the points (a box is its 8 corners, a wedge its 6, a column of n sides its
// 2n), each in material at the material's own tile.
type BrushOp struct {
	Points   [][3]float64      `json:"points"`
	Material string            `json:"material"`
	Contents string            `json:"contents,omitempty"`
	Class    string            `json:"class,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`
}

// PatchOp adds a patch through its control points: rows of as many points
// each, a curve (3, 5, 7… each way: Radiant's Bézier patch) or a mesh
// (terrain: 2 or more), in material, its texture laid along the rows and
// columns by their length (Radiant's patch tools lay it otherwise).
type PatchOp struct {
	Rows     [][][3]float64    `json:"rows"`
	Material string            `json:"material"`
	Mesh     bool              `json:"mesh,omitempty"`
	Contents string            `json:"contents,omitempty"`
	Class    string            `json:"class,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`
}

const (
	maxPoints  = 64 // a brush's corners at most
	patchTexel = 16 // a patch's texture coordinates per unit along it, as Radiant writes them
)

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

// brushOp runs a brush op.
func (b *batch) brushOp(i int, o *BrushOp) error {
	if err := checkElement(o.Material, o.Contents); err != nil {
		return fmt.Errorf("brush: %w", err)
	}
	faces, err := hullFaces(o.Points)
	if err != nil {
		return fmt.Errorf("brush: %w", err)
	}
	t := b.tile(o.Material)
	var s strings.Builder
	s.WriteString("{\n")
	writeContents(&s, o.Contents)
	for _, f := range faces {
		s.WriteString(" " + point(f[0]) + " " + point(f[1]) + " " + point(f[2]) + " " + o.Material + " " + texNumbers(t.W, t.H) + "\n")
	}
	s.WriteString("}\n")
	return b.placed(i, "brush", s.String(), o.Class, o.Keys)
}

// patchOp runs a patch op.
func (b *batch) patchOp(i int, o *PatchOp) error {
	if err := checkElement(o.Material, o.Contents); err != nil {
		return fmt.Errorf("patch: %w", err)
	}
	if err := checkGrid(o.Rows, o.Mesh); err != nil {
		return fmt.Errorf("patch: %w", err)
	}
	kind, dims := "curve", " 16 8"
	if o.Mesh {
		kind, dims = "mesh", " 0 8"
	}
	var s strings.Builder
	s.WriteString("{\n " + kind + "\n {\n")
	writeContents(&s, o.Contents)
	s.WriteString("  " + o.Material + "\n  lightmap_gray\n  " + fmt.Sprint(len(o.Rows)) + " " + fmt.Sprint(len(o.Rows[0])) + dims + "\n")
	across := make([]float64, len(o.Rows[0])) // each column's length so far, across the rows
	for r, row := range o.Rows {
		s.WriteString("  (\n")
		along := 0.0
		for c, p := range row {
			if r > 0 {
				across[c] += dist(mapfile.Vec3(o.Rows[r-1][c]), mapfile.Vec3(p))
			}
			if c > 0 {
				along += dist(mapfile.Vec3(row[c-1]), mapfile.Vec3(p))
			}
			s.WriteString("   v " + num(p[0]) + " " + num(p[1]) + " " + num(p[2]) + " t " + num(across[c]*patchTexel) + " " + num(-along*patchTexel) + " " + fmt.Sprint(2*r+1) + " " + fmt.Sprint(2*c+1) + "\n")
		}
		s.WriteString("  )\n")
	}
	s.WriteString(" }\n}\n")
	return b.placed(i, "patch", s.String(), o.Class, o.Keys)
}

// checkElement checks a new element's material and contents.
func checkElement(material, contents string) error {
	if !word(material) {
		return fmt.Errorf("material %q: one word, a material's name", material)
	}
	return checkContents(contents)
}

// checkGrid checks a patch's control points: rows alike, a curve's counts odd
// from 3, a mesh's from 2.
func checkGrid(rows [][][3]float64, mesh bool) error {
	least, odd := 3, true
	if mesh {
		least, odd = 2, false
	}
	if len(rows) == 0 {
		return fmt.Errorf("no rows")
	}
	if len(rows) < least || len(rows[0]) < least {
		return fmt.Errorf("%d rows of %d points: %d each way at least", len(rows), len(rows[0]), least)
	}
	for k, row := range rows {
		if len(row) != len(rows[0]) {
			return fmt.Errorf("row %d has %d points, row 0 %d: every row as many", k, len(row), len(rows[0]))
		}
	}
	if odd && (len(rows)%2 == 0 || len(rows[0])%2 == 0) {
		return fmt.Errorf("a curve of %d rows of %d points: odd counts each way (3, 5, 7…), or mesh", len(rows), len(rows[0]))
	}
	return nil
}

// writeContents writes a brush's contents line, as Radiant writes one.
func writeContents(s *strings.Builder, contents string) {
	if c := strings.TrimSpace(strings.TrimSuffix(contents, ";")); c != "" {
		s.WriteString(" contents " + c + ";\n")
	}
}

// placed adds what op i places: a world brush, or the brush of a new entity
// of class.
func (b *batch) placed(i int, what, brush, class string, keys map[string]string) error {
	if class == "" {
		if len(keys) > 0 {
			return fmt.Errorf("%s: keys belong to an entity: give its class", what)
		}
		return b.made(i, EntityText("worldspawn", nil, brush), true)
	}
	if err := checkKeys(class, keys); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return b.made(i, EntityText(class, keys, brush), false)
}

// raw adds the map text op i gives: its entities, a worldspawn's brushes as
// world brushes. Their GUIDs are dropped, as the import gives new ones; each
// brush must read as faces or a patch.
func (b *batch) raw(i int, text string) error {
	var kept []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if f := strings.Fields(l); len(f) == 0 || f[0] != "guid" {
			kept = append(kept, l)
		}
	}
	m, err := mapfile.Parse([]byte(strings.Join(kept, "\n")))
	if err != nil {
		return fmt.Errorf("raw: %w", err)
	}
	if len(m.Entities) == 0 {
		return fmt.Errorf("raw: no entity in the text")
	}
	for _, e := range m.Entities {
		for _, br := range e.Brushes() {
			if err := readable(br); err != nil {
				return fmt.Errorf("raw: %w", err)
			}
		}
		if e.Class() != "worldspawn" {
			b.units = append(b.units, &unit{ent: e, state: created, op: i})
			continue
		}
		for _, br := range e.Brushes() {
			b.units = append(b.units, &unit{brush: br, world: true, state: created, op: i})
		}
	}
	return nil
}

// readable checks that a brush reads as faces (4 or more) or a patch.
func readable(br *mapfile.Brush) error {
	switch br.Kind() {
	case mapfile.KindFaces:
		if faces, err := br.Faces(); err != nil || len(faces) < 4 {
			return fmt.Errorf("a brush of %d faces (%v): 4 at least, each through three points", len(faces), err)
		}
	case mapfile.KindCurve, mapfile.KindMesh:
		if _, err := br.Patch(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("a brush that is neither faces nor a patch")
	}
	return nil
}

// hullFaces are the faces of the convex hull of points, each as three of
// them wound so that (p1-p0)×(p2-p0) points into the hull, as Radiant winds
// a face; the hull must enclose a volume.
func hullFaces(pts [][3]float64) ([][3]mapfile.Vec3, error) {
	if len(pts) < 4 || len(pts) > maxPoints {
		return nil, fmt.Errorf("%d points: 4 to %d corners", len(pts), maxPoints)
	}
	ps := make([]mapfile.Vec3, len(pts))
	var c mapfile.Vec3
	for k, p := range pts {
		ps[k] = mapfile.Vec3(p)
		for a := range 3 {
			c[a] += p[a] / float64(len(pts))
		}
	}
	var planes []geom.Plane
	var faces [][3]mapfile.Vec3
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			for k := j + 1; k < len(ps); k++ {
				f, pl, ok := hullFace(ps, i, j, k, c)
				if ok && !slices.ContainsFunc(planes, func(q geom.Plane) bool { return samePlane(q, pl) }) {
					planes, faces = append(planes, pl), append(faces, f)
				}
			}
		}
	}
	if len(faces) < 4 || len(geom.Vertices(planes)) < 4 {
		return nil, fmt.Errorf("the points enclose no volume: they lie on a plane or a line")
	}
	return faces, nil
}

// hullFace is the face of the hull through points i, j, k, if every point
// lies on its inner side: the three points wound into the hull, its plane.
func hullFace(ps []mapfile.Vec3, i, j, k int, c mapfile.Vec3) ([3]mapfile.Vec3, geom.Plane, bool) {
	f := [3]mapfile.Vec3{ps[i], ps[j], ps[k]}
	n := geom.Cross(geom.Sub(f[1], f[0]), geom.Sub(f[2], f[0]))
	if l := math.Sqrt(geom.Dot(n, n)); l < 1e-6 {
		return f, geom.Plane{}, false // in a line
	}
	if geom.Dot(n, geom.Sub(c, f[0])) < 0 {
		f[1], f[2] = f[2], f[1]
	}
	pl := geom.PlaneOf(f)
	for _, p := range ps {
		if geom.Dot(pl.N, p) < pl.D-geom.Eps {
			return f, pl, false
		}
	}
	return f, pl, true
}

// samePlane tells whether two planes are one.
func samePlane(a, b geom.Plane) bool {
	return math.Abs(a.D-b.D) < geom.Eps && math.Abs(geom.Dot(a.N, b.N)-1) < 1e-9
}

func point(p mapfile.Vec3) string { return "( " + num(p[0]) + " " + num(p[1]) + " " + num(p[2]) + " )" }
