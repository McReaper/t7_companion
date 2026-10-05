package gdt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixture builds a tiny mod-tools root: a material + xmodel deffile, a lit
// techset with an include, a stock GDT and a user GDT.
func fixture(t *testing.T) *Workspace {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir()) // keep the index cache out of the real profile
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("deffiles/material.awi", `
void GenerateUI( asset Asset )
{
	// Asset.AddEntry_Float( "commentedOut", 1, 0, 2 );
	Asset.AddEntry_Combo( "materialCategory", materialCategories ).SetDefaultValue( "Geometry" );
	Asset.AddEntry_Combo( "materialType", MaterialTypes ).SetTitle( "Material Type" );
	Asset.AddEntry_Combo( "stencil", "Disable* | One-sided | Two-sided" ).Show( false );
	Asset.AddEntry_Float( "normalHeightScale", 1, 0, 1 ).SetTitle( "Normal Height" ).Show( false );
	Asset.AddEntry_CheckBox( "noCastShadow", false );
	Asset.AddEntry_AssetCombo( "colorMap", "image" ).SetTitle( "Color Map" );
	Asset.AddEntry_AssetCombo( "specColorMap", "image" );
	Asset.AddEntry_Vector2( "glossRangeMin", "glossRangeMax", 0.0, 13.0, 0.0, 17.0 ).SetTitle( "Gloss Range" );
	Asset.AddEntry_Int( "layer" + i, 0, 0, 4 );
	Asset.AddEntry_AssetCombo( prefix + "viewModel" + suffix, "xmodel" );
	Asset.AddEntry_Label( "decoration", "not a field" );
}
`)
	write("deffiles/xmodel.awi", `
	Asset.AddEntry_Path( "filename", "" );
	Asset.AddEntry_Path( "mediumLod", "" );
	Asset.AddEntry_Path( "lowLod", "" );
	Asset.AddEntry_CheckBox( "autogenLod" + lodIndex, false );
	Asset.AddEntry_Int( "autogenLod" + lodIndex + "Percent", 13, 0, 100 );
`)
	write("deffiles/image.awi", `	Asset.AddEntry_Combo( "semantic", "diffuseMap | normalMap | effectMap" );`)
	write("share/raw/techsetdefs_stable/include/color_base.techsetdef", `
Texture( "colorMap" )
{
	image = Image( <colorMap, $white_diffuse> )
	semantic = "diffuseMap"
	usage = "diffuse map"
}
`)
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", `#include "color_base"

Globals()
{
	category = "Geometry"
	renderFlags = "lit deferred opaque"
}

float2( "glossRange" )
{
	x = <glossRangeMin>
	y = <glossRangeMax>
}

Technique( "gbuffer" )
{
	source = "gbuffer_lit.hlsl"
}
`)
	write("share/raw/techsetdefs_stable/geometry_advanced/lit_advanced.techsetdef", "Globals()\n{\n\tcategory = \"Geometry Advanced\"\n}\n")
	write("bin/converter_gdt_dirs_0.txt", "source_data\nmodel_export\n")
	write("stock.gdtdef", "model_export/stock.gdt\n")
	write("model_export/stock.gdt", "{\r\n\t\"stock_mtl\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n\t\"stock_model\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t\t\"filename\" \"a\\\\b.xmodel_bin\"\r\n\t\t\"lowLod\" \"donor\\\\low.xmodel_bin\"\r\n\t\t\"mediumLod\" \"donor\\\\med.xmodel_bin\"\r\n\t}\r\n}\r\n")
	write("source_data/mine.gdt", "{\r\n\t\"my_img\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"semantic\" \"normalMap\"\r\n\t}\r\n\t\"mine_base\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n\t\"child_mtl\" [ \"mine_base\" ]\r\n\t{\r\n\t\t\"colorMap\" \"my_img\"\r\n\t}\r\n}\r\n")
	w, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestParseRoundTripKeepsCRLFAndEscapes(t *testing.T) {
	src := "{\r\n\t\"m\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t\t\"filename\" \"a\\\\b.xmodel_bin\"\r\n\t}\r\n\t\"d\" [ \"m\" ]\r\n\t{\r\n\t}\r\n}\r\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(f.Bytes()); got != src {
		t.Fatalf("round trip changed the file:\n%q\n%q", src, got)
	}
	v, _ := f.Assets[0].Get("filename")
	if Unquote(v) != `a\b.xmodel_bin` || Quote(`a\b`) != `a\\b` {
		t.Fatalf("escaping: %q", v)
	}
	if f.Assets[1].Parent != "m" {
		t.Fatalf("derived asset parent = %q", f.Assets[1].Parent)
	}
}

func TestParseRejectsUnbalanced(t *testing.T) {
	if _, err := Parse([]byte("{\n\t\"m\" ( \"material.gdf\" )\n\t{\n\t\t\"k\" \"v\"\n}\n")); err == nil {
		t.Fatal("expected an error for a missing closing brace")
	}
}

func TestSchema(t *testing.T) {
	w := fixture(t)
	s, err := w.Schema("material")
	if err != nil {
		t.Fatal(err)
	}
	if s.Entries["commentedOut"] != nil || s.Entries["decoration"] != nil {
		t.Fatal("commented-out and UI-only entries must be skipped")
	}
	st := s.Entries["stencil"]
	if st == nil || st.Default != "Disable" || len(st.Options) != 3 || !st.Hidden {
		t.Fatalf("combo parse: %+v", st)
	}
	nh := s.Entries["normalHeightScale"]
	if nh == nil || *nh.Min != 0 || *nh.Max != 1 || nh.Title != "Normal Height" {
		t.Fatalf("float parse: %+v", nh)
	}
	if s.Entries["colorMap"].AssetType != "image" {
		t.Fatal("asset combo target type")
	}
	if s.Lookup("glossRangeMax") == nil {
		t.Fatal("Vector2 must declare both of its fields")
	}
	if e := s.Lookup("layer3"); e == nil || !e.Pattern {
		t.Fatal("runtime-built names must match as a prefix pattern")
	}
	if e := s.Lookup("attach2_viewModel_b"); e == nil || e.AssetType != "xmodel" {
		t.Fatal("a name with a runtime prefix must match its literal core anywhere")
	}
	x, _ := w.Schema("xmodel")
	if e := x.Lookup("autogenLod4Percent"); e == nil || e.Kind != "Int" || e.Name != "autogenLod*Percent" {
		t.Fatalf("the most specific runtime pattern must win: %+v", e)
	}
	if e := x.Lookup("autogenLod4"); e == nil || e.Kind != "CheckBox" {
		t.Fatalf("autogenLod4: %+v", e)
	}
	if s.Entries["materialCategory"].Default != "Geometry" {
		t.Fatal("SetDefaultValue")
	}
}

func TestTechsetResolveFollowsIncludes(t *testing.T) {
	w := fixture(t)
	ts, err := w.Techsets.Resolve("lit")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Category != "Geometry" || len(ts.Textures) != 1 || ts.Textures[0].Field != "colorMap" || ts.Textures[0].Semantic != "diffuseMap" {
		t.Fatalf("techset: %+v", ts)
	}
	if ex := ts.ExposedFields(); !ex["glossRangeMax"] || ex["specColorMap"] {
		t.Fatalf("exposed fields: %v", ex)
	}
	if !slices.Contains(ts.Sources, "gbuffer_lit.hlsl") {
		t.Fatalf("sources: %v", ts.Sources)
	}
	if w.Techsets.Exists("color_base") {
		t.Fatal("an include is not a material type")
	}
}

func TestFindFlagsStockAndResolvesParents(t *testing.T) {
	w := fixture(t)
	locs, err := w.Find("stock_mtl")
	if err != nil || len(locs) != 1 || !locs[0].Stock {
		t.Fatalf("find stock: %v %v", locs, err)
	}
	f, _ := w.Load("source_data/mine.gdt")
	typ, fields, err := w.Resolved(f, f.Find("child_mtl"))
	if err != nil || typ != "material" {
		t.Fatalf("resolved type %q %v", typ, err)
	}
	got := map[string]string{}
	for _, fl := range fields {
		got[fl.Key] = fl.Value
	}
	if got["materialType"] != "lit" || got["colorMap"] != "my_img" {
		t.Fatalf("inherited + own fields: %v", got)
	}
}

func issueFor(r *EditResult, field, level string) bool {
	for _, is := range r.Issues {
		if is.Field == field && is.Level == level {
			return true
		}
	}
	return false
}

func TestEditValidation(t *testing.T) {
	w := fixture(t)
	r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "new_mtl", Type: "material", DryRun: true, Set: map[string]string{
		"materialType": "lit_advanced", "materialCategory": "Geometry", "normalHeightScale": "3",
		"stencil": "Sometimes", "noCastShadow": "yes", "specColorMap": "my_img", "colorMap": "my_img", "mysteryKey": "1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ field, level string }{
		{"materialCategory", "error"}, {"normalHeightScale", "warning"}, {"stencil", "error"},
		{"noCastShadow", "error"}, {"mysteryKey", "warning"},
	} {
		if !issueFor(r, want.field, want.level) {
			t.Errorf("missing %s %s: %+v", want.level, want.field, r.Issues)
		}
	}
	if r.Written {
		t.Fatal("a dry run with errors must not write")
	}

	r, err = w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "new_mtl", Type: "material", DryRun: true,
		Set: map[string]string{"materialType": "lit", "colorMap": "my_img", "specColorMap": "my_img"}})
	if err != nil {
		t.Fatal(err)
	}
	if !issueFor(r, "specColorMap", "warning") {
		t.Error("lit doesn't read specColorMap: expected a warning")
	}
	if !issueFor(r, "colorMap", "warning") {
		t.Error("my_img is a normalMap in a diffuseMap slot: expected a semantic warning")
	}
}

func TestEditWritesAndGuards(t *testing.T) {
	w := fixture(t)
	if _, err := w.Edit(EditRequest{File: "model_export/stock.gdt", Asset: "stock_mtl", Set: map[string]string{"materialType": "lit"}}); err == nil ||
		!strings.Contains(err.Error(), "stock") {
		t.Fatalf("editing a stock GDT must be refused: %v", err)
	}
	if _, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "stock_mtl", Type: "material"}); err == nil ||
		!strings.Contains(err.Error(), "Duplicate") {
		t.Fatalf("a second definition must be refused: %v", err)
	}
	r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "my_model", CopyFrom: "stock_model",
		Set: map[string]string{"filename": `mine\model.xmodel_bin`}})
	if err != nil || !r.Written || !r.Created {
		t.Fatalf("copy write: %+v %v", r, err)
	}
	f, err := w.Load("source_data/new.gdt")
	if err != nil {
		t.Fatal(err)
	}
	m := f.Find("my_model")
	if fn, _ := m.Get("filename"); fn != `mine\\model.xmodel_bin` {
		t.Fatalf("filename not escaped on disk: %q", fn)
	}
	if lo, _ := m.Get("lowLod"); lo != "" {
		t.Fatalf("donor LOD must be cleared, got %q", lo)
	}
	if locs, _ := w.Find("my_model"); len(locs) != 1 {
		t.Fatalf("index not refreshed after write: %v", locs)
	}
	if !f.CRLF {
		t.Fatal("new GDTs are written with CRLF like APE")
	}
}

func TestEditIsSurgical(t *testing.T) {
	src := "{\r\n\r\n\"odd\" (\"material.gdf\")\r\n{\r\n\"materialType\" \"lit\"\r\n}\r\n\t\"target\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n}\r\n"
	f, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(f.Bytes()) != src {
		t.Fatal("an unmodified file must round-trip byte for byte")
	}
	f.Find("target").Set("colorTint", "1 0 0 1")
	f.Add(&Asset{Name: "fresh", Type: "material"})
	got := string(f.Bytes())
	if !strings.HasPrefix(got, "{\r\n\r\n\"odd\" (\"material.gdf\")\r\n{\r\n\"materialType\" \"lit\"\r\n}\r\n") {
		t.Fatalf("an untouched, oddly formatted asset was rewritten:\n%q", got)
	}
	if !strings.Contains(got, "\t\t\"colorTint\" \"1 0 0 1\"\r\n") || !strings.Contains(got, "\t\"fresh\" ( \"material.gdf\" )\r\n") {
		t.Fatalf("edit or addition missing:\n%q", got)
	}
	if _, err := Parse([]byte(got)); err != nil {
		t.Fatalf("result no longer parses: %v", err)
	}
}

// A slot's default keeps its $: a built-in ($white_diffuse) differs from a
// stock image of the same name.
func TestTechsetDefaultImageKeepsDollar(t *testing.T) {
	w := fixture(t)
	ts, err := w.Techsets.Resolve("lit")
	if err != nil {
		t.Fatal(err)
	}
	if len(ts.Textures) == 0 || ts.Textures[0].DefaultImage != "$white_diffuse" {
		t.Fatalf("textures: %+v", ts.Textures)
	}
}
