package zone

import (
	"github.com/McReaper/t7_companion/internal/asset"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeRoot lays out a mod-tools root: two classes, one map with a zone and a
// package, a shared package, and two stock assetlists.
func fakeRoot(t *testing.T) (root, zoneFile string) {
	t.Helper()
	root = t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("share/zone_source/zm_mod_level.class", "#include \"zm_level.class\"\nignore_missing_shipped,zm_levelcommon\n")
	write("share/zone_source/zm_level.class", ">mode,zm\nignore,zm_patch\nignore,core_common\n#include \"zm_mod_level.class\"\n")
	write("usermaps/zm_x/zone_source/zm_x.zone", ">class,zm_mod_level\n>group,modtools\n\n// scripts\nscriptparsetree,scripts/zm/_zm_weapons.gsc\ninclude,mine\ninclude,shared\nxmodel,my_model\n")
	write("usermaps/zm_x/zone_source/mine.zpkg", "fx,zombie/fx_blood\n")
	write("share/zone_source/mine.zpkg", "xmodel,never_read\n") // the map's own package wins
	write("share/zone_source/shared.zpkg", "include,mine\nimage,shared_img\n")
	write("zone_source/all/assetlist/zm_patch.csv", "scriptparsetree,scripts/zm/_zm_weapons.gsc\n//scriptparsetree,scripts/zm/_zm_perks.gsc\n")
	write("zone_source/all/assetlist/core_common.csv", "\nfx,zombie/fx_blood\n")
	return root, filepath.Join(root, "usermaps", "zm_x", "zone_source", "zm_x.zone")
}

func TestInherit(t *testing.T) {
	root, zf := fakeRoot(t)
	got, err := Inherit(root, zf)
	if err != nil {
		t.Fatal(err)
	}
	want := Inherited{Ignore: []string{"zm_patch", "core_common"}, IgnoreMissingShipped: []string{"zm_levelcommon"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v (includes followed once, even when circular)", got, want)
	}
	if found, err := ZoneFile(root, "zm_x"); err != nil || found != zf {
		t.Errorf("ZoneFile = %q, %v", found, err)
	}
}

func TestZoneLines(t *testing.T) {
	root, zf := fakeRoot(t)
	lines, err := ZoneLines(root, zf)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range lines {
		got = append(got, l.String()+" @"+filepath.Base(l.File))
	}
	want := []string{
		"scriptparsetree scripts/zm/_zm_weapons.gsc @zm_x.zone",
		"fx zombie/fx_blood @mine.zpkg", // the map's package, not share's; read once though included twice
		"image shared_img @shared.zpkg",
		"xmodel my_model @zm_x.zone",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if lines[0].N != 5 {
		t.Errorf("line numbers count every line, comments and blanks included: got %d, want 5", lines[0].N)
	}
}

func TestAssetlists(t *testing.T) {
	root, _ := fakeRoot(t)
	lists, err := LoadAssetlists(root, []string{"zm_patch", "core_common"})
	if err != nil {
		t.Fatal(err)
	}
	active := lists.Lookup(asset.ID{Type: "ScriptParseTree", Name: "SCRIPTS/zm/_zm_weapons.gsc"})
	if len(active) != 1 || !active[0].Active || active[0].Line != 1 || active[0].File() != "zone_source/all/assetlist/zm_patch.csv" {
		t.Errorf("active entry: %+v", active)
	}
	if c := lists.Lookup(asset.ID{Type: "scriptparsetree", Name: "scripts/zm/_zm_perks.gsc"}); len(c) != 1 || c[0].Active || c[0].Line != 2 {
		t.Errorf("a // line is an override, kept with its line number: %+v", c)
	}
	if e := lists.Lookup(asset.ID{Type: "fx", Name: "zombie/fx_blood"}); len(e) != 1 || e[0].Line != 2 {
		t.Errorf("blank lines still count: %+v", e)
	}
	if _, err := LoadAssetlists(root, []string{"nope"}); err == nil {
		t.Error("a missing list must be an error")
	}
}
