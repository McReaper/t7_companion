package cli

import (
	"os"
	"path/filepath"
	"reflect"
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

// bareRoot lays out a map that zones materials on their own: of a geometry
// techset (built only mc/, wc/…), of a 2D one (no prefixes), of a decal one
// that lists "/", and of a tools one stock ships bare anyway.
func bareRoot(t *testing.T) string {
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
	globals := func(category, prefixes string) string {
		return "Globals()\n{\n\tcategory = \"" + category + "\"\n" + prefixes + "}\n"
	}
	write("deffiles/material.awi", `	Asset.AddEntry_Combo( "materialType", "lit" );`)
	write("bin/converter_gdt_dirs_0.txt", "source_data\n")
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", globals("Geometry", "\tavailablePrefixes = \"mc/ mcs/ wc/\"\n"))
	write("share/raw/techsetdefs_stable/2d/hud.techsetdef", globals("2D", ""))
	write("share/raw/techsetdefs_stable/decal/decal.techsetdef", globals("Decal", "\tavailablePrefixes = \"mc/ wc/ / vd/\"\n"))
	write("share/raw/techsetdefs_stable/tools/tools_blend.techsetdef", globals("Tools", "\tavailablePrefixes = \"mc/ mcs/ wc/\"\n"))
	write("zone_source/all/assetlist/core_gfx.csv", "material,$blend\ntechset,tools_blend#da7372ff\n")
	mtl := func(name, typ string) string {
		return "\t\"" + name + "\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"" + typ + "\"\r\n\t}\r\n"
	}
	write("source_data/mine.gdt", "{\r\n"+mtl("mtl_wall", "lit")+mtl("hud_icon", "hud")+mtl("decal_mtl", "decal")+
		mtl("tool_mtl", "tools_blend")+"\t\"mtl_child\" [ \"mtl_wall\" ]\r\n\t{\r\n\t}\r\n}\r\n")
	write("share/zone_source/zm_mod_level.class", "ignore,core_gfx\n")
	write("usermaps/zm_x/zone_source/zm_x.zone", ">class,zm_mod_level\nmaterial,mc/mtl_wall\nmaterial,hud_icon\n"+
		"material,mtl_wall\nmaterial,decal_mtl\nmaterial,tool_mtl\nmaterial,no_such_mtl\ninclude,mine\n")
	write("usermaps/zm_x/zone_source/mine.zpkg", "material,mtl_child\n")
	return root
}

// A material zoned on its own is reported when its techset has no bare
// variant in the shader cache; not with a prefix, of a techset that builds
// bare or that stock ships bare, nor when no GDT defines it.
func TestZoneCheckBareMaterials(t *testing.T) {
	root := bareRoot(t)
	res, err := zoneCheck(root, "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	want := []bareMaterial{
		{Line: "material,mtl_wall", At: "usermaps/zm_x/zone_source/zm_x.zone:4", MaterialType: "lit", Prefixes: []string{"mc/", "mcs/", "wc/"}},
		{Line: "material,mtl_child", At: "usermaps/zm_x/zone_source/mine.zpkg:1", MaterialType: "lit", Prefixes: []string{"mc/", "mcs/", "wc/"}},
	}
	if !reflect.DeepEqual(res.Bare, want) {
		t.Errorf("bare materials:\n got %+v\nwant %+v", res.Bare, want)
	}
	if !strings.Contains(res.BareAdvice, "X1507") || !strings.Contains(res.BareAdvice, "mc/<name>") {
		t.Errorf("advice: %q", res.BareAdvice)
	}
	if res.Advice != "" || len(res.Zoned) != 0 {
		t.Errorf("no stock shadow here: %+v", res)
	}
}

// A zone with no material line, or whose materials all build, gets no advice.
func TestZoneCheckBareMaterialsNone(t *testing.T) {
	root := bareRoot(t)
	zf := filepath.Join(root, "usermaps", "zm_x", "zone_source", "zm_x.zone")
	for _, body := range []string{">class,zm_mod_level\nxmodel,a\n", ">class,zm_mod_level\nmaterial,hud_icon\n"} {
		if err := os.WriteFile(zf, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := zoneCheck(root, "zm_x")
		if err != nil {
			t.Fatal(err)
		}
		if res.Bare != nil || res.BareAdvice != "" {
			t.Errorf("%q: %+v", body, res)
		}
	}
}
