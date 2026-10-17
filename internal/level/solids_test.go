package level

import (
	"fmt"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// prefabMap is a prefab's map: world brushes, then entities.
func prefabMap(t *testing.T, world []string, ents ...string) *mapfile.Map {
	t.Helper()
	var text strings.Builder
	text.WriteString("iwmap 4\n// entity 0\n{\n\"classname\" \"worldspawn\"\n")
	for k, b := range world {
		fmt.Fprintf(&text, "// brush %d\n%s", k, b)
	}
	text.WriteString("}\n")
	for k, e := range ents {
		fmt.Fprintf(&text, "// entity %d\n%s", k+1, e)
	}
	m, err := mapfile.Parse([]byte(text.String()))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func box6(b [6]float64) Box {
	return Box{Min: mapfile.Vec3{b[0], b[1], b[2]}, Max: mapfile.Vec3{b[3], b[4], b[5]}, Ok: true}
}

// A room inside a prefab inside a prefab, the inner one at a quarter size
// scaled 2 and the outer one scaled 2, turned 90 (pitch and roll 180, yaw -90):
// the probe fills the room where the prefabs put it, through a pillar of
// clip_ai and one nonColliding, on a floor of two brushes in a func_group; a
// prefab whose file isn't there is left out, said. Without the files, the
// walk finds no room, and says why.
func TestProbesStandOnPrefabs(t *testing.T) {
	var world []string
	for _, b := range [][6]float64{ // 64 by 32 inside, 28 high, walls 4 thick
		{-4, -4, 0, 0, 36, 28}, {64, -4, 0, 68, 36, 28}, {0, -4, 0, 64, 0, 28}, {0, 32, 0, 64, 36, 28}, {-4, -4, 28, 68, 36, 32},
	} {
		world = append(world, BoxBrush(box6(b), "brick", ""))
	}
	world = append(world, BoxBrush(box6([6]float64{30, 14, 0, 34, 18, 28}), "clip_ai", ""), BoxBrush(box6([6]float64{15, 14, 0, 19, 18, 28}), "brick", "nonColliding"))
	quarter := prefabMap(t, world, EntityText("func_group", nil,
		BoxBrush(box6([6]float64{-4, -4, -4, 32, 36, 0}), "concrete", ""), BoxBrush(box6([6]float64{32, -4, -4, 68, 36, 0}), "concrete", "")))
	outer := prefabMap(t, nil,
		EntityText("misc_prefab", map[string]string{"model": "_prefabs/test/quarter.map", "origin": "0 0 0", "modelscale": "2"}),
		EntityText("misc_prefab", map[string]string{"model": "_prefabs/test/missing.map", "origin": "0 0 -1000"}))
	read := func(model string) (*mapfile.Map, error) {
		switch model {
		case "_prefabs/test/quarter.map":
			return quarter, nil
		case "_prefabs/test/outer.map":
			return outer, nil
		}
		return nil, fmt.Errorf("no %s", model)
	}
	op := `[{"entity": {"class": "misc_prefab", "origin": [7000, 0, 0], "angles": [180, -90, 180], "keys": {"model": "_prefabs/test/outer.map", "modelscale": "2"}}},
		{"probes": {"at": [6936, 128, 8]}}]`
	d, ps, err := probesRun(t, 953, op, WithPrefabs(read))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{6872, 0, 0}, mapfile.Vec3{7000, 256, 112}) // turned 90: its x along the world's y, its y back along -x
	if !strings.Contains(strings.Join(d.Notes, "\n"), "missing.map left out: no _prefabs/test/missing.map") {
		t.Errorf("notes %q: the missing prefab", d.Notes)
	}
	if _, _, err := probesRun(t, 954, op); err == nil || !strings.Contains(err.Error(), "outer.map left out") {
		t.Errorf("unread prefab: %v", err)
	}
}

// A patch is a floor: a room whose floor is a flat curve patch is probed.
func TestProbesStandOnPatches(t *testing.T) {
	var world []string
	for _, b := range [][6]float64{{-8, -8, 0, 0, 264, 112}, {256, -8, 0, 264, 264, 112}, {0, -8, 0, 256, 0, 112}, {0, 256, 0, 256, 264, 112}, {-8, -8, 112, 264, 264, 128}} {
		world = append(world, BoxBrush(box6(b), "brick", ""))
	}
	var rows strings.Builder
	for _, y := range []int{0, 128, 256} {
		rows.WriteString("   (\n")
		for _, x := range []int{0, 128, 256} {
			fmt.Fprintf(&rows, "\tv %d %d 0 t 0 0 0 0\n", x, y)
		}
		rows.WriteString("   )\n")
	}
	patch := " {\n  curve\n  {\n   concrete\n   lightmap_gray\n   3 3 16 1\n" + rows.String() + "  }\n }\n"
	pm := prefabMap(t, append(world, patch))
	read := func(string) (*mapfile.Map, error) { return pm, nil }
	_, ps, err := probesRun(t, 982, `[{"entity": {"class": "misc_prefab", "origin": [8000, 0, 0], "keys": {"model": "_prefabs/test/patch.map"}}},
		{"probes": {"at": [8128, 128, 8]}}]`, WithPrefabs(read))
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{8000, 0, 0}, mapfile.Vec3{8256, 256, 112})
}

// blocks is what a player collides with: a brush with one face of a
// material players don't walk through, unless its contents let them.
func TestBlocks(t *testing.T) {
	b := box6([6]float64{0, 0, 0, 64, 64, 64})
	faces := func(text string, n int) string { // the brush with its first n faces
		var out []string
		k := 0
		for _, l := range strings.SplitAfter(text, "\n") {
			if strings.Contains(l, "( ") {
				if k++; k > n {
					continue
				}
			}
			out = append(out, l)
		}
		return strings.Join(out, "")
	}
	clip := BoxBrush(b, "clip_ai", "")
	for want, cases := range map[bool][]string{
		true: {BoxBrush(b, "brick", ""), strings.Replace(clip, "clip_ai", "brick", 1), faces(BoxBrush(b, "brick", ""), 4)},
		false: {clip, strings.ReplaceAll(clip, "clip_ai", "CLIP_AI"), BoxBrush(b, "brick", "nonColliding"), BoxBrush(b, "brick", "weaponClip detail"),
			faces(BoxBrush(b, "brick", ""), 3), BoxBrush(b, "trigger", "")},
	} {
		for _, text := range cases {
			if got := blocks(prefabMap(t, []string{text}).Entities[0].Brushes()[0]); got != want {
				t.Errorf("blocks = %v, want %v:\n%s", got, want, text)
			}
		}
	}
}

// A room at the low corner of everything solid still has its ring of empty
// cells around it.
func TestProbesBoxARoomAtTheMapsEdge(t *testing.T) {
	_, ps, err := probesRun(t, 977, `[`+room(-9000, -9000, -8744, -8744, 0, "")+`, {"probes": {"at": [-8872, -8872, 8]}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 {
		t.Fatalf("%d probes: %+v", len(ps), ps)
	}
	wantBox(t, ps, mapfile.Vec3{-8996, -8996, 0}, mapfile.Vec3{-8748, -8748, 112})
}

// An open slab at a corner of everything solid: it is boxed from its edge on
// the grid's lines to its last whole cell (no wall to snap to), open sky
// over it.
func TestProbesBoxASlabAtTheMapsCorners(t *testing.T) {
	for _, c := range []struct{ lo, hi, box float64 }{{-9000, -8740, -8744}, {9000, 9252, 9248}} {
		_, ps, err := probesRun(t, 978, fmt.Sprintf(`[{"box": {"min": [%[1]g, %[1]g, -16], "max": [%[2]g, %[2]g, 0], "material": "concrete"}},
			{"probes": {"at": [%[3]g, %[3]g, 8]}}]`, c.lo, c.hi, (c.lo+c.hi)/2))
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != 1 {
			t.Fatalf("slab %g to %g: %+v", c.lo, c.hi, ps)
		}
		wantBox(t, ps, mapfile.Vec3{c.lo, c.lo, 0}, mapfile.Vec3{c.box, c.box, 384})
	}
}
