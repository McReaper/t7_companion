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
