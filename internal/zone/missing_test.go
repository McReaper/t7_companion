package zone

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
)

// depsFixture is a .deps as the linker writes it: the inherited lists, then
// each asset line followed by its indented lookups.
const depsFixture = "version,3421953,593\n" +
	"ignore_missing_shipped,zm_levelcommon\n" + // a list the install lacks: skipped
	"ignore,core_common\n" +
	"material,mc/mtl_a\n" +
	"\tgdt,material,mtl_a,AA-BB\n" +
	"\tgdt,image,img_found,11-22\n" +
	"\tgdt,image,img_missing,\n" +
	"\tgdt,image,pc:img_missing,\n" +
	"\tgdt,image,[french]img_missing,\n" +
	"\tgdt,image,pc:[french]img_missing,\n" +
	"\tgdt,image,$white,\n" + // an engine built-in
	"\tgdt,image,img_variant,\n" +
	"\tgdt,image,pc:img_variant,33-44\n" + // found under its platform variant
	"\tfile,C:\\bo3\\deffiles\\material.awi,1731004459,\n" +
	"\tgdt,image\n" + // too short: ignored
	"material,mc/mtl_b\n" +
	"\tgdt,emblem,icon_shipped,\n" +
	"\tgdt,image,icon_shipped,\n" + // core_common ships it
	"\tgdt,emblem,poly,\n" +
	"\tgdt,image,poly,55-66\n" + // found as another type
	"\tgdt,image,IMG_Case,\n" +
	"\tgdt,image,img_case,77-88\n" + // names are case-insensitive
	"\tgdt,image,img_missing,\n" + // reported once, under its first parent
	"\tgdt,image,other_missing,\n" +
	"no comma line\n"

func writeDeps(t *testing.T, body string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "usermaps", "zm_x", "zone_source", "all", "assetinfo")
	lists := filepath.Join(root, "zone_source", "all", "assetlist")
	for _, d := range []string{dir, lists} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(lists, "core_common.csv"), []byte("image,icon_shipped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zm_x.deps"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

// A name is missing when none of its lookups, under any variant or type,
// found it; built-ins and what an inherited list ships are not.
func TestMissingLookups(t *testing.T) {
	root, dir := writeDeps(t, depsFixture)
	got, err := MissingLookups(root, dir, "zm_x")
	if err != nil {
		t.Fatal(err)
	}
	mtlA, mtlB := asset.ID{Type: "material", Name: "mc/mtl_a"}, asset.ID{Type: "material", Name: "mc/mtl_b"}
	want := []Missing{
		{ID: asset.ID{Type: "image", Name: "img_missing"}, By: mtlA},
		{ID: asset.ID{Type: "image", Name: "other_missing"}, By: mtlB},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if s := got[0].String(); s != "image img_missing (named by material mc/mtl_a)" {
		t.Errorf("String: %q", s)
	}
}

func TestMissingLookupsUnreadable(t *testing.T) {
	root, dir := writeDeps(t, "material,x\n\tgdt,image,"+strings.Repeat("y", 2<<20)+",\n")
	if _, err := MissingLookups(root, dir, "zm_x"); err == nil {
		t.Error("a line past the scanner's buffer must be an error")
	}
	if _, err := MissingLookups(root, dir, "zm_none"); err == nil {
		t.Error("no .deps must be an error")
	}
}
