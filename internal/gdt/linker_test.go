package gdt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
)

// A GDT may define one name twice, as a material and an xmodel: a derived asset
// of that name takes the first definition as its parent, as gdtdb does.
func TestTypeOfTakesTheFirstParentDefinition(t *testing.T) {
	w := fixture(t)
	gdt := "{\r\n\t\"twin\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n" +
		"\t\"twin\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t}\r\n" +
		"\t\"kid\" [ \"twin\" ]\r\n\t{\r\n\t\t\"colorMap\" \"my_img\"\r\n\t}\r\n}\r\n"
	if err := os.WriteFile(filepath.Join(w.Root, "source_data", "twins.gdt"), []byte(gdt), 0o644); err != nil {
		t.Fatal(err)
	}
	w.Touched(filepath.Join(w.Root, "source_data", "twins.gdt"))
	locs, err := w.FindAsset(asset.ID{Type: "material", Name: "kid"})
	if err != nil || len(locs) != 1 || locs[0].Type != "material" {
		t.Fatalf("kid as a material: %+v, %v", locs, err)
	}
	if locs, _ := w.FindAsset(asset.ID{Type: "xmodel", Name: "kid"}); len(locs) != 0 {
		t.Errorf("kid is not an xmodel: %+v", locs)
	}
}

// The linker packs every weapon class as a weapon, and a script bundle class
// (a "type" combo with one option) as a scriptbundle.
func TestLinkerType(t *testing.T) {
	w := fixture(t)
	awi := `	Asset.AddEntry_Combo( "type", "gibcharacterdef" ).Show( false );`
	if err := os.WriteFile(filepath.Join(w.Root, "deffiles", "gibcharacterdef.awi"), []byte(awi), 0o644); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"bulletweapon": "weapon", "projectileweapon | bulletweapon": "weapon", "weaponcamotable": "weaponcamo",
		"gibcharacterdef": "scriptbundle", "material": "material", "xmodel": "xmodel", "image": "image",
	} {
		if got := w.LinkerType(in); got != want {
			t.Errorf("LinkerType(%q) = %q, want %q", in, got, want)
		}
	}
}

// An item list (GenerateItemList in asset_list_helper.h) declares <prefix>NN
// references and <prefix>Count; only the items the count covers are read.
func TestItemListRefs(t *testing.T) {
	w := fixture(t)
	awi := `	GenerateItemList( Asset, "playerbodystyle", "Body Style", "bodyStyle", MAX_BODY_COUNT, "Body Styles" );`
	if err := os.WriteFile(filepath.Join(w.Root, "deffiles", "playerbodytype.awi"), []byte(awi), 0o644); err != nil {
		t.Fatal(err)
	}
	sc, err := w.Schema("playerbodytype")
	if err != nil {
		t.Fatal(err)
	}
	if e := sc.Lookup("bodyStyle02"); e == nil || e.Kind != "AssetCombo" || e.AssetType != "playerbodystyle" {
		t.Fatalf("bodyStyle02: %+v", e)
	}
	if e := sc.Lookup("bodyStyleCount"); e == nil || e.Kind != "Int" {
		t.Fatalf("bodyStyleCount: %+v", e)
	}
	refs := w.Refs("playerbodytype", []Field{
		{Key: "bodyStyle01", Value: "a"}, {Key: "bodyStyle02", Value: "b"},
		{Key: "bodyStyle03", Value: "stale"}, {Key: "bodyStyleCount", Value: "2"},
	})
	var got []string
	for _, r := range refs {
		got = append(got, r.Target)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("refs in use: %v", got)
	}
}
