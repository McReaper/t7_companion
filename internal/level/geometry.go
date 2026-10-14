package level

import (
	"math"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// Box is an axis-aligned box; the zero Box with Ok false holds nothing.
type Box struct {
	Min, Max mapfile.Vec3
	Ok       bool
}

// Add grows the box to hold p.
func (b *Box) Add(p mapfile.Vec3) {
	if !b.Ok {
		*b = Box{p, p, true}
		return
	}
	for i := range 3 {
		b.Min[i], b.Max[i] = math.Min(b.Min[i], p[i]), math.Max(b.Max[i], p[i])
	}
}

// Union grows the box to hold o.
func (b *Box) Union(o Box) {
	if o.Ok {
		b.Add(o.Min)
		b.Add(o.Max)
	}
}

// Centre is the box's centre.
func (b Box) Centre() mapfile.Vec3 {
	return mapfile.Vec3{(b.Min[0] + b.Max[0]) / 2, (b.Min[1] + b.Max[1]) / 2, (b.Min[2] + b.Max[2]) / 2}
}

// Corners are the box's eight corners.
func (b Box) Corners() []mapfile.Vec3 {
	var out []mapfile.Vec3
	for i := range 8 {
		out = append(out, mapfile.Vec3{pick(i&1, b.Min[0], b.Max[0]), pick(i&2, b.Min[1], b.Max[1]), pick(i&4, b.Min[2], b.Max[2])})
	}
	return out
}

func pick(bit int, lo, hi float64) float64 {
	if bit != 0 {
		return hi
	}
	return lo
}

// Distance is how far p is from the box, 0 inside it.
func (b Box) Distance(p mapfile.Vec3) float64 {
	var d2 float64
	for i := range 3 {
		if d := math.Max(b.Min[i]-p[i], p[i]-b.Max[i]); d > 0 {
			d2 += d * d
		}
	}
	return math.Sqrt(d2)
}

// eps is how far outside a face a brush vertex may fall: map coordinates are
// written with a few decimals.
const eps = 0.01

// BrushBox is a brush's bounds: a classic brush's vertices, where three of its
// face planes meet inside all the others (a face's three points wind so that
// (p1-p0)×(p2-p0) points into the brush), or a patch's control points. A
// point entity's empty brush, or a brush that encloses nothing, has none.
func BrushBox(b *mapfile.Brush) Box {
	var box Box
	switch b.Kind() {
	case mapfile.KindFaces:
		faces, err := b.Faces()
		if err != nil {
			return box
		}
		planes := make([]plane, len(faces))
		for i, f := range faces {
			planes[i] = planeOf(f.Points)
		}
		for _, v := range vertices(planes) {
			box.Add(v)
		}
	case mapfile.KindCurve, mapfile.KindMesh:
		p, err := b.Patch()
		if err != nil {
			return box
		}
		for _, row := range p.Rows {
			for _, v := range row {
				box.Add(v.Pos)
			}
		}
	}
	return box
}

// plane is n·x = d, n pointing into the brush.
type plane struct {
	n mapfile.Vec3
	d float64
}

func planeOf(p [3]mapfile.Vec3) plane {
	n := cross(sub(p[1], p[0]), sub(p[2], p[0]))
	if l := math.Sqrt(dot(n, n)); l > 0 {
		n = mapfile.Vec3{n[0] / l, n[1] / l, n[2] / l}
	}
	return plane{n, dot(n, p[0])}
}

// vertices are the points where three planes meet inside every plane.
func vertices(planes []plane) []mapfile.Vec3 {
	var out []mapfile.Vec3
	for i := range planes {
		for j := i + 1; j < len(planes); j++ {
			for k := j + 1; k < len(planes); k++ {
				v, ok := meet(planes[i], planes[j], planes[k])
				if ok && inside(v, planes) {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// meet solves the three planes' system by Cramer's rule.
func meet(a, b, c plane) (mapfile.Vec3, bool) {
	bc := cross(b.n, c.n)
	det := dot(a.n, bc)
	if math.Abs(det) < 1e-9 {
		return mapfile.Vec3{}, false
	}
	ca, ab := cross(c.n, a.n), cross(a.n, b.n)
	var v mapfile.Vec3
	for i := range 3 {
		v[i] = (a.d*bc[i] + b.d*ca[i] + c.d*ab[i]) / det
	}
	return v, true
}

func inside(v mapfile.Vec3, planes []plane) bool {
	for _, p := range planes {
		if dot(p.n, v) < p.d-eps {
			return false
		}
	}
	return true
}

func sub(a, b mapfile.Vec3) mapfile.Vec3 { return mapfile.Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func dot(a, b mapfile.Vec3) float64      { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b mapfile.Vec3) mapfile.Vec3 {
	return mapfile.Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// Origin reads an entity's "origin" key, false when it has none or it is not
// three numbers.
func Origin(e *mapfile.Entity) (mapfile.Vec3, bool) {
	s, ok := e.Key("origin")
	if !ok {
		return mapfile.Vec3{}, false
	}
	return ParseVec3(s)
}

// ParseVec3 reads "x y z".
func ParseVec3(s string) (mapfile.Vec3, bool) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return mapfile.Vec3{}, false
	}
	var v mapfile.Vec3
	for i, x := range f {
		n, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return mapfile.Vec3{}, false
		}
		v[i] = n
	}
	return v, true
}

// EntityBox is an entity's bounds: its brushes', else its origin.
func EntityBox(e *mapfile.Entity) Box {
	var box Box
	for _, b := range e.Brushes() {
		box.Union(BrushBox(b))
	}
	if !box.Ok {
		if o, ok := Origin(e); ok {
			box.Add(o)
		}
	}
	return box
}
