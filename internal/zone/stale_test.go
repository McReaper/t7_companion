package zone

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// A report is stale when a file its link read changed since (.deps), or when
// a GDT of yours defining a packed asset was saved after it. A stock GDT
// saved since doesn't count: it isn't the user's edit.
func TestChangedSince(t *testing.T) {
	root, w := gdtRoot(t, map[string]string{
		"source_data/mine.gdt":   "{\n" + gdtAsset("img_a", "image") + "}\n",
		"source_data/old.gdt":    "{\n" + gdtAsset("img_b", "image") + "}\n",
		"source_data/stock.gdt":  "{\n" + gdtAsset("img_c", "image") + "}\n",
		"usermaps/zm_x/zm_x.gsc": "",
	})
	r := linked(t, root, "zm_x", "1,image,img_a,20,0,|zone_source/zm_x.zone|csv\n"+
		"2,image,img_b,20,0,|zone_source/zm_x.zone|csv\n"+
		"3,image,img_c,20,0,|zone_source/zm_x.zone|csv\n")
	gsc := filepath.Join(root, "usermaps", "zm_x", "zm_x.gsc")
	deps := fmt.Sprintf("version,1,2\nscriptparsetree,scripts/zm/zm_x.gsc\n\tfile,%s,%d,\n", gsc, r.Linked.Add(-time.Hour).Unix())
	if err := os.WriteFile(filepath.Join(r.Dir, "zm_x.deps"), []byte(deps), 0o644); err != nil {
		t.Fatal(err)
	}
	before, after := r.Linked.Add(-time.Hour), r.Linked.Add(time.Hour)
	for f, at := range map[string]time.Time{"mine.gdt": after, "old.gdt": before, "stock.gdt": after} {
		if err := os.Chtimes(filepath.Join(root, "source_data", f), at, at); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{gsc, w.Abs("source_data/mine.gdt")}
	if got := ChangedSince(r, w); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if got := ChangedSince(r, nil); !reflect.DeepEqual(got, want[:1]) {
		t.Errorf("without a workspace, .deps alone: %v", got)
	}
}

// The report names a material with its category (mc/x) and a second packing
// x|dup: a GDT of yours defining it, saved after the link, still makes the
// report stale.
func TestChangedSinceMaterial(t *testing.T) {
	root, w := gdtRoot(t, map[string]string{
		"source_data/mats.gdt": "{\n" + gdtAsset("mtl_wall", "material", "materialType", "lit") + "}\n",
	})
	r := linked(t, root, "zm_x", "1,material,mc/mtl_wall,20,0,|zone_source/zm_x.zone|csv\n"+
		"2,material,mc/mtl_wall|dup,20,0,|zone_source/zm_x.zone|csv\n")
	after := r.Linked.Add(time.Hour)
	if err := os.Chtimes(filepath.Join(root, "source_data", "mats.gdt"), after, after); err != nil {
		t.Fatal(err)
	}
	if got := ChangedSince(r, w); !reflect.DeepEqual(got, []string{w.Abs("source_data/mats.gdt")}) {
		t.Errorf("got %v", got)
	}
}
