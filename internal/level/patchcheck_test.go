package level

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// meshText is a 2 by 2 mesh patch over a 100-unit square at x0, its texture
// running tex a unit.
func meshText(material string, x0, tex float64) string {
	v := func(x, y float64) string {
		return fmt.Sprintf("   v %g %g 2 t %g %g 0 0\n", x0+x, y, x*tex, y*tex)
	}
	return "{\n mesh\n {\n  " + material + "\n  lightmap_gray\n  2 2 0 8\n  (\n" + v(0, 0) + v(0, 100) + "  )\n  (\n" + v(100, 0) + v(100, 100) + "  )\n }\n}\n"
}

// map_check reads patches too: a texture running 16 a unit is the stock
// patches' and passes; one past a repeat a unit (cod2map's "repeats more than
// once per inch", what Radiant's terrain triangle writes) is stretched; a
// material no GDT defines ($default, the terrain triangle's) is unknown.
func TestMapCheckReadsPatches(t *testing.T) {
	_, c, snap := viewing(t, 647)
	text := "{\n\"classname\" \"worldspawn\"\n" + meshText("grass", 1000, 16) + meshText("grass", 1200, 20971.52) + meshText("$default", 1400, 16) + "}\n"
	raw, _ := json.Marshal(text)
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"raw": `+string(raw)+`}]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	info := func(m string) MaterialInfo {
		return MaterialInfo{Known: !strings.HasPrefix(m, "$"), Drawn: true, Opaque: true, Tile: Tile{W: 64, H: 64}}
	}
	got := map[string]string{}
	for _, f := range mustCheck(t, d.After, nil, info) {
		if f.Material == "grass" || f.Material == "$default" {
			got[f.Kind+" "+f.Material] = f.Detail
		}
	}
	if len(got) != 2 || !strings.HasPrefix(got["stretched grass"], "its texture repeats 20.5 times a unit") || got["unknown $default"] != "" {
		t.Errorf("findings %v: the dense mesh stretched, the $default one unknown, the natural one none", got)
	}
	var ids []string
	for _, m := range d.Made {
		ids = append(ids, d.After.IDs.Short(m.ID))
	}
	if len(mustCheck(t, d.After, ids[:1], info)) > 1 {
		t.Errorf("named alone, one patch is checked alone")
	}
}

func mustCheck(t *testing.T, snap *Snapshot, ids []string, info func(string) MaterialInfo) []Finding {
	t.Helper()
	out, err := CheckTextures(snap, ids, info)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
