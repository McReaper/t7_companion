package level

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// madeBrushes are the brushes op n made, read back from the map.
func madeBrushes(t *testing.T, d *Done, n int) (*mapfile.Entity, []*mapfile.Brush) {
	t.Helper()
	id := hexOf(made(d, n).ID)
	for _, e := range d.After.Map.Entities {
		if hexOf(e.Guid()) == id {
			return e, e.Brushes()
		}
		for _, br := range e.Brushes() {
			if hexOf(br.Guid()) == id {
				return nil, []*mapfile.Brush{br}
			}
		}
	}
	t.Fatalf("op %d's %q not in the map read back", n, id)
	return nil, nil
}

// A brush is the hull of its corners: a box's 8 give 6 faces, a wedge's 6
// give 5, a hexagonal column's 12 give 8, a corner inside the hull adds none;
// each face winds into the brush (its bounds are the corners'), in the
// material at its tile; with a class, it is a new entity's.
func TestABrushIsTheHullOfItsCorners(t *testing.T) {
	_, c, snap := viewing(t, 640)
	var hex []string
	for k := range 6 {
		x, y := 64*math.Cos(float64(k)*math.Pi/3), 64*math.Sin(float64(k)*math.Pi/3)
		hex = append(hex, fmt.Sprintf("[%.3f, %.3f, 0], [%.3f, %.3f, 96]", x, y, x, y))
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"brush": {"points": [[0, 0, 0], [64, 0, 0], [0, 32, 0], [64, 32, 0], [0, 0, 16], [64, 0, 16], [0, 32, 16], [64, 32, 16], [32, 16, 8]], "material": "brick"}},
		{"brush": {"points": [[0, 0, 0], [128, 0, 0], [0, 64, 0], [128, 64, 0], [0, 0, 64], [0, 64, 64]], "material": "concrete", "contents": "detail"}},
		{"brush": {"points": [`+strings.Join(hex, ", ")+`], "material": "metal", "class": "func_group"}}
	]`), "", func(string) (Tile, error) { return Tile{W: 128, H: 128, From: "test"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range []struct {
		faces    int
		material string
		lo, hi   mapfile.Vec3
	}{{6, "brick", mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 32, 16}}, {5, "concrete", mapfile.Vec3{0, 0, 0}, mapfile.Vec3{128, 64, 64}}, {8, "metal", mapfile.Vec3{-64, -55.426, 0}, mapfile.Vec3{64, 55.426, 96}}} {
		e, brs := madeBrushes(t, d, n)
		faces, err := brs[0].Faces()
		box := geom.BrushBox(brs[0])
		if err != nil || len(faces) != want.faces || !vecNear(box.Min, want.lo) || !vecNear(box.Max, want.hi) {
			t.Errorf("op %d: %d faces (%v), bounds %v-%v; want %d, %v-%v", n, len(faces), err, box.Min, box.Max, want.faces, want.lo, want.hi)
			continue
		}
		for _, f := range faces {
			if f.Material != want.material || f.Tex[0] != 128 {
				t.Errorf("op %d: a face %s %v, want %s at 128", n, f.Material, f.Tex, want.material)
			}
		}
		if (n == 2) != (e != nil && e.Class() == "func_group") {
			t.Errorf("op %d: entity %v", n, e)
		}
		if n == 1 && brs[0].Contents() != "detail" {
			t.Errorf("the wedge's contents %q", brs[0].Contents())
		}
	}
}

// A brush refuses corners that enclose nothing, too few or too many, a
// material of two words, contents Treyarch's brushes don't carry, keys
// without a class.
func TestBrushRefusals(t *testing.T) {
	_, c, snap := viewing(t, 641)
	var many []string
	for k := range 65 {
		many = append(many, fmt.Sprintf("[%d, %d, %d]", k, k*k%7, k%3))
	}
	cube := `[[0, 0, 0], [8, 0, 0], [0, 8, 0], [8, 8, 0], [0, 0, 8], [8, 0, 8], [0, 8, 8], [8, 8, 8]]`
	for js, want := range map[string]string{
		`[{"brush": {"points": [[0, 0, 0], [8, 0, 0], [0, 8, 0]], "material": "x"}}]`:                 "3 points: 4 to 64 corners",
		`[{"brush": {"points": [` + strings.Join(many, ", ") + `], "material": "x"}}]`:                "65 points",
		`[{"brush": {"points": [[0, 0, 0], [8, 0, 0], [0, 8, 0], [8, 8, 0]], "material": "x"}}]`:      "enclose no volume",
		`[{"brush": {"points": [[0, 0, 0], [1, 1, 1], [2, 2, 2], [3, 3, 3]], "material": "x"}}]`:      "enclose no volume",
		`[{"brush": {"points": ` + cube + `, "material": "two words"}}]`:                              `material "two words"`,
		`[{"brush": {"points": ` + cube + `, "material": "x", "contents": "solid"}}]`:                 `"solid" is not one of`,
		`[{"brush": {"points": ` + cube + `, "material": "x", "keys": {"a": "b"}}}]`:                  "keys belong to an entity: give its class",
		`[{"brush": {"points": ` + cube + `, "material": "x", "class": "t", "keys": {"guid": "x"}}}]`: `key "guid"`,
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.90s: %v, want %q", js, err, want)
		}
	}
}

// A patch is its control points as Radiant writes them: a curve of 3 by 3, a
// mesh of 2 by 4, row by row, their texture along the rows and columns at 16
// a unit, in a new entity with a class.
func TestAPatchIsItsControlPoints(t *testing.T) {
	_, c, snap := viewing(t, 642)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"patch": {"rows": [[[0, 0, 0], [0, 0, 32], [0, 0, 64]], [[32, 0, 0], [32, 32, 32], [32, 0, 64]], [[64, 0, 0], [64, 0, 32], [64, 0, 64]]], "material": "metal", "contents": "nonColliding"}},
		{"patch": {"rows": [[[0, 0, 0], [16, 0, 0], [32, 0, 0], [48, 0, 0]], [[0, 16, 4], [16, 16, 4], [32, 16, 4], [48, 16, 4]]], "material": "grass", "mesh": true, "class": "func_group"}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, curve := madeBrushes(t, d, 0)
	p, err := curve[0].Patch()
	if err != nil || p.Kind != mapfile.KindCurve || p.Material != "metal" || p.Dims != [4]int{3, 3, 16, 8} || len(p.Rows) != 3 || curve[0].Contents() != "nonColliding" {
		t.Fatalf("the curve: %+v, %v, contents %q", p, err, curve[0].Contents())
	}
	diag := dist(mapfile.Vec3{0, 0, 32}, mapfile.Vec3{32, 32, 32}) * 16 // across from row 0's, along from its row's first
	if v := p.Rows[1][1]; v.Pos != (mapfile.Vec3{32, 32, 32}) || math.Abs(v.UV[0]-diag) > 0.001 || math.Abs(v.UV[1]+diag) > 0.001 {
		t.Errorf("the middle control point %+v", v)
	}
	e, mesh := madeBrushes(t, d, 1)
	q, err := mesh[0].Patch()
	if err != nil || q.Kind != mapfile.KindMesh || q.Dims != [4]int{2, 4, 0, 8} || e == nil || e.Class() != "func_group" {
		t.Fatalf("the mesh: %+v, %v, %v", q, err, e)
	}
	if v := q.Rows[1][3]; math.Abs(v.UV[0]-dist(mapfile.Vec3{48, 0, 0}, mapfile.Vec3{48, 16, 4})*16) > 0.001 || v.UV[1] != -48*16 {
		t.Errorf("the mesh's last control point %+v", v)
	}
}

// A patch refuses rows of different lengths, a curve's even counts, too few.
func TestPatchRefusals(t *testing.T) {
	_, c, snap := viewing(t, 643)
	row3 := `[[0, 0, 0], [8, 0, 0], [16, 0, 0]]`
	for js, want := range map[string]string{
		`[{"patch": {"rows": [], "material": "x"}}]`:                                                             "no rows",
		`[{"patch": {"rows": [` + row3 + `, ` + row3 + `], "material": "x"}}]`:                                   "3 each way at least",
		`[{"patch": {"rows": [` + row3 + `, ` + row3 + `, [[0, 0, 0]]], "material": "x"}}]`:                      "row 2 has 1 points",
		`[{"patch": {"rows": [` + row3 + `, ` + row3 + `, ` + row3 + `, ` + row3 + `], "material": "x"}}]`:       "odd counts each way",
		`[{"patch": {"rows": [[[0, 0, 0]], [[0, 8, 0]]], "material": "x", "mesh": true}}]`:                       "2 each way at least",
		`[{"patch": {"rows": [` + row3 + `, ` + row3 + `], "material": "", "mesh": true}}]`:                      `material ""`,
		`[{"patch": {"rows": [` + row3 + `, ` + row3 + `], "material": "x", "mesh": true, "keys": {"a": "b"}}}]`: "give its class",
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.90s: %v, want %q", js, err, want)
		}
	}
}

// Raw text lands as written: an entity with its brush and patch, and a
// worldspawn's brush as a world brush, their GUIDs given anew.
func TestRawTextLandsAsWritten(t *testing.T) {
	_, c, snap := viewing(t, 644)
	text := "{\n\"classname\" \"func_group\"\n// brush 0\n{\n guid \"{11111111-0000-0000-0000-000000000000}\"\n" + BoxBrush(geom.Box{Min: mapfile.Vec3{0, 0, 0}, Max: mapfile.Vec3{16, 16, 16}, Ok: true}, "brick", "")[2:] +
		"{\n curve\n {\n  metal\n  lightmap_gray\n  3 3 16 8\n  (\n   v 0 0 0 t 0 0 1 1\n   v 0 0 8 t 0 -128 1 3\n   v 0 0 16 t 0 -256 1 5\n  )\n  (\n   v 8 0 0 t 128 0 3 1\n   v 8 8 8 t 128 -128 3 3\n   v 8 0 16 t 128 -256 3 5\n  )\n  (\n   v 16 0 0 t 256 0 5 1\n   v 16 0 8 t 256 -128 5 3\n   v 16 0 16 t 256 -256 5 5\n  )\n }\n}\n}\n" +
		"{\n\"classname\" \"worldspawn\"\n" + BoxBrush(geom.Box{Min: mapfile.Vec3{100, 0, 0}, Max: mapfile.Vec3{116, 16, 16}, Ok: true}, "concrete", "") + "}\n"
	raw, _ := json.Marshal(text)
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"raw": `+string(raw)+`}]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var group *mapfile.Entity
	var world *mapfile.Brush
	for _, m := range d.Made {
		for _, e := range d.After.Map.Entities {
			if hexOf(e.Guid()) == hexOf(m.ID) {
				group = e
			}
			for _, br := range e.Brushes() {
				if e.Class() == "worldspawn" && hexOf(br.Guid()) == hexOf(m.ID) {
					world = br
				}
			}
		}
	}
	if len(d.Made) != 2 || group == nil || group.Class() != "func_group" || len(group.Brushes()) != 2 || group.Brushes()[1].Kind() != mapfile.KindCurve {
		t.Fatalf("made %+v, the group %v", d.Made, group)
	}
	if world == nil || Material(world) != "concrete" {
		t.Errorf("the world brush %v", world)
	}
	for _, br := range group.Brushes() {
		if strings.Contains(br.Guid(), "11111111") {
			t.Errorf("the text's GUID was kept: %s", br.Guid())
		}
	}
}

// Raw text must read: an entity at least, each brush 4 faces or more or a
// patch.
func TestRawRefusals(t *testing.T) {
	_, c, snap := viewing(t, 645)
	three := "{\n\"classname\" \"func_group\"\n{\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) x 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n ( 0 0 8 ) ( 0 1 8 ) ( 1 0 8 ) x 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n ( 0 0 0 ) ( 0 1 0 ) ( 0 0 1 ) x 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n}\n}\n"
	for text, want := range map[string]string{
		"":                   "no entity",
		"// nothing\n":       "no entity",
		three:                "a brush of 3 faces",
		"{\n\"classname\"\n": "raw:",
	} {
		raw, _ := json.Marshal(text)
		if _, err := Edit(context.Background(), c, snap, ops(t, `[{"raw": `+string(raw)+`}]`), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", text, err, want)
		}
	}
}

func vecNear(a, b mapfile.Vec3) bool {
	for k := range 3 {
		if math.Abs(a[k]-b[k]) > 0.01 {
			return false
		}
	}
	return true
}

// Raw text goes to the import without its GUIDs: the import gives new ones,
// and a GUID the map has would name two things.
func TestRawDropsTheTextsGUIDs(t *testing.T) {
	_, _, snap := viewing(t, 646)
	b := newBatch(snap, nil)
	text := "{\nguid \"{22222222-0000-0000-0000-000000000000}\"\n\"classname\" \"func_group\"\n{\n guid \"{33333333-0000-0000-0000-000000000000}\"\n" +
		BoxBrush(geom.Box{Min: mapfile.Vec3{0, 0, 0}, Max: mapfile.Vec3{8, 8, 8}, Ok: true}, "brick", "")[2:] + "}\n"
	if err := b.raw(0, text); err != nil {
		t.Fatal(err)
	}
	if got := string(b.units[len(b.units)-1].ent.Bytes()); strings.Contains(got, "guid") {
		t.Errorf("sent with its GUIDs: %s", got)
	}
}
