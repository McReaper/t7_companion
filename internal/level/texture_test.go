package level

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
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
		n := geom.PlaneOf(f.Points).N
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
	trim := boxOf(t, mapfile.Vec3{10, 0, 3}, mapfile.Vec3{266, 8, 11}, "caulk")
	textured(t, trim, &TextureOp{Faces: "top", Material: "plank", Scale: 0.5, Flip: "uv"}, tile)
	if z := faceTex(t, trim)["+z"]; z.Tex[0] != -64 || z.Tex[1] != -8 {
		t.Errorf("top: %v: half of 128 by 16, mirrored both ways", z.Tex)
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
	// A wedge's slope projects along the axis it faces most: it is a top face.
	wedge := brushOf(t,
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}),
		face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{128, 0, 0}),
	)
	if tx := textured(t, wedge, &TextureOp{Faces: "top,+y"}, nil); tx.n != 2 {
		t.Errorf("the slope and the +y side: %d faces textured", tx.n)
	}
	// On a tie the normal goes to z, then x, then y.
	for n, want := range map[mapfile.Vec3]string{{1, 0, 1}: "+z", {0, -1, 1}: "+z", {1, 1, 0}: "+x", {-1, 1, 0}: "-x", {0, 1, -1}: "-z"} {
		if got, _, _ := texAxes(n); got != want {
			t.Errorf("texAxes(%v) = %s, want %s", n, got, want)
		}
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
		{TextureOp{Tile: &[2]float64{0, 64}}, "two sizes above 0"},
		{TextureOp{Tile: &[2]float64{-0.5, 64}}, "two sizes above 0"},
		{TextureOp{Scale: -1}, "scale -1"},
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
	for _, tl := range [][2]float64{{1, 64}, {64, 1}, {0.25, 0.25}} {
		if _, err := newTexturing(&TextureOp{Tile: &tl}, nil); err != nil {
			t.Errorf("tile %v: %v", tl, err)
		}
	}
	thin := boxOf(t, mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}, "caulk")
	tx, _ := newTexturing(&TextureOp{}, func(string) Tile { return Tile{W: 64, H: 64} })
	short := brushOf(t, " ( 0 0 0 ) short")
	if err := tx.brush(short); err == nil {
		t.Error("a face line Radiant can't have written")
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
		{"brush": {"points": [[0, 0, 0], [256, 0, 0], [0, 16, 0], [256, 16, 0], [0, 0, 128], [256, 0, 128], [0, 16, 128], [256, 16, 128]], "material": "brick"}},
		{"entity": {"class": "light", "origin": [0, 0, 64]}},
		{"texture": {"ids": ["@0"], "faces": "top,bottom", "material": "caulk"}},
		{"move": {"ids": ["@1"], "by": [0, 0, 8]}}
	]`), "", tiles)
	if err != nil {
		t.Fatal(err)
	}
	if got := sent(f, "edit"); len(got) != 1 || got[0] != "edit t7kb: 1 brush, 1 entity, 1 move, 1 texture" {
		t.Errorf("edits = %q", got)
	}
	if len(d.Made) != 2 || made(d, 0).ID == "" || made(d, 1).Box.Min != (mapfile.Vec3{0, 0, 72}) || d.Faces[2] != 2 {
		t.Errorf("made %+v, faces %v: the brush and the moved light, each under the op that made it", d.Made, d.Faces)
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
		`[{"brush": {"points": [[0, 0, 0], [8, 0, 0], [0, 8, 0], [8, 8, 0], [0, 0, 8], [8, 0, 8], [0, 8, 8], [8, 8, 8]], "material": "caulk"}}, {"delete": ["@0"]}, {"move": {"ids": ["@0"], "by": [1, 0, 0]}}]`: "@0 is deleted by an earlier op",
		`[{"texture": {"ids": ["@x"]}}]`:                            "@ and an earlier op's index",
		`[{"texture": {"ids": ["af3c9c64"]}}]`:                      "none of its faces",
		`[{"texture": {"ids": ["5ec8de49"], "faces": "sideways"}}]`: `faces "sideways"`,
		`[{"brush": {"points": [[0, 0, 0], [8, 0, 0], [0, 8, 0], [8, 8, 0], [0, 0, 8], [8, 0, 8], [0, 8, 8], [8, 8, 8]], "material": "caulk"}}, {"delete": ["@0"]}]`: "the ops change nothing",
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", js, err, want)
		}
	}
}

// A batch asks for a material's tile once; a size of 0 is no size, while a
// trim's half unit is one.
func TestTilesAreAskedOnceAndASizeOfZeroIsNone(t *testing.T) {
	_, c, snap := viewing(t, 304)
	asked := map[string]int{}
	tiles := func(m string) (Tile, error) {
		asked[m]++
		switch m {
		case "flat":
			return Tile{W: 0, H: 64}, nil
		case "low":
			return Tile{W: 64, H: 0}, nil
		}
		return Tile{W: 0.5, H: 1, From: "a trim"}, nil
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"brush": {"points": [[0, 0, 0], [8, 0, 0], [0, 8, 0], [8, 8, 0], [0, 0, 8], [8, 0, 8], [0, 8, 8], [8, 8, 8]], "material": "edge"}},
		{"brush": {"points": [[0, 0, 8], [8, 0, 8], [0, 8, 8], [8, 8, 8], [0, 0, 16], [8, 0, 16], [0, 8, 16], [8, 8, 16]], "material": "edge"}},
		{"brush": {"points": [[0, 0, 16], [8, 0, 16], [0, 8, 16], [8, 8, 16], [0, 0, 24], [8, 0, 24], [0, 8, 24], [8, 8, 24]], "material": "flat"}},
		{"brush": {"points": [[0, 0, 24], [8, 0, 24], [0, 8, 24], [8, 8, 24], [0, 0, 32], [8, 0, 32], [0, 8, 32], [8, 8, 32]], "material": "low"}}
	]`), "", tiles)
	if err != nil {
		t.Fatal(err)
	}
	if asked["edge"] != 1 || d.Tiles["edge"] != (Tile{W: 0.5, H: 1, From: "a trim"}) {
		t.Errorf("asked %v, tiles %+v", asked, d.Tiles)
	}
	if none := (Tile{W: 64, H: 64, From: "assumed, no install to size it"}); d.Tiles["flat"] != none || d.Tiles["low"] != none {
		t.Errorf("a zero size: %+v", d.Tiles)
	}
}

// A texture op on one brush of an entity leaves its other brushes; a fit on a
// face with no area refuses the batch.
func TestATextureOnOneBrushOfAnEntity(t *testing.T) {
	f, c, _ := viewing(t, 305)
	two := EntityText("trigger_multiple", nil,
		strings.Replace(BoxBrush(box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 8, 8}), "trigger", ""), "{\n", "{\n guid \"{0C000000-0000-0000-0000-000000000002}\"\n", 1),
		strings.Replace(BoxBrush(box(mapfile.Vec3{16, 0, 0}, mapfile.Vec3{24, 8, 8}), "trigger", ""), "{\n", "{\n guid \"{0C000000-0000-0000-0000-000000000003}\"\n", 1))
	edge := "{\n guid \"{0C000000-0000-0000-0000-000000000004}\"\n" +
		" " + face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n" +
		" " + face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n" +
		" " + face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 0, 1}, mapfile.Vec3{1, 0, 0}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n" +
		" " + face(mapfile.Vec3{0, 64, 0}, mapfile.Vec3{1, 64, 0}, mapfile.Vec3{0, 64, 1}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n" +
		" " + face(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{128, 0, 64}, mapfile.Vec3{128, 1, 0}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n" +
		" " + face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{128, 0, 0}) + " caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n}\n"
	f.Update(func(f *radianttest.Fake) {
		f.Text = strings.Replace(f.Text, "\"classname\" \"worldspawn\"\n", "\"classname\" \"worldspawn\"\n"+edge, 1)
		f.Text += strings.Replace(two, "{\n", "{\nguid \"{0C000000-0000-0000-0000-000000000001}\"\n", 1)
	})
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"texture": {"ids": ["0c000000-0000-0000-0000-000000000003"], "material": "clip"}}]`), "", nil)
	if err != nil || d.Faces[0] != 6 {
		t.Fatalf("%+v, %v", d, err)
	}
	hits, _ := Find(d.After.Map, d.After.IDs, Query{Class: "trigger_multiple"}, Selection{})
	if len(hits) != 1 || Material(hits[0].Entity.Brushes()[0]) != "trigger" || Material(hits[0].Entity.Brushes()[1]) != "clip" {
		t.Errorf("the trigger after: %+v", hits)
	}
}

func TestWrapKeepsTheSameTilesInSmallerNumbers(t *testing.T) {
	for _, c := range [][3]float64{{-24, 256, 232}, {1256, 256, 232}, {0, 64, 0}, {64, 64, 0}, {-64, 64, 0}, {63.9999999, 64, 0}, {12, 128, 12},
		{-0.5, 64, 63.5}, {0.5, 64, 0.5}, {63.5, 64, 63.5}} {
		if got := wrap(c[0], c[1]); got != c[2] {
			t.Errorf("wrap(%v, %v) = %v, want %v", c[0], c[1], got, c[2])
		}
	}
}

// A side wall's texture runs as Radiant lays it, u along -y on an x face and
// along -x on a y face, so its tiles start on the origin across the wall: an
// origin 40 in from the corner puts the u offset at 40 on all four sides.
func TestASideWallsTilesStartOnTheOrigin(t *testing.T) {
	b := boxOf(t, mapfile.Vec3{0, 0, 0}, mapfile.Vec3{128, 128, 64}, "caulk")
	textured(t, b, &TextureOp{Faces: "sides", Origin: &[3]float64{40, 40, 0}}, nil)
	for dir, f := range faceTex(t, b) {
		if dir == "+z" || dir == "-z" {
			continue
		}
		if f.Tex[2] != 40 || f.Tex[3] != 0 {
			t.Errorf("%s: offsets %v %v, want 40 0", dir, f.Tex[2], f.Tex[3])
		}
	}
}
