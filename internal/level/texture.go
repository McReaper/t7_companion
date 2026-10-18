package level

import (
	"fmt"
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// TextureOp lays a material on faces of brushes (every brush of an entity the
// id names). Radiant projects a face's texture along the world axis its normal
// is closest to, so faces given the same tile and origin line up across
// brushes, seamless.
type TextureOp struct {
	IDs      []string    `json:"ids"`
	Faces    string      `json:"faces,omitempty"`    // all (the default), top, bottom, sides, or +x, -x, +y, -y, +z, -z, comma separated
	Material string      `json:"material,omitempty"` // none keeps each face's
	Tile     *[2]float64 `json:"tile,omitempty"`     // world units one repeat spans, u then v; none: the material's own
	Scale    float64     `json:"scale,omitempty"`    // multiplies the tile; none: 1
	Origin   *[3]float64 `json:"origin,omitempty"`   // a world point a tile's corner sits on; none: 0 0 0
	Rotate   float64     `json:"rotate,omitempty"`   // degrees
	Flip     string      `json:"flip,omitempty"`     // u, v or uv: mirrors the texture
}

// texturing is a TextureOp read and checked, with what it has textured.
type texturing struct {
	op           *TextureOp
	faces        map[string]bool
	flipU, flipV bool
	tile         func(material string) Tile
	n            int // faces textured
}

func newTexturing(o *TextureOp, tile func(string) Tile) (*texturing, error) {
	faces, err := faceSet(o.Faces)
	if err != nil {
		return nil, err
	}
	t := &texturing{op: o, faces: faces, tile: tile}
	switch {
	case o.Material != "" && !word(o.Material):
		return nil, fmt.Errorf("material %q: one word, a material's name", o.Material)
	case o.Tile != nil && (o.Tile[0] <= 0 || o.Tile[1] <= 0):
		return nil, fmt.Errorf("tile %v: two sizes above 0, world units across and down", *o.Tile)
	case o.Scale < 0:
		return nil, fmt.Errorf("scale %v: above 0 (2 makes the tiles twice as large)", o.Scale)
	}
	switch strings.ToLower(o.Flip) {
	case "":
	case "u":
		t.flipU = true
	case "v":
		t.flipV = true
	case "uv", "vu":
		t.flipU, t.flipV = true, true
	default:
		return nil, fmt.Errorf("flip %q: u, v or uv", o.Flip)
	}
	return t, nil
}

// faceDirs are the directions a face's outward normal can be closest to.
var faceDirs = []string{"+x", "-x", "+y", "-y", "+z", "-z"}

// faceSet reads a TextureOp's faces.
func faceSet(s string) (map[string]bool, error) {
	if strings.TrimSpace(s) == "" {
		s = "all"
	}
	out := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		switch p = strings.ToLower(strings.TrimSpace(p)); p {
		case "all":
			for _, d := range faceDirs {
				out[d] = true
			}
		case "top":
			out["+z"] = true
		case "bottom":
			out["-z"] = true
		case "sides":
			for _, d := range faceDirs[:4] {
				out[d] = true
			}
		case "+x", "-x", "+y", "-y", "+z", "-z":
			out[p] = true
		default:
			return nil, fmt.Errorf("faces %q: all, top, bottom, sides, or +x, -x, +y, -y, +z, -z, comma separated", s)
		}
	}
	return out, nil
}

// brush textures a brush's faces among the op's; a patch has none.
func (t *texturing) brush(b *mapfile.Brush) error {
	if b.Kind() != mapfile.KindFaces {
		return nil
	}
	faces, err := b.Faces()
	if err != nil {
		return err
	}
	planes := make([]geom.Plane, len(faces))
	for i, f := range faces {
		planes[i] = geom.PlaneOf(f.Points)
	}
	k := 0
	return rewrite(b, func(w []string) error {
		i := k
		k++
		if i >= len(planes) {
			return nil
		}
		return t.face(w, planes, i)
	}, nil, nil)
}

// face writes face i's texture into its words, when its direction is one of
// the op's.
func (t *texturing) face(w []string, planes []geom.Plane, i int) error {
	n := planes[i].N // into the brush
	dir, u, v := texAxes(mapfile.Vec3{-n[0], -n[1], -n[2]})
	if !t.faces[dir] {
		return nil
	}
	mat := w[15]
	if t.op.Material != "" {
		mat = t.op.Material
	}
	tw, th := t.size(mat)
	u, v = turned(u, v, t.op.Rotate)
	var o mapfile.Vec3
	if t.op.Origin != nil {
		o = *t.op.Origin
	}
	ou, ov := -geom.Dot(o, u), -geom.Dot(o, v)
	sw, sh := tw, th
	if t.flipU {
		sw = -sw
	}
	if t.flipV {
		sh = -sh
	}
	w[15], w[16], w[17] = mat, num(sw), num(sh)
	w[18], w[19], w[20] = num(wrap(ou, tw)), num(wrap(ov, th)), num(t.op.Rotate)
	t.n++
	return nil
}

// size is a material's tile for the op: its own or the op's, scaled.
func (t *texturing) size(mat string) (float64, float64) {
	var tw, th float64
	if t.op.Tile != nil {
		tw, th = t.op.Tile[0], t.op.Tile[1]
	} else {
		got := t.tile(mat)
		tw, th = got.W, got.H
	}
	if t.op.Scale > 0 {
		tw, th = tw*t.op.Scale, th*t.op.Scale
	}
	return tw, th
}

// texAxes are the direction an outward normal is closest to (z first, then x,
// then y on a tie) and the world directions a face's texture runs along
// unrotated: Radiant writes a face's u as (p·U + its u offset) / its width,
// and v the same way (read off Radiant's Fit Texture on a test box).
func texAxes(out mapfile.Vec3) (string, mapfile.Vec3, mapfile.Vec3) {
	ax, ay, az := math.Abs(out[0]), math.Abs(out[1]), math.Abs(out[2])
	sign := func(x float64) string {
		if x < 0 {
			return "-"
		}
		return "+"
	}
	switch {
	case az >= ax && az >= ay:
		return sign(out[2]) + "z", mapfile.Vec3{-1, 0, 0}, mapfile.Vec3{0, 1, 0}
	case ax >= ay:
		return sign(out[0]) + "x", mapfile.Vec3{0, -1, 0}, mapfile.Vec3{0, 0, 1}
	}
	return sign(out[1]) + "y", mapfile.Vec3{-1, 0, 0}, mapfile.Vec3{0, 0, 1}
}

// turned is u and v turned by deg degrees as Radiant turns a texture: at 90,
// u runs where -v ran and v where u ran.
func turned(u, v mapfile.Vec3, deg float64) (mapfile.Vec3, mapfile.Vec3) {
	s, c := math.Sincos(deg * math.Pi / 180)
	if math.Mod(deg, 90) == 0 { // exact, so tiles land on whole units
		s, c = math.Round(s), math.Round(c)
	}
	var tu, tv mapfile.Vec3
	for i := range 3 {
		tu[i] = c*u[i] - s*v[i]
		tv[i] = s*u[i] + c*v[i]
	}
	return tu, tv
}

// wrap is an offset brought into [0, size): the same tiles, smaller numbers.
func wrap(x, size float64) float64 {
	x = math.Mod(x, size)
	if x < 0 {
		x += size
	}
	if size-x < 1e-6 {
		return 0
	}
	return x
}
