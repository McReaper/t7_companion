package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mod-tools root holding one linked map's report.
func zoneRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "usermaps", "zm_x", "zone_source", "all", "assetinfo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	csv := "index,type,name,resident,streamed,parentStack\n" +
		"1,image,fxt_fog,22,0,|ei/fog_mtl|material|fury.zpkg|csv|zone_source/zm_x.zone|csv\n" +
		"2,material,ei/fog_mtl,500,0,|fury.zpkg|csv|zone_source/zm_x.zone|csv\n" +
		"3,xmodel,skybox_default_day,92,9000,|zone_source/zm_x.zone|csv\n"
	if err := os.WriteFile(filepath.Join(dir, "zm_x.csv"), []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zm_x.deps"), []byte("version,1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestZoneExplain(t *testing.T) {
	root := zoneRoot(t)
	res, err := zoneExplain(root, "zm_x", "fxt_fog", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stale || len(res.Packed) != 1 {
		t.Fatalf("got %+v", res)
	}
	if got := strings.Join(res.Packed[0].PulledInBy, " < "); got != "material ei/fog_mtl < fury.zpkg < zone_source/zm_x.zone" {
		t.Errorf("chain = %s", got)
	}

	res, err = zoneExplain(root, "zm_x", "fog", "")
	if err != nil || res.NotPacked == "" || len(res.Similar) != 2 {
		t.Errorf("an unpacked name says so and suggests near names: %+v, %v", res, err)
	}
	if _, err := zoneExplain(root, "zm_other", "x", ""); err == nil || !strings.Contains(err.Error(), "link it first") {
		t.Errorf("a map never linked: %v", err)
	}
}

func TestZoneContents(t *testing.T) {
	root := zoneRoot(t)
	all, err := zoneContents(root, "zm_x", "")
	if err != nil {
		t.Fatal(err)
	}
	if all.Assets != 3 || len(all.Lines) != 2 || all.Lines[0].Line != "xmodel skybox_default_day" {
		t.Errorf("summary by zone line, heaviest first: %+v", all)
	}
	pkg, err := zoneContents(root, "zm_x", "fury.zpkg")
	if err != nil || pkg.Assets != 2 || pkg.ByType["image"] != 1 || pkg.Largest[0].Asset != "material ei/fog_mtl" {
		t.Errorf("a package's contents: %+v, %v", pkg, err)
	}
	if typed, err := zoneContents(root, "zm_x", "xmodel,skybox_default_day"); err != nil || typed.Assets != 1 {
		t.Errorf("a typed zone line: %+v, %v", typed, err)
	}
	if _, err := zoneContents(root, "zm_x", "nothing_here"); err == nil {
		t.Error("a line that pulled nothing in must be an error the agent can read")
	}
}
