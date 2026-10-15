package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// checkedRadiant serves a map on the materials install: a wood floor, a brick
// wall on it, a brick wall beside that one shifted a quarter tile, a box of a
// material no GDT defines and a caulk box.
func checkedRadiant(t *testing.T) *radianttest.Fake {
	t.Helper()
	root := materialsInstall(t)
	if err := os.WriteFile(filepath.Join(root, "source_data", "tools.gdt"), []byte("{\r\n"+
		"\t\"caulk\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"tools_replace\"\r\n\t\t\"usage\" \"tools\"\r\n\t}\r\n}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	brush := func(i int, lo, hi mapfile.Vec3, mat, tex string) string {
		b := level.BoxBrush(level.Box{Min: lo, Max: hi, Ok: true}, mat, "")
		b = strings.ReplaceAll(b, " 64 64 0 0 0 0 lightmap_gray", " "+tex+" lightmap_gray")
		return strings.Replace(b, "{\n", fmt.Sprintf("{\n guid \"{0E0000%02X-0000-0000-0000-000000000000}\"\n", i), 1)
	}
	text := level.EntityText("worldspawn", nil,
		brush(0, mapfile.Vec3{0, 0, -16}, mapfile.Vec3{512, 256, 0}, "t7_wood_planks", "64 32 0 0 0 0"),
		brush(1, mapfile.Vec3{0, 0, 0}, mapfile.Vec3{256, 8, 112}, "t7_brick_worn_red", "64 32 0 0 0 0"),
		brush(2, mapfile.Vec3{256, 0, 0}, mapfile.Vec3{512, 8, 112}, "t7_brick_worn_red", "64 32 16 0 0 0"),
		brush(3, mapfile.Vec3{0, 128, 0}, mapfile.Vec3{64, 192, 64}, "no_such_material", "64 32 0 0 0 0"),
		brush(4, mapfile.Vec3{256, 128, 0}, mapfile.Vec3{320, 192, 64}, "caulk", "64 64 0 0 0 0"))
	f := radianttest.New(43, filepath.Join(root, "map_source", "zm", "checked.map"))
	f.Text = text
	onMachine(t, f)
	return f
}

func TestMapCheckFindsTheWorstFirst(t *testing.T) {
	checkedRadiant(t)
	out, err := mapCheck(context.Background(), nil, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// unknown: the box's six faces; seam: the two walls' -y, +y and +z planes
	// (their bottoms are hidden: no seam shows there); hidden: both walls'
	// bottoms on the floor, and the ends they butt together at x = 256.
	if out.Map != "checked.map" || out.Counts["unknown"] != 6 || out.Counts["seam"] != 3 || out.Counts["hidden"] != 4 || out.Counts["stretched"] != 0 || out.More != 0 {
		t.Fatalf("counts %v, more %d", out.Counts, out.More)
	}
	if f := out.Findings[0]; f.Kind != "unknown" || f.ID != "0e000003" || f.Material != "no_such_material" {
		t.Errorf("first %+v: the unknown material", f)
	}
	if f := out.Findings[6]; f.Kind != "seam" || f.ID != "0e000001" || !strings.Contains(f.Detail, "0e000002") {
		t.Errorf("seventh %+v: the walls' seam", f)
	}
	hidden := map[string]bool{}
	for _, f := range out.Findings[9:] {
		hidden[f.ID+" "+f.Face] = f.Kind == "hidden"
	}
	if !hidden["0e000001 -z"] || !hidden["0e000002 -z"] || !hidden["0e000001 +x"] || !hidden["0e000002 -x"] || len(hidden) != 4 {
		t.Errorf("hidden %v: the walls' bottoms and butted ends (the caulk box's draw nothing; the unknown one's is reported as unknown)", hidden)
	}
	few, _ := mapCheck(context.Background(), nil, "hidden, seam", 2, 0)
	if len(few.Findings) != 2 || few.More != 5 || few.Counts["unknown"] != 0 || few.Findings[0].Kind != "seam" {
		t.Errorf("two of the seams and hidden faces: %+v", few)
	}
	if most, err := mapCheck(context.Background(), nil, "", 200, 0); err != nil || len(most.Findings) != 13 {
		t.Errorf("the most a list takes: %v", err)
	}
	one, _ := mapCheck(context.Background(), []string{"0e000002"}, "", 0, 0)
	if one.Counts["seam"] != 3 || one.Counts["hidden"] != 2 || one.Counts["unknown"] != 0 {
		t.Errorf("the second wall's findings: %v", one.Counts)
	}
	for _, c := range []struct {
		kinds string
		limit int
		want  string
	}{{"seams", 0, `kind "seams"`}, {"", -1, "limit -1"}, {"", 201, "limit 201"}} {
		if _, err := mapCheck(context.Background(), nil, c.kinds, c.limit, 0); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestMapCheckToolAndCommand(t *testing.T) {
	checkedRadiant(t)
	res, err := mapCheckTool().Handler(context.Background(), mcpRequest("map_check", map[string]any{"kinds": "unknown", "limit": 1}))
	if err != nil || res.IsError || !strings.Contains(resultText(res), `"counts":{"unknown":6}`) || !strings.Contains(resultText(res), `"more":5`) {
		t.Errorf("tool: %v %s", err, resultText(res))
	}
	cmd := newMapCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"check", "0e000001", "--kinds", "hidden"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var answer mapCheckOut
	if json.Unmarshal([]byte(out.String()), &answer) != nil || answer.Counts["hidden"] != 2 || answer.Findings[0].Face != "-z" {
		t.Errorf("command: %s", out.String())
	}
}

// What the checks know of each kind of material: a blockout texture filed
// under tools draws (its materialType is lit) and hides what it covers; caulk
// hides and doesn't draw; clip and glass are seen through.
func TestMaterialInfoJudgesByMaterialType(t *testing.T) {
	root := materialsInstall(t)
	gdtLine := func(name, typ, usage string) string {
		return "\t\"" + name + "\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"" + typ + "\"\r\n\t\t\"usage\" \"" + usage + "\"\r\n\t}\r\n"
	}
	if err := os.WriteFile(filepath.Join(root, "source_data", "kinds.gdt"), []byte("{\r\n"+
		gdtLine("script_blockout", "lit", "tools")+gdtLine("caulk", "tools_replace", "tools")+gdtLine("clip_player", "tools_blend", "clip")+
		gdtLine("glass_pane", "lit", "window")+gdtLine("model_mat", "lit", "<not in editor>")+"}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := workspace(root)
	if err != nil {
		t.Fatal(err)
	}
	info := materialInfo(w, tilesIn(root))
	for m, want := range map[string][3]bool{ // known, drawn, opaque
		"script_blockout":   {true, true, true},
		"caulk":             {true, false, true},
		"clip_player":       {true, false, false},
		"glass_pane":        {true, true, false},
		"model_mat":         {true, true, true},
		"t7_brick_worn_red": {true, true, true},
		"no_such":           {false, true, true},
	} {
		got := info(m)
		if [3]bool{got.Known, got.Drawn, got.Opaque} != want {
			t.Errorf("%s: %+v, want known, drawn, opaque %v", m, got, want)
		}
	}
	if info("t7_brick_worn_red").Tile.W != 64 || info("t7_brick_worn_red") != info("t7_brick_worn_red") {
		t.Errorf("the brick's tile: %+v", info("t7_brick_worn_red"))
	}
}
