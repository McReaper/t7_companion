package level

import (
	"fmt"
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// TextureOp lays a material on faces of brushes (every brush of an entity the
// id names). Radiant projects a face's texture along the world axis its normal
// is closest to, so faces given the same tile and origin line up across
// brushes, seamless.
type TextureOp struct {
	IDs      []string    `json:"ids"`
	Faces    string      `json:"faces,omitempty"`    // all (the default), top, bottom, sides, or +x, -x, +y, -y, +z, -z, comma separated; or hidden
	Material string      `json:"material,omitempty"` // none keeps each face's
	Tile     *[2]float64 `json:"tile,omitempty"`     // world units one repeat spans, u then v; none: the material's own
	Scale    float64     `json:"scale,omitempty"`    // multiplies the tile; none: 1
	Origin   *[3]float64 `json:"origin,omitempty"`   // a world point a tile's corner sits on; none: 0 0 0
	Fit      *[2]float64 `json:"fit,omitempty"`      // repeats across each face, u then v, 0 to size that axis as the other
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
	case o.Fit != nil && (o.Fit[0] < 0 || o.Fit[1] < 0 || o.Fit[0]+o.Fit[1] == 0):
		return nil, fmt.Errorf("fit %v: repeats across each face, u then v, one of them above 0", *o.Fit)
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
	planes := make([]plane, len(faces))
	for i, f := range faces {
		planes[i] = planeOf(f.Points)
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
func (t *texturing) face(w []string, planes []plane, i int) error {
	n := planes[i].n // into the brush
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
	ou, ov := -dot(o, u), -dot(o, v)
	if t.op.Fit != nil {
		var err error
		if tw, th, ou, ov, err = t.fitted(faceVertices(planes, i), u, v, tw, th, ou, ov); err != nil {
			return fmt.Errorf("its %s face: %w", dir, err)
		}
	}
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

// fitted sizes the tile so the op's repeats span the face's corners along u
// and v (an axis at 0 is sized as the other, keeping the tile's shape), and
// starts the tiles at the face's edge on each fitted axis.
func (t *texturing) fitted(pts []mapfile.Vec3, u, v mapfile.Vec3, tw, th, ou, ov float64) (float64, float64, float64, float64, error) {
	if len(pts) < 3 {
		return 0, 0, 0, 0, fmt.Errorf("no area to fit")
	}
	u0, u1 := extent(pts, u)
	v0, v1 := extent(pts, v)
	fu, fv := t.op.Fit[0], t.op.Fit[1]
	nw, nh := tw, th
	if fu > 0 {
		nw, ou = (u1-u0)/fu, -u0
	}
	if fv > 0 {
		nh, ov = (v1-v0)/fv, -v0
	}
	switch {
	case fv == 0:
		nh = th * nw / tw
	case fu == 0:
		nw = tw * nh / th
	}
	if nw < 1e-3 || nh < 1e-3 {
		return 0, 0, 0, 0, fmt.Errorf("too thin to fit")
	}
	return nw, nh, ou, ov, nil
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

// faceVertices are face i's corners: where it meets two other planes inside
// every plane.
func faceVertices(planes []plane, i int) []mapfile.Vec3 {
	var out []mapfile.Vec3
	for j := range planes {
		for k := j + 1; k < len(planes); k++ {
			if j == i || k == i {
				continue
			}
			if v, ok := meet(planes[i], planes[j], planes[k]); ok && inside(v, planes) {
				out = append(out, v)
			}
		}
	}
	return out
}

// extent is the span of the points along a direction.
func extent(pts []mapfile.Vec3, d mapfile.Vec3) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		x := dot(p, d)
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return lo, hi
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

// textureHidden lays a material, caulk by default, on the faces of the ids'
// world brushes that other world brushes hide, as the batch leaves the map:
// against an opaque face or inside an opaque brush, the checks' own test. A
// face is named by its index, as faces at an angle can share a direction.
func (b *batch) textureHidden(i int, o *TextureOp) error {
	mat := o.Material
	if mat == "" {
		mat = "caulk"
	}
	switch {
	case !word(mat):
		return fmt.Errorf("texture: material %q: one word, a material's name", mat)
	case o.Tile != nil || o.Scale != 0 || o.Origin != nil || o.Fit != nil || o.Rotate != 0 || o.Flip != "":
		return fmt.Errorf("texture: faces hidden takes a material only: nobody sees how it lies")
	}
	info := b.info
	if info == nil {
		info = func(string) MaterialInfo { return MaterialInfo{Known: true, Opaque: true, Drawn: true} }
	}
	c := newChecker(func(g string) string { return g }, info)
	index := map[*mapfile.Brush]int{}
	for _, u := range b.units {
		if u.world && u.state != deleted {
			index[u.brush] = len(c.solids)
			c.add(u.brush)
		}
	}
	hidden := map[int][]int{}
	for _, f := range c.faces {
		if c.hidden(f) != "" {
			hidden[f.bi] = append(hidden[f.bi], f.fi)
		}
	}
	n := 0
	err := b.each(i, o.IDs, func(u *unit, br *mapfile.Brush) error {
		for _, x := range u.brushes(br) {
			if k, ok := index[x]; ok {
				if err := setFaces(x, hidden[k], mat, b.tile(mat)); err != nil {
					return err
				}
				n += len(hidden[k])
			}
		}
		return nil
	})
	b.faces[i] = n
	return err
}
