package zone

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
)

// Before any link: a stock script copied into the map and zoned, while an
// `ignore` list still ships it, is a shadow; one whose stock line is commented
// out, or that only an ignore_missing_shipped list names, is not.
func TestZonedShadows(t *testing.T) {
	root := layout(t, map[string]string{
		"share/zone_source/zm_mod_level.class":         "ignore,zm_patch\nignore_missing_shipped,zm_levelcommon\n",
		"usermaps/zm_x/zone_source/zm_x.zone":          ">class,zm_mod_level\nscriptparsetree,scripts/zm/_zm_weapons.gsc\nscriptparsetree,scripts/zm/_zm_perks.gsc\nscriptparsetree,scripts/zm/_zm_common.gsc\n",
		"zone_source/all/assetlist/zm_patch.csv":       "scriptparsetree,scripts/zm/_zm_weapons.gsc\n//scriptparsetree,scripts/zm/_zm_perks.gsc\n",
		"zone_source/all/assetlist/zm_levelcommon.csv": "scriptparsetree,scripts/zm/_zm_common.gsc\n",
	})
	zf := filepath.Join(root, "usermaps", "zm_x", "zone_source", "zm_x.zone")
	lists, inh, err := IgnoreLists(root, zf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inh.Ignore, []string{"zm_patch"}) {
		t.Errorf("inherited %+v", inh)
	}
	got := ZonedShadows(root, zf, lists)
	want := []Shadow{{ID: asset.ID{Type: "scriptparsetree", Name: "scripts/zm/_zm_weapons.gsc"},
		File: "usermaps/zm_x/zone_source/zm_x.zone", Line: 2, Stock: ListEntry{List: "zm_patch", Line: 1, Active: true}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got := ZonedShadows(root, filepath.Join(root, "missing.zone"), lists); got != nil {
		t.Errorf("an unreadable zone file: %+v", got)
	}
	if _, _, err := IgnoreLists(root, filepath.Join(root, "missing.zone")); err == nil {
		t.Error("no zone file: want an error")
	}
}

func TestIgnoreListsMissingList(t *testing.T) {
	root := layout(t, map[string]string{
		"share/zone_source/zm_mod_level.class": "ignore,zm_absent\n",
		"usermaps/zm_x/zone_source/zm_x.zone":  ">class,zm_mod_level\n",
	})
	if _, _, err := IgnoreLists(root, filepath.Join(root, "usermaps", "zm_x", "zone_source", "zm_x.zone")); err == nil {
		t.Error("an ignore list the install lacks: want an error")
	}
}

// After a link: an asset packed only as a reference to the stock one, that a
// GDT of yours defines too, is a shadow — found under the name the report
// gives (a material with its category, a second packing |dup) and the type
// the linker packs it as (a bulletweapon is a weapon). A stock GDT's
// definition is not.
func TestGDTShadows(t *testing.T) {
	root, w := gdtRoot(t, map[string]string{
		"source_data/mine.gdt": "{\n" + gdtAsset("my_gun", "bulletweapon", "worldModel", "gun_world") +
			gdtAsset("mtl_wall", "material", "materialType", "lit") + gdtAsset("mtl_free", "material", "materialType", "lit") + "}\n",
		"source_data/stock.gdt":                  "{\n" + gdtAsset("mtl_wall", "material", "materialType", "lit") + "}\n",
		"zone_source/all/assetlist/zm_patch.csv": "material,mc/mtl_wall\nweapon,my_gun\n//material,mc/mtl_free\n",
	})
	lists, err := LoadAssetlists(root, []string{"zm_patch"})
	if err != nil {
		t.Fatal(err)
	}
	r := linked(t, root, "zm_x", "1,material,mc/mtl_wall,20,0,|zone_source/zm_x.zone|csv\n"+
		"2,material,mc/mtl_wall|dup,20,0,|zone_source/zm_x.zone|csv\n"+
		"3,weapon,my_gun,30,0,|zone_source/zm_x.zone|csv\n"+
		"4,material,mc/mtl_free,20,0,|zone_source/zm_x.zone|csv\n")
	got := GDTShadows(r, w, lists)
	if len(got) != 2 {
		t.Fatalf("want the material once and the weapon: %+v", got)
	}
	for _, s := range got {
		if s.File != "source_data/mine.gdt" || !s.InGDT || s.Line == 0 || !s.Stock.Active || s.Stock.List != "zm_patch" {
			t.Errorf("%+v", s)
		}
	}
	if got[0].ID != (asset.ID{Type: "material", Name: "mtl_wall"}) || got[1].ID != (asset.ID{Type: "weapon", Name: "my_gun"}) {
		t.Errorf("ids: %v, %v", got[0].ID, got[1].ID)
	}
}

func TestActive(t *testing.T) {
	a := Assetlists{
		asset.ID{Type: "image", Name: "x"}: {{List: "a", Line: 1}, {List: "b", Line: 7, Active: true}},
		asset.ID{Type: "image", Name: "y"}: {{List: "a", Line: 2}},
	}
	if e, ok := a.Active(asset.ID{Type: "image", Name: "X"}); !ok || e.List != "b" || e.Line != 7 {
		t.Errorf("x: %+v %v", e, ok)
	}
	if e, ok := a.Active(asset.ID{Type: "image", Name: "y"}); ok {
		t.Errorf("y is only commented out: %+v", e)
	}
	if _, ok := a.Active(asset.ID{Type: "image", Name: "z"}); ok {
		t.Error("z is on no list")
	}
}

func TestRelTo(t *testing.T) {
	root := t.TempDir()
	if got := relTo(root, filepath.Join(root, "a", "b.zone")); got != "a/b.zone" {
		t.Errorf("under the root: %q", got)
	}
	outside := filepath.Join(filepath.Dir(root), "elsewhere.zone")
	if got := relTo(root, outside); got != outside {
		t.Errorf("outside the root: %q", got)
	}
}
