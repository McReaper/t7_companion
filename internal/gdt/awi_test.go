package gdt

import (
	"os"
	"path/filepath"
	"testing"
)

// appendAwi adds script to one of the fixture's deffiles.
func appendAwi(t *testing.T, w *Workspace, typ, script string) {
	t.Helper()
	p := filepath.Join(w.Root, "deffiles", typ+".awi")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	put(t, w, "deffiles/"+typ+".awi", append(b, script...))
}

// The shapes material.awi, image.awi, scriptbundle.awi and gfx_bundle.h use.
const awiVarsScript = `
string hero_list = " | hendricks | khalil";
void GenerateUI( asset Asset )
{
	string SurfaceTypeEntries = //wood | bark
	"""
		<error> |
		<none> |
		metal |
		wood
	""";
	Asset.AddEntry_Combo( "surfaceType", SurfaceTypeEntries );

	string FilterModes = "nearest (mip none) | linear (mip linear)";
	string FilterDefaultModes = "linear (mip linear) | " + FilterModes;
	Asset.AddEntry_Combo( "filterColor", FilterDefaultModes );

	string modeStr = "a | b";
	if ( x ) { modeStr = "c"; }
	Asset.AddEntry_Combo( "mode", modeStr );

	string categories = Asset.GetTechsetdefCategories();
	Asset.AddEntry_Combo( "category", categories );
	Asset.AddEntry_Combo( "branchy", "x | y" );
	Asset.AddEntry_Combo( "branchy", categories );

	Asset.AddEntry_Combo( prefix + "Name", hero_list, true );
	Asset.AddEntry_Combo( constString + "name", "scriptVector0 | scriptVector1" );

	string glossSurfaceTypes = " <custom> | <full> | wood";
	Asset.AddEntry_Combo( "glossSurfaceType", glossSurfaceTypes ).SetValidateCallback("void ValidateGlossSurfaceType(asset Asset, const string& ID)");
}

void ValidateGlossSurfaceType(asset Asset, const string& ID)
{
	string surfType = glossSurfaceType.GetValue();
	if ( surfType == "<custom>" )
	{
	}
	else if ( surfType == "<full>" )
	{
		entryGlossMin.SetFloat( 0 );
		entryGlossMax.SetFloat( 17 );
	}
	else if ( surfType == "wood" )
	{
		entryGlossMin.SetFloat( 2 );
		entryGlossMax.SetFloat( 5 );
	}
}
`

func TestComboOptionsFromVariables(t *testing.T) {
	w := fixture(t)
	appendAwi(t, w, "material", awiVarsScript)
	s, err := w.Schema("material")
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]int{"surfaceType": 4, "filterColor": 2, "mode": 3} {
		if e := s.Lookup(field); e == nil || len(e.Options) != want {
			t.Errorf("%s: want %d options from its variable, got %+v", field, want, e)
		}
	}
	if e := s.Lookup("surfaceType"); e.Default != "<error>" {
		t.Errorf("an unmarked list defaults to its first option, like APE: %q", e.Default)
	}
	if e := s.Lookup("category"); len(e.Options) != 0 {
		t.Errorf("options from a function call are unknown: %+v", e.Options)
	}

	r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "m", Type: "material", DryRun: true, Set: map[string]string{
		"surfaceType": "metl", "filterColor": "linear (mip linear)", "mode": "c", "category": "anything",
		"branchy": "z", "object1_Name": "my_targetname", "name": "pstfx_teleport",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !issueFor(r, "surfaceType", "error") {
		t.Errorf("a surfaceType typo is an error: %+v", r.Issues)
	}
	for _, f := range []string{"filterColor", "mode", "category", "branchy", "object1_Name", "name"} {
		if issueFor(r, f, "error") {
			t.Errorf("%s: no error expected (concatenated / branch / dynamic / editable / not a pattern match): %+v", f, r.Issues)
		}
	}
}

func TestComboValueStockUsesIsNotAnError(t *testing.T) {
	w := fixture(t)
	appendAwi(t, w, "material", awiVarsScript)
	// a "legacy support" value the current .awi dropped, still in a stock GDT
	put(t, w, "model_export/stock.gdt", []byte("{\r\n\t\"stock_mtl\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"filterColor\" \"mip standard (2x bilinear)*\"\r\n\t}\r\n}\r\n"))
	r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "m", Type: "material", DryRun: true,
		Set: map[string]string{"filterColor": "mip standard (2x bilinear)", "mode": "zzz"}})
	if err != nil {
		t.Fatal(err)
	}
	if issueFor(r, "filterColor", "error") {
		t.Errorf("a value stock GDTs use links: %+v", r.Issues)
	}
	if !issueFor(r, "mode", "error") {
		t.Errorf("a value neither the .awi nor stock has is still an error: %+v", r.Issues)
	}
}

func TestSurfaceTypeErrorWarns(t *testing.T) {
	w := fixture(t)
	appendAwi(t, w, "material", awiVarsScript)
	for _, set := range []map[string]string{
		{"materialType": "lit"},                           // never set: APE's default is <error>
		{"materialType": "lit", "surfaceType": "<error>"}, // set to it
	} {
		r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "m", Type: "material", DryRun: true, Set: set})
		if err != nil {
			t.Fatal(err)
		}
		if !issueFor(r, "surfaceType", "warning") {
			t.Errorf("%v: the linker stops on a colliding material without a surface type: %+v", set, r.Issues)
		}
	}
	r, _ := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "m", Type: "material", DryRun: true,
		Set: map[string]string{"materialType": "lit", "surfaceType": "<none>"}})
	if issueFor(r, "surfaceType", "warning") {
		t.Errorf("<none> is a surface type: %+v", r.Issues)
	}
}

func TestGlossPresetWritesItsRange(t *testing.T) {
	w := fixture(t)
	appendAwi(t, w, "material", awiVarsScript)
	r, err := w.Edit(EditRequest{File: "source_data/mine.gdt", Asset: "mine_base", Set: map[string]string{"glossSurfaceType": "wood"}})
	if err != nil {
		t.Fatal(err)
	}
	_, a, _ := w.loadAsset("mine_base", "material")
	if v, _ := a.Get("glossRangeMin"); Unquote(v) != "2" {
		t.Errorf("wood writes glossRangeMin 2 like APE: %q (changes %v)", v, r.Changes)
	}
	if v, _ := a.Get("glossRangeMax"); Unquote(v) != "5" {
		t.Errorf("wood writes glossRangeMax 5 like APE: %q", v)
	}

	w.Edit(EditRequest{File: "source_data/mine.gdt", Asset: "mine_base", Set: map[string]string{"glossSurfaceType": "<full>", "glossRangeMax": "9"}})
	_, a, _ = w.loadAsset("mine_base", "material")
	if v, _ := a.Get("glossRangeMax"); Unquote(v) != "9" {
		t.Errorf("a range the request sets wins over the preset: %q", v)
	}
	w.Edit(EditRequest{File: "source_data/mine.gdt", Asset: "mine_base", Set: map[string]string{"glossSurfaceType": "<custom>"}})
	_, a, _ = w.loadAsset("mine_base", "material")
	if v, _ := a.Get("glossRangeMax"); Unquote(v) != "9" {
		t.Errorf("<custom> leaves the range alone: %q", v)
	}
}

func TestImageSemanticSetsPremulAlpha(t *testing.T) {
	w := fixture(t)
	r, err := w.Edit(EditRequest{File: "source_data/mine.gdt", Asset: "my_img", DryRun: true, Set: map[string]string{"semantic": "effectMap"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{`+ premulAlpha = "1"`: true, `+ streamable = "0"`: true}
	for _, c := range r.Changes {
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("effectMap sets premulAlpha and clears streamable, as image.awi does: %v", r.Changes)
	}
}

func TestLODsMustGoFartherOut(t *testing.T) {
	w := fixture(t)
	appendAwi(t, w, "xmodel", `
	Asset.AddEntry_Float( "highLodDist", 0, 0, 1000000 );
	Asset.AddEntry_Float( "mediumLodDist", 0, 0, 1000000 );
	Asset.AddEntry_Float( "lowLodDist", 0, 0, 1000000 );
`)
	r, err := w.Edit(EditRequest{File: "source_data/new.gdt", Asset: "x", Type: "xmodel", DryRun: true, Set: map[string]string{
		"filename": "a.xmodel_bin", "mediumLod": "b.xmodel_bin", "highLodDist": "1000", "mediumLodDist": "800", "lowLodDist": "500",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !issueFor(r, "mediumLodDist", "warning") {
		t.Errorf("a used LOD switching nearer than the one before is never drawn: %+v", r.Issues)
	}
	if issueFor(r, "lowLodDist", "warning") {
		t.Errorf("an unused LOD's distance doesn't matter: %+v", r.Issues)
	}
}
