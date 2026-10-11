package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/zone"
)

// Before any link: a stock script copied into the map and zoned, while the
// stock assetlist still lists it, is reported with the line to comment out.
func TestZoneCheckBeforeLink(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("share/zone_source/zm_mod_level.class", "ignore,zm_patch\nignore_missing_shipped,zm_levelcommon\n")
	write("usermaps/zm_x/zone_source/zm_x.zone", ">class,zm_mod_level\nscriptparsetree,scripts/zm/_zm_weapons.gsc\nscriptparsetree,scripts/zm/_zm_perks.gsc\nscriptparsetree,scripts/zm/zm_x.gsc\n")
	write("zone_source/all/assetlist/zm_patch.csv", "scriptparsetree,scripts/zm/_zm_weapons.gsc\n//scriptparsetree,scripts/zm/_zm_perks.gsc\n")

	res, err := zoneCheck(root, "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	if res.zoneStatus != nil || len(res.GDTs) != 0 {
		t.Errorf("no report yet: no link status, no GDT check: %+v", res)
	}
	if len(res.Zoned) != 1 {
		t.Fatalf("want only the script whose stock line is still active, got %+v", res.Zoned)
	}
	z := res.Zoned[0]
	if z.Asset != "scriptparsetree scripts/zm/_zm_weapons.gsc" || z.Yours != "zoned at usermaps/zm_x/zone_source/zm_x.zone:2" ||
		z.Stock != "zone_source/all/assetlist/zm_patch.csv:1" || res.Advice == "" {
		t.Errorf("got %+v", z)
	}
	if strings.Join(res.Inherited.Ignore, ",") != "zm_patch" {
		t.Errorf("inherited: %+v", res.Inherited)
	}
	if _, err := zoneCheck(root, "zm_none"); err == nil {
		t.Error("a map with no zone file must be an error")
	}
}

func TestStockNote(t *testing.T) {
	active := []zone.ListEntry{{List: "core_common", Line: 8, Active: true}}
	if n := stockNote(active); !strings.Contains(n, "only a reference") || !strings.Contains(n, "core_common.csv:8") {
		t.Errorf("active: %q", n)
	}
	if n := stockNote([]zone.ListEntry{{List: "core_common", Line: 8}}); !strings.Contains(n, "commented out") {
		t.Errorf("commented: %q", n)
	}
	if n := stockNote(nil); n != "" {
		t.Errorf("not on a list: %q", n)
	}
}

// sourcesRoot lays out a mod-tools root: a map zoning a weapon whose world
// model no GDT defines, and a material and the weapon that a stock list
// provides while the map's own GDT defines them too.
func sourcesRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir()) // keep the index caches out of the real profile
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
	write("deffiles/bulletweapon.awi", `	Asset.AddEntry_AssetCombo( "worldModel", "xmodel" );`)
	write("deffiles/xmodel.awi", `	Asset.AddEntry_Path( "filename", "" );`)
	write("deffiles/material.awi", `	Asset.AddEntry_Combo( "materialType", "lit" );`)
	write("bin/converter_gdt_dirs_0.txt", "source_data\n")
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", "Globals()\n{\n\tcategory = \"Geometry\"\n}\n")
	write("source_data/mine.gdt", "{\r\n\t\"my_gun\" ( \"bulletweapon.gdf\" )\r\n\t{\r\n\t\t\"worldModel\" \"gun_world\"\r\n\t}\r\n"+
		"\t\"mtl_wall\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n}\r\n")
	write("share/zone_source/zm_mod_level.class", "ignore,zm_patch\n")
	write("zone_source/all/assetlist/zm_patch.csv", "material,mc/mtl_wall\nweapon,my_gun\nattachmentcosmeticvariant,defaultattachmentcosmeticvariant\n")
	write("usermaps/zm_x/zone_source/zm_x.zone", ">class,zm_mod_level\nweapon,my_gun\nmaterial,mtl_wall\n")
	return root
}

// After a link, zone_check finds the GDT versions stock replaces from the
// report, which names a material with its category and a second packing |dup,
// and packs a bulletweapon as a weapon.
func TestZoneCheckAfterLink(t *testing.T) {
	root := sourcesRoot(t)
	dir := filepath.Join(root, "usermaps", "zm_x", "zone_source", "all", "assetinfo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	csv := "index,type,name,resident,streamed,parentStack\n" +
		"1,material,mc/mtl_wall,20,0,|zone_source/zm_x.zone|csv\n" +
		"2,material,mc/mtl_wall|dup,20,0,|zone_source/zm_x.zone|csv\n" +
		"3,weapon,my_gun,30,0,|zone_source/zm_x.zone|csv\n"
	if err := os.WriteFile(filepath.Join(dir, "zm_x.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := zoneCheck(root, "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GDTs) != 1 || res.GDTs[0].GDT != "source_data/mine.gdt" || res.GDTs[0].Assets != 2 {
		t.Errorf("GDT versions stock replaces: %+v", res.GDTs)
	}
}
