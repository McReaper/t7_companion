package level

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// boxOf is a box brush, parsed.
func boxOf(t *testing.T, lo, hi mapfile.Vec3, material string) *mapfile.Brush {
	t.Helper()
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, BoxBrush(box(lo, hi), material, ""))))
	if err != nil {
		t.Fatal(err)
	}
	return m.Entities[0].Brushes()[0]
}

// faceTex is each face's material and texture numbers, by the direction its
// outward normal is closest to.
func faceTex(t *testing.T, b *mapfile.Brush) map[string]mapfile.Face {
	t.Helper()
	faces, err := b.Faces()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]mapfile.Face{}
	for _, f := range faces {
		n := planeOf(f.Points).n
		dir, _, _ := texAxes(mapfile.Vec3{-n[0], -n[1], -n[2]})
		out[dir] = f
	}
	return out
}

func textured(t *testing.T, b *mapfile.Brush, o *TextureOp, tile func(string) Tile) *texturing {
	t.Helper()
	if tile == nil {
		tile = func(string) Tile { return Tile{W: 64, H: 64} }
	}
	tx, err := newTexturing(o, tile)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.brush(b); err != nil {
		t.Fatal(err)
	}
	return tx
}

// Radiant's own Textures/Fit Texture on a test box (x 1000..1256, y 500..628),
// read back from Radiant: a fit of one repeat each way gives the same tiles,
// its sizes negative (mirrored) and its offsets another of the same tiles' (an
// offset is the same tiles at any multiple of the size). That pins the axes
// each face's texture runs along, and how a rotation turns them.
func TestAFitLandsTheTilesWhereRadiantsFitTextureDoes(t *testing.T) {
	for _, c := range []struct {
		z0, z1  float64
		rot     float64
		dir     string
		radiant [4]float64 // width, height, u offset, v offset, as Radiant wrote them
	}{
		{0, 64, 0, "+z", [4]float64{-256, 128, -24, 12}},
		{0, 64, 0, "-z", [4]float64{-256, 128, -24, 12}},
		{0, 64, 0, "+x", [4]float64{-128, 64, -12, 0}},
		{0, 64, 0, "-x", [4]float64{-128, 64, -12, 0}},
		{0, 64, 0, "+y", [4]float64{-256, 64, -24, 0}},
		{0, 64, 0, "-y", [4]float64{-256, 64, -24, 0}},
		{0, 64, 90, "+z", [4]float64{-128, -256, -12, -24}},
		{0, 64, 90, "+y", [4]float64{-64, -256, 0, -24}},
		{8, 72, 90, "+z", [4]float64{-128, -256, -12, -24}},
		{8, 72, 90, "-x", [4]float64{-64, -128, -56, -12}},
		{8, 72, 90, "+y", [4]float64{-64, -256, -56, -24}},
	} {
		b := boxOf(t, mapfile.Vec3{1000, 500, c.z0}, mapfile.Vec3{1256, 628, c.z1}, "script_wall")
		textured(t, b, &TextureOp{Fit: &[2]float64{1, 1}, Rotate: c.rot}, nil)
		f := faceTex(t, b)[c.dir]
		r := c.radiant
		want := [4]float64{math.Abs(r[0]), math.Abs(r[1]), wrap(r[2], math.Abs(r[0])), wrap(r[3], math.Abs(r[1]))}
		if got := [4]float64{f.Tex[0], f.Tex[1], f.Tex[2], f.Tex[3]}; got != want || f.Tex[4] != c.rot {
			t.Errorf("z %v..%v, rotate %v, %s face: %v rotated %v, Radiant's tiles are %v", c.z0, c.z1, c.rot, c.dir, got, f.Tex[4], want)
		}
	}
}

func TestATextureLaysTheMaterialsTileFromTheWorldsOrigin(t *testing.T) {
	sizes := map[string]Tile{"brick": {W: 64, H: 32}, "plank": {W: 128, H: 16}, "caulk": {W: 64, H: 64}}
	tile := func(m string) Tile { return sizes[m] }
	b := boxOf(t, mapfile.Vec3{10, 20, -16}, mapfile.Vec3{100, 120, 112}, "caulk")
	tx := textured(t, b, &TextureOp{Faces: "sides", Material: "brick"}, tile)
	if tx.n != 4 {
		t.Errorf("%d faces textured, the four sides", tx.n)
	}
	for dir, f := range faceTex(t, b) {
		want := "brick 64 32 0 0 0"
		if dir == "+z" || dir == "-z" {
			want = "caulk 64 64 0 0 0"
		}
		if got := fmt.Sprintf("%s %v %v %v %v %v", f.Material, f.Tex[0], f.Tex[1], f.Tex[2], f.Tex[3], f.Tex[4]); got != want {
			t.Errorf("%s: %s, want %s", dir, got, want)
		}
	}
	// Each face's own material sized, on a tile corner at the floor's level,
	// twice as large and mirrored across.
	textured(t, b, &TextureOp{Faces: "top,-x", Origin: &[3]float64{0, 0, -16}, Scale: 2, Flip: "u"}, tile)
	f := faceTex(t, b)
	if x := f["-x"]; x.Material != "brick" || x.Tex[0] != -128 || x.Tex[1] != 64 || x.Tex[3] != 16 {
		t.Errorf("-x: %s %v: its tiles start on z = -16", x.Material, x.Tex)
	}
	if z := f["+z"]; z.Material != "caulk" || z.Tex[0] != -128 || z.Tex[2] != 0 || z.Tex[3] != 0 {
		t.Errorf("top: %s %v: the origin is on its tile corners already", z.Material, z.Tex)
	}
	// A tile given outright, mirrored down.
	textured(t, b, &TextureOp{Faces: "+y", Tile: &[2]float64{32, 16}, Flip: "v"}, tile)
	if y := faceTex(t, b)["+y"]; y.Tex[0] != 32 || y.Tex[1] != -16 || y.Tex[3] != 0 {
		t.Errorf("+y: %v: 32 across, 16 down mirrored", y.Tex)
	}
	// A fit of one repeat down a trim's height keeps the tile's shape across.
	trim := boxOf(t, mapfile.Vec3{0, 0, 3}, mapfile.Vec3{256, 8, 11}, "caulk")
	textured(t, trim, &TextureOp{Faces: "-y", Material: "plank", Fit: &[2]float64{0, 1}}, tile)
	if y := faceTex(t, trim)["-y"]; y.Tex[0] != 64 || y.Tex[1] != 8 || y.Tex[2] != 0 || y.Tex[3] != 5 {
		t.Errorf("trim: %v: 8 high, so 64 across, from z = 3 (5 into a tile)", y.Tex)
	}
}

func TestTexturingAtAnAngle(t *testing.T) {
	b := boxOf(t, mapfile.Vec3{0, 0, 0}, mapfile.Vec3{128, 128, 64}, "caulk")
	textured(t, b, &TextureOp{Faces: "top", Rotate: 30, Origin: &[3]float64{10, 0, 0}}, nil)
	z := faceTex(t, b)["+z"]
	c, s := math.Cos(math.Pi/6), math.Sin(math.Pi/6)
	if z.Tex[4] != 30 || math.Abs(z.Tex[2]-wrap(10*c, 64)) > 1e-3 || math.Abs(z.Tex[3]-wrap(10*s, 64)) > 1e-3 {
		t.Errorf("top at 30°: %v", z.Tex)
	}
	if u, v := turned(mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}, -90); u != (mapfile.Vec3{0, 1, 0}) || v != (mapfile.Vec3{-1, 0, 0}) {
		t.Errorf("-90 turns u to %v, v to %v", u, v)
	}
	// A wedge's slope projects along the axis it faces most.
	wedge := brushOf(t,
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}),
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{128, 0, 0}),
	)
	tx := textured(t, wedge, &TextureOp{Faces: "top", Fit: &[2]float64{1, 1}}, nil)
	if f := faceTex(t, wedge)["+z"]; tx.n != 1 || f.Tex[0] != 128 || f.Tex[1] != 64 {
		t.Errorf("the slope, %d faces: %v spans 128 by 64 seen from above", tx.n, f.Tex)
	}
}

func TestTextureRefusals(t *testing.T) {
	for _, c := range []struct {
		op   TextureOp
		want string
	}{
		{TextureOp{Faces: "front"}, `faces "front"`},
		{TextureOp{Material: "two words"}, "one word"},
		{TextureOp{Tile: &[2]float64{64, 0}}, "two sizes above 0"},
		{TextureOp{Scale: -1}, "scale -1"},
		{TextureOp{Fit: &[2]float64{0, 0}}, "one of them above 0"},
		{TextureOp{Fit: &[2]float64{-1, 1}}, "one of them above 0"},
		{TextureOp{Flip: "x"}, `flip "x"`},
	} {
		if _, err := newTexturing(&c.op, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want %q", c.op, err, c.want)
		}
	}
	for _, f := range []string{"u", "V", "uv", "vu"} {
		if _, err := newTexturing(&TextureOp{Flip: f}, nil); err != nil {
			t.Errorf("flip %q: %v", f, err)
		}
	}
	thin := boxOf(t, mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}, "caulk")
	tx, _ := newTexturing(&TextureOp{Fit: &[2]float64{1, 0}}, func(string) Tile { return Tile{W: 64, H: 64} })
	if _, _, _, _, err := tx.fitted(nil, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}, 64, 64, 0, 0); err == nil {
		t.Error("a face with no corners has no area to fit")
	}
	if _, _, _, _, err := tx.fitted([]mapfile.Vec3{{0, 0, 0}, {0, 1, 0}, {0, 0, 1}}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}, 64, 64, 0, 0); err == nil {
		t.Error("a face with no width along u is too thin to fit")
	}
	if err := tx.brush(thin); err != nil || tx.n != 6 {
		t.Errorf("%d faces, %v", tx.n, err)
	}
}

// What a batch makes, an op after it names by its index: one undo step builds
// a box and textures its faces, at the size the tiles give.
func TestATextureOpNamesWhatAnEarlierOpMade(t *testing.T) {
	f, c, snap := viewing(t, 301)
	tiles := func(m string) (Tile, error) {
		if m == "brick" {
			return Tile{W: 128, H: 64, From: "test"}, nil
		}
		return Tile{}, fmt.Errorf("no GDT defines it")
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [0, 0, 0], "max": [256, 16, 128], "material": "brick"}},
		{"entity": {"class": "light", "origin": [0, 0, 64]}},
		{"texture": {"ids": ["@0"], "faces": "top,bottom", "material": "caulk"}},
		{"move": {"ids": ["@1"], "by": [0, 0, 8]}}
	]`), "", tiles)
	if err != nil {
		t.Fatal(err)
	}
	if got := sent(f, "edit"); len(got) != 1 || got[0] != "edit t7kb: 1 box, 1 entity, 1 move, 1 texture" {
		t.Errorf("edits = %q", got)
	}
	if len(d.Made) != 2 || made(d, 0).ID == "" || made(d, 1).Box.Min != (mapfile.Vec3{0, 0, 72}) || d.Faces[2] != 2 {
		t.Errorf("made %+v, faces %v: the box and the moved light, each under the op that made it", d.Made, d.Faces)
	}
	if d.Tiles["brick"].W != 128 || !strings.HasPrefix(d.Tiles["caulk"].From, "assumed, no GDT defines it") || d.Tiles["caulk"].W != 64 {
		t.Errorf("tiles %+v", d.Tiles)
	}
	for _, e := range d.After.Map.Entities {
		for _, br := range e.Brushes() {
			if hexOf(br.Guid()) != hexOf(made(d, 0).ID) {
				continue
			}
			got := faceTex(t, br)
			if got["+x"].Material != "brick" || got["+x"].Tex[0] != 128 || got["+z"].Material != "caulk" || got["-z"].Tex[0] != 64 {
				t.Errorf("faces %+v", got)
			}
		}
	}
	one, err := Edit(context.Background(), c, snap, ops(t, `[{"texture": {"ids": ["6e73936c"], "faces": "bottom", "material": "clip"}}]`), "", nil)
	if err != nil || len(one.Made) != 1 || one.Made[0].Class != "volume_sun" || one.Faces[0] != 1 {
		t.Errorf("one brush of an entity, its bottom: %+v, %v", one, err)
	}
	for js, want := range map[string]string{
		`[{"texture": {"ids": ["@0"]}}]`: "no earlier op 0 made",
		`[{"box": {"min": [0, 0, 0], "max": [8, 8, 8], "material": "caulk"}}, {"delete": ["@0"]}, {"move": {"ids": ["@0"], "by": [1, 0, 0]}}]`: "@0 is deleted by an earlier op",
		`[{"texture": {"ids": ["@x"]}}]`:                                                           "@ and an earlier op's index",
		`[{"texture": {"ids": ["af3c9c64"]}}]`:                                                     "none of its faces",
		`[{"texture": {"ids": ["5ec8de49"], "faces": "sideways"}}]`:                                `faces "sideways"`,
		`[{"box": {"min": [0, 0, 0], "max": [8, 8, 8], "material": "caulk"}}, {"delete": ["@0"]}]`: "the ops change nothing",
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", js, err, want)
		}
	}
}

func TestWrapKeepsTheSameTilesInSmallerNumbers(t *testing.T) {
	for _, c := range [][3]float64{{-24, 256, 232}, {1256, 256, 232}, {0, 64, 0}, {64, 64, 0}, {-64, 64, 0}, {63.9999999, 64, 0}, {12, 128, 12}} {
		if got := wrap(c[0], c[1]); got != c[2] {
			t.Errorf("wrap(%v, %v) = %v, want %v", c[0], c[1], got, c[2])
		}
	}
}

// Radiant's Fit Texture at 30° on a box x 1400..1656, y 500..628, z 0..96:
// its widths are a fit's, and its u offsets land the same tiles, but it sizes
// v along the face's bounding box's diagonal (0.5×256 − 0.866×128 on the top,
// not the face's span), which only the axes this package turns give back.
func TestAt30DegreesRadiantsFitAgreesWithTheTurnedAxes(t *testing.T) {
	b := boxOf(t, mapfile.Vec3{1400, 500, 0}, mapfile.Vec3{1656, 628, 96}, "brick")
	textured(t, b, &TextureOp{Fit: &[2]float64{1, 1}, Rotate: 30}, nil)
	radiant := map[string][4]float64{ // width, height, u offset, v offset, as Radiant wrote them
		"+z": {-285.70251, -17.148743, -251.77954, -7.3925781},
		"+x": {-158.85126, 19.138428, -43.541077, 1.2004395},
		"+y": {-269.70251, -44.861572, -136.07703, -17.785156},
	}
	faces, _ := b.Faces()
	for _, f := range faces {
		n := planeOf(f.Points).n
		dir, u, v := texAxes(mapfile.Vec3{-n[0], -n[1], -n[2]})
		r, ok := radiant[dir]
		if !ok {
			continue
		}
		_, v = turned(u, v, 30)
		var lo, hi mapfile.Vec3 // the face's bounding box
		for k := range 3 {
			lo[k], hi[k] = 1400, 1656
		}
		lo[1], hi[1], lo[2], hi[2] = 500, 628, 0, 96
		switch dir {
		case "+z":
			lo[2] = 96
		case "+x":
			lo[0] = 1656
		default:
			lo[1] = 628
		}
		diag := sub(hi, lo)
		if math.Abs(f.Tex[0]-math.Abs(r[0])) > 1e-3 || math.Abs(f.Tex[2]-wrap(r[2], math.Abs(r[0]))) > 1e-3 {
			t.Errorf("%s: width %v, u offset %v; Radiant's tiles are %v from %v", dir, f.Tex[0], f.Tex[2], math.Abs(r[0]), wrap(r[2], math.Abs(r[0])))
		}
		if h := math.Abs(dot(v, diag)); math.Abs(h-math.Abs(r[1])) > 1e-3 {
			t.Errorf("%s: v along the diagonal %v, Radiant's height %v", dir, h, r[1])
		}
		if k := (dot(v, lo) + r[3]) / r[1]; math.Abs(k-math.Round(k)) > 1e-3 {
			t.Errorf("%s: Radiant's v tiles start at the box's corner: %v tiles there", dir, k)
		}
	}
}
