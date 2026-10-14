package level

import (
	"context"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// A row from one: a world brush and a brush entity copied three times along
// x, the copies' tops textured through @0, a point entity copied once.
func TestCopiesMakeARowFromOne(t *testing.T) {
	_, c, snap := viewing(t, 302)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"copy": {"ids": ["5ec8de49", "6e73936d"], "by": [1000, 0, 0], "count": 3}},
		{"texture": {"ids": ["@0"], "faces": "top", "material": "caulk"}},
		{"copy": {"ids": ["5ec8de46"], "by": [0, 0, 64]}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Label != "t7kb: 1 texture, 2 copy" || len(d.Made) != 7 || d.Faces[1] != 6 || d.Deleted != 0 {
		t.Fatalf("done %+v", d)
	}
	floors, volumes := 0, 0
	for _, m := range d.Made {
		switch {
		case m.Op == 0 && m.Class == "":
			floors++
			if k := (m.Box.Min[0] + 216) / 1000; m.Box != box(mapfile.Vec3{-216 + 1000*k, -232, -64}, mapfile.Vec3{248 + 1000*k, 256, -16}) || k < 1 || k > 3 {
				t.Errorf("floor copy %+v", m.Box)
			}
		case m.Op == 0 && m.Class == "volume_sun":
			volumes++
		case m.Op == 2 && m.Class == "reflection_probe" && m.Box.Min[2] > 0:
		default:
			t.Errorf("made %+v", m)
		}
		if m.ID == "" || m.Was != "" {
			t.Errorf("a copy is new: %+v", m)
		}
	}
	if floors != 3 || volumes != 3 {
		t.Errorf("%d floors, %d volumes", floors, volumes)
	}
	if hits, _ := Find(d.After.Map, d.After.IDs, Query{Class: "volume_sun"}, Selection{}); len(hits) != 4 {
		t.Errorf("%d volumes: the original and three copies", len(hits))
	}
	if _, err := d.After.IDs.Resolve("5ec8de49"); err != nil {
		t.Errorf("the original keeps its id: %v", err)
	}
	for _, e := range d.After.Map.Entities {
		for _, br := range e.Brushes() {
			if b := BrushBox(br); e.Class() == "worldspawn" && b.Min[0] > 700 && b.Max[2] == -16 {
				if f := faceTex(t, br)["+z"]; f.Material != "caulk" {
					t.Errorf("a copy's top is %s", f.Material)
				}
			}
		}
	}
}

func TestCopyRefusals(t *testing.T) {
	_, c, snap := viewing(t, 303)
	for js, want := range map[string]string{
		`[{"copy": {"ids": ["5ec8de49"], "by": [0, 0, 0]}}]`:                                                        "by is [0, 0, 0]",
		`[{"copy": {"ids": ["5ec8de49"], "by": [8, 0, 0], "count": -2}}]`:                                           "count -2",
		`[{"copy": {"ids": [], "by": [8, 0, 0]}}]`:                                                                  "copy: no ids",
		`[{"copy": {"ids": ["6e73936c"], "by": [8, 0, 0]}}]`:                                                        "one brush of a volume_sun: copy the entity",
		`[{"copy": {"ids": ["5ec8de49"], "by": [8, 0, 0], "count": 300}}]`:                                          "300 copies of 1 things is over 256",
		`[{"copy": {"ids": ["nothing"], "by": [8, 0, 0]}}]`:                                                         "nothing",
		`[{"copy": {"ids": ["5ec8de46"], "by": [8, 0, 0], "count": 2}}, {"set": {"id": "@0", "keys": {"a": "b"}}}]`: "set: @0 names 2 things",
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", js, err, want)
		}
	}
	if _, err := Edit(context.Background(), c, snap, ops(t, `[{"copy": {"ids": ["5ec8de46"], "by": [8, 0, 0]}}, {"set": {"id": "@0", "keys": {"targetname": "b"}}}]`), "", nil); err != nil {
		t.Errorf("set on the one copy: %v", err)
	}
}

func TestUnguidedDropsEveryGUIDLine(t *testing.T) {
	in := "// entity 3\n{\nguid \"{A}\"\n\"classname\" \"x\"\n\"guidance\" \"kept\"\n{\n guid \"{B}\"\r\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n}\n}\n"
	want := "// entity 3\n{\n\"classname\" \"x\"\n\"guidance\" \"kept\"\n{\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n}\n}\n"
	if got := unguided(in); got != want {
		t.Errorf("unguided:\n%q\nwant\n%q", got, want)
	}
}
