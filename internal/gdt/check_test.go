package gdt

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// put writes a file under the workspace root (before the index is first used,
// or before a refresh is due).
func put(t *testing.T, w *Workspace, rel string, body []byte) {
	t.Helper()
	p := filepath.Join(w.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func pngOf(t *testing.T, wd, ht int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, wd, ht))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// tiffOf is a header-only little-endian TIFF: enough for imageSize.
func tiffOf(wd, ht int) []byte {
	b := []byte("II*\x00")
	b = binary.LittleEndian.AppendUint32(b, 8)
	b = binary.LittleEndian.AppendUint16(b, 2)
	for _, e := range [][2]int{{256, wd}, {257, ht}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(e[0]))
		b = binary.LittleEndian.AppendUint16(b, 4) // LONG
		b = binary.LittleEndian.AppendUint32(b, 1)
		b = binary.LittleEndian.AppendUint32(b, uint32(e[1]))
	}
	return append(b, 0, 0, 0, 0)
}

func reportFor(r *CheckResult, asset string) *AssetReport {
	for i := range r.Assets {
		if r.Assets[i].Asset == asset {
			return &r.Assets[i]
		}
	}
	return nil
}

func hasIssue(rep *AssetReport, field, level, text string) bool {
	if rep == nil {
		return false
	}
	for _, is := range rep.Issues {
		if is.Field == field && is.Level == level && strings.Contains(is.Msg, text) {
			return true
		}
	}
	return false
}

func TestCheckFindsBrokenReferences(t *testing.T) {
	w := fixture(t)
	put(t, w, "model_export/mine/ok.xmodel_bin", []byte("x"))
	put(t, w, "source_data/check.gdt", []byte("{\r\n"+
		"\t\"good_model\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t\t\"filename\" \"mine\\\\ok.xmodel_bin\"\r\n\t}\r\n"+
		"\t\"bad_model\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t\t\"filename\" \"mine\\\\missing.xmodel_bin\"\r\n\t}\r\n"+
		"\t\"wrong_kind\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"colorMap\" \"good_model\"\r\n\t}\r\n"+
		"\t\"ghost_ref\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"colorMap\" \"not_anywhere\"\r\n\t}\r\n"+
		"\t\"builtin\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"colorMap\" \"$white_diffuse\"\r\n\t}\r\n"+
		"\t\"orphan\" [ \"no_such_parent\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"my_img\" ( \"image.gdf\" )\r\n\t{\r\n\t}\r\n"+
		"}\r\n"))
	r, err := w.Check("source_data/check.gdt", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Checked != 7 {
		t.Fatalf("checked %d assets, want 7", r.Checked)
	}
	if reportFor(r, "good_model") != nil || reportFor(r, "builtin") != nil {
		t.Fatalf("clean assets reported: %+v", r.Assets)
	}
	for _, c := range []struct{ asset, field, level, text string }{
		{"bad_model", "filename", "error", "model_export/mine/missing.xmodel_bin"},
		{"wrong_kind", "colorMap", "error", "expects a image"},
		{"ghost_ref", "colorMap", "warning", "in no GDT"},
		{"orphan", "", "error", "parent"},
		{"my_img", "", "error", "more than once"},
	} {
		if !hasIssue(reportFor(r, c.asset), c.field, c.level, c.text) {
			t.Errorf("%s: missing %s on %q containing %q: %+v", c.asset, c.level, c.field, c.text, reportFor(r, c.asset))
		}
	}
	if one, err := w.Check("source_data/check.gdt", "bad_model"); err != nil || one.Checked != 1 {
		t.Fatalf("single-asset check: %+v %v", one, err)
	}
}

func TestReferencedBy(t *testing.T) {
	w := fixture(t)
	hits, _, err := w.ReferencedBy("mine_base")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Asset != "child_mtl" || hits[0].Field != "[parent]" {
		t.Fatalf("parent reference: %+v", hits)
	}
	hits, _, _ = w.ReferencedBy("my_img")
	if len(hits) != 1 || hits[0].Asset != "child_mtl" || hits[0].Field != "colorMap" || hits[0].File != "source_data/mine.gdt" {
		t.Fatalf("field reference: %+v", hits)
	}
}

func TestEditBatchIsAllOrNothing(t *testing.T) {
	w := fixture(t)
	bad := []EditRequest{
		{Asset: "base_mtl", Type: "material", Set: map[string]string{"materialType": "lit"}},
		{Asset: "bad_mtl", Parent: "base_mtl", Set: map[string]string{"noCastShadow": "maybe"}},
	}
	br, err := w.EditBatch("source_data/batch.gdt", bad, false)
	if err != nil {
		t.Fatal(err)
	}
	if br.Written || br.Errors == 0 {
		t.Fatalf("a batch with an error must write nothing: %+v", br)
	}
	if _, err := os.Stat(filepath.Join(w.Root, "source_data", "batch.gdt")); !os.IsNotExist(err) {
		t.Fatal("file created despite the error")
	}

	good := []EditRequest{
		{Asset: "base_mtl", Type: "material", Set: map[string]string{"materialType": "lit"}},
		{Asset: "batch_img", Type: "image", Set: map[string]string{"semantic": "diffuseMap"}},
		{Asset: "red_mtl", Parent: "base_mtl", Set: map[string]string{"colorMap": "batch_img"}},
		{Asset: "red_copy", CopyFrom: "red_mtl"},
	}
	br, err = w.EditBatch("source_data/batch.gdt", good, false)
	if err != nil {
		t.Fatal(err)
	}
	if !br.Written || len(br.Results) != 4 {
		t.Fatalf("batch write: %+v", br)
	}
	if red := br.Results[2]; red.Type != "material" || len(red.Issues) != 0 {
		t.Fatalf("a derived asset must resolve its parent, and its image, from the same batch: %+v", red)
	}
	f, _ := w.Load("source_data/batch.gdt")
	cp := f.Find("red_copy")
	if mt, _ := cp.Get("materialType"); mt != "lit" || cp.Type != "material" || cp.Parent != "" {
		t.Fatalf("copy of an in-batch derived asset: %+v", cp)
	}
	for _, n := range []string{"base_mtl", "batch_img", "red_mtl", "red_copy"} {
		if locs, _ := w.Find(n); len(locs) != 1 {
			t.Fatalf("%s not indexed after the batch: %v", n, locs)
		}
	}
}

func TestImageFromTexture(t *testing.T) {
	w := fixture(t)
	put(t, w, "stock.gdtdef", []byte("model_export/stock.gdt\nsource_data/stock_img.gdt\n"))
	put(t, w, "source_data/stock_img.gdt", []byte("{\r\n\t\"stock_c\" ( \"image.gdf\" )\r\n\t{\r\n"+
		"\t\t\"baseImage\" \"texture_assets\\\\stock\\\\c.tif\"\r\n\t\t\"semantic\" \"diffuseMap\"\r\n\t\t\"compressionMethod\" \"compressed high color\"\r\n\t}\r\n}\r\n"))
	put(t, w, "texture_assets/mine/wall_c.png", pngOf(t, 256, 128))
	put(t, w, "texture_assets/mine/wall_n.tif", tiffOf(512, 512))
	put(t, w, "texture_assets/mine/odd.tif", tiffOf(300, 256))

	// semantic derived from the techset slot the image will fill
	r, err := w.Edit(EditRequest{File: "source_data/img.gdt", Asset: "wall_c", DryRun: true,
		Image: &ImageSpec{Texture: "texture_assets/mine/wall_c.png", MaterialType: "lit", Field: "colorMap"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Type != "image" || len(r.Issues) != 0 {
		t.Fatalf("image from slot: %+v", r)
	}
	r, err = w.Edit(EditRequest{File: "source_data/img.gdt", Asset: "wall_c",
		Image: &ImageSpec{Texture: "texture_assets/mine/wall_c.png", MaterialType: "lit", Field: "colorMap"}})
	if err != nil || !r.Written {
		t.Fatalf("image write: %+v %v", r, err)
	}
	f, _ := w.Load("source_data/img.gdt")
	a := f.Find("wall_c")
	if v, _ := a.Get("baseImage"); v != `texture_assets\\mine\\wall_c.png` {
		t.Fatalf("baseImage = %q", v)
	}
	if v, _ := a.Get("compressionMethod"); v != "compressed high color" {
		t.Fatal("settings must come from a stock image of the same semantic")
	}

	if _, err := w.Edit(EditRequest{File: "source_data/img.gdt", Asset: "x", DryRun: true,
		Image: &ImageSpec{Texture: "texture_assets/mine/wall_n.tif", MaterialType: "lit", Field: "normalMap"}}); err == nil ||
		!strings.Contains(err.Error(), "no texture slot") {
		t.Fatalf("a slot the techset doesn't have must be refused: %v", err)
	}
	r, err = w.Edit(EditRequest{File: "source_data/img.gdt", Asset: "odd", DryRun: true,
		Image: &ImageSpec{Texture: "texture_assets/mine/odd.tif", Semantic: "diffuseMap"}})
	if err != nil || !hasIssue(&AssetReport{Issues: r.Issues}, "baseImage", "error", "300x256") {
		t.Fatalf("non power-of-two TIFF: %+v %v", r, err)
	}
	r, _ = w.Edit(EditRequest{File: "source_data/img.gdt", Asset: "gone", DryRun: true,
		Image: &ImageSpec{Texture: "texture_assets/mine/nope.tif", Semantic: "diffuseMap"}})
	if !hasIssue(&AssetReport{Issues: r.Issues}, "baseImage", "error", "does not exist") {
		t.Fatalf("missing texture: %+v", r.Issues)
	}
}

func TestColorValidation(t *testing.T) {
	w := fixture(t)
	put(t, w, "deffiles/fx.awi", []byte(`	Asset.AddEntry_Color( "tint", 1, 1, 1, 1 );`))
	for val, bad := range map[string]bool{"1 0.5 0 1": false, "1 0 0": true, "red": true, "": false} {
		r, err := w.Edit(EditRequest{File: "source_data/fx.gdt", Asset: "fx", Type: "fx", DryRun: true, Set: map[string]string{"tint": val}})
		if err != nil {
			t.Fatal(err)
		}
		if got := issueFor(r, "tint", "error"); got != bad {
			t.Errorf("tint %q: error=%v, want %v (%+v)", val, got, bad, r.Issues)
		}
	}
}

func TestIndexSeesFilesChangedDuringSession(t *testing.T) {
	w := fixture(t)
	if locs, _ := w.Find("late_asset"); len(locs) != 0 {
		t.Fatal("not there yet")
	}
	// APE saves an indexed GDT behind our back: the re-stat must pick it up.
	p := filepath.Join(w.Root, "source_data", "mine.gdt")
	b, _ := os.ReadFile(p)
	b = bytes.Replace(b, []byte("}\r\n}\r\n"), []byte("}\r\n\t\"late_asset\" ( \"image.gdf\" )\r\n\t{\r\n\t}\r\n}\r\n"), 1)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(p, future, future)
	w.refreshMu.Lock()
	w.lastStat = time.Time{}
	w.refreshMu.Unlock()
	if locs, _ := w.Find("late_asset"); len(locs) != 1 {
		t.Fatalf("changed GDT not re-read: %v", locs)
	}
}

func TestCheckToleratesConverterOutput(t *testing.T) {
	w := fixture(t)
	put(t, w, "deffiles/image.awi", []byte("\tAsset.AddEntry_Texture( \"baseImage\", \"\" );\n\tAsset.AddEntry_Combo( \"semantic\", \"diffuseMap | normalMap\" );\n"))
	put(t, w, "texture_assets/a.png", pngOf(t, 4, 4))
	put(t, w, "source_data/conv.gdt", []byte("{\r\n"+
		"\t\"conv_mtl\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"stencil\" \"Disable*\"\r\n\t}\r\n"+
		"\t\"conv_img\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"baseImage\" \"texture_assets\\a.png\"\r\n\t\t\"semantic\" \"diffuseMap\"\r\n\t}\r\n}\r\n"))
	r, err := w.Check("source_data/conv.gdt", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Assets) != 0 {
		t.Fatalf("a combo value with APE's default marker, or a texture path, is not an issue: %+v", r.Assets)
	}
}

func TestSchemaFactsFromTheRealInstall(t *testing.T) {
	w := fixture(t)
	put(t, w, "deffiles/bundle.awi", []byte(`
	Asset.AddEntry_Combo( "type", "scene" );
	Asset.AddEntry_Combo( prefix + "Stance", "Stand | Crouch | Prone" );
	Asset.AddEntry_AssetCombo( prefix + "_Model", "xmodel | character | aitype" );
	if ( action == "weapon" ) Asset.AddEntry_AssetCombo( id + "param", "bulletweapon" );
	if ( action == "fx" ) Asset.AddEntry_Path( id + "param", "" ).SetRelativePath( "share/raw/fx/" );
	Asset.AddEntry_Float( "dist", 0, 0, 100 );
	Asset.AddEntry_CheckBox( "flag", false );
	Asset.AddEntry_Combo( "flap", "Pitch{0} | Yaw{1}" );
`))
	put(t, w, "source_data/b.gdt", []byte("{\r\n\t\"b\" ( \"bundle.gdf\" )\r\n\t{\r\n"+
		"\t\t\"Type\" \"scene\"\r\n\t\t\"o1_Stance\" \"stand\"\r\n\t\t\"o1_Model\" \"stock_model\"\r\n\t\t\"n0param\" \"my_img\"\r\n\t\t\"dist\" \"\"\r\n\t\t\"flag\" \"True\"\r\n\t\t\"flap\" \"1\"\r\n\t}\r\n}\r\n"))
	s, _ := w.Schema("bundle")
	if e := s.Lookup("n0param"); e == nil || !e.Varies {
		t.Fatalf("a field declared in several script branches with different kinds must be Varies: %+v", e)
	}
	if e := s.Lookup("Type"); e == nil || e.Name != "type" {
		t.Fatalf("field names match case-insensitively: %+v", e)
	}
	r, err := w.Check("source_data/b.gdt", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Assets) != 0 {
		t.Fatalf("case, multi-type targets, branch-dependent fields and empty numbers are not issues: %+v", r.Assets)
	}
}

func TestNamesArePerType(t *testing.T) {
	w := fixture(t)
	// my_img is an image in mine.gdt: a material of the same name is not a duplicate
	r, err := w.Edit(EditRequest{File: "source_data/pt.gdt", Asset: "my_img", Type: "material", Set: map[string]string{"materialType": "lit"}})
	if err != nil || !r.Written {
		t.Fatalf("a material may share an image's name: %+v %v", r, err)
	}
	c, err := w.Check("source_data/pt.gdt", "")
	if err != nil || len(c.Assets) != 0 {
		t.Fatalf("an image and a material sharing a name are not a duplicate: %+v %v", c, err)
	}
	if _, err := w.Edit(EditRequest{File: "source_data/pt.gdt", Asset: "my_img", DryRun: true, CopyFrom: "stock_mtl"}); err != nil {
		t.Fatalf("editing the existing material: %v", err)
	}
	if _, err := w.Edit(EditRequest{File: "source_data/pt2.gdt", Asset: "my_img", Type: "image"}); err == nil || !strings.Contains(err.Error(), "Duplicate 'image'") {
		t.Fatalf("a second image named my_img must be refused: %v", err)
	}
	if _, err := w.Edit(EditRequest{File: "source_data/mine.gdt", Asset: "my_img", DryRun: true}); err != nil {
		t.Fatalf("my_img exists in mine.gdt: %v", err)
	}
	if _, err := w.Edit(EditRequest{File: "source_data/pt2.gdt", Asset: "x_mtl", Parent: "stock_mtl"}); err == nil ||
		!strings.Contains(err.Error(), "same GDT") {
		t.Fatalf("a parent from another GDT must be refused: %v", err)
	}
	if d, _ := w.Duplicates("my_img"); len(d) != 0 {
		t.Fatalf("Duplicates: %v", d)
	}
}

func TestParentMustBeInTheSameGDT(t *testing.T) {
	w := fixture(t)
	put(t, w, "source_data/orphan.gdt", []byte("{\r\n\t\"far_child\" [ \"stock_mtl\" ]\r\n\t{\r\n\t}\r\n}\r\n"))
	r, err := w.Check("source_data/orphan.gdt", "")
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(reportFor(r, "far_child"), "", "error", "is only in model_export/stock.gdt") {
		t.Fatalf("a parent defined only in another GDT is gdtdb's `Parent Entity does not exist` error: %+v", r.Assets)
	}
}
