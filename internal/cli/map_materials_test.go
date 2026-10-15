package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// materialsInstall is an install with six editor materials, one of them DO
// NOT USE, and a map that uses two of them.
func materialsInstall(t *testing.T) string {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
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
	write("deffiles/material.awi", "")
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", "")
	mat := func(name, usage, surface string, more ...string) string {
		s := "\t\"" + name + "\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"" + usage + "\"\r\n\t\t\"surfaceType\" \"" + surface + "\"\r\n" +
			"\t\t\"tilingWidth\" \"1024\"\r\n\t\t\"tilingHeight\" \"512\"\r\n"
		for i := 0; i < len(more); i += 2 {
			s += "\t\t\"" + more[i] + "\" \"" + more[i+1] + "\"\r\n"
		}
		return s + "\t}\r\n"
	}
	write("source_data/mats.gdt", "{\r\n"+
		mat("t7_brick_worn_red", "exterior wall", "brick")+
		mat("t7_brick_clean_red", "interior wall", "brick")+
		mat("t7_plaster_white", "interior wall", "plaster")+
		mat("t7_wood_planks", "floor", "wood")+
		mat("t7_metal_grate", "floor", "metal")+
		mat("old_script_wall", "tools", "concrete", "doNotUse", "1")+
		"\t\"t7_model_only\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"<not in editor>\"\r\n\t}\r\n}\r\n")
	face := func(mat string) string {
		return " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) " + mat + " 128 128 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n"
	}
	write("map_source/zm/a.map", "{\n{\n"+strings.Repeat(face("t7_plaster_white"), 12)+strings.Repeat(face("t7_brick_clean_red"), 3)+"}\n}\n")
	return root
}

func TestMapMaterialsListsTheBrowsersMaterialsMostUsedFirst(t *testing.T) {
	root := materialsInstall(t)
	list := func(a mapMaterialsArgs) []string {
		t.Helper()
		a.ToolsPath = root
		out, png, err := mapMaterials(context.Background(), a)
		if err != nil || png != nil {
			t.Fatalf("%+v: %v", a, err)
		}
		var names []string
		for _, m := range out.Materials {
			names = append(names, m.Name)
		}
		return names
	}
	for _, c := range []struct {
		a    mapMaterialsArgs
		want string
	}{
		{mapMaterialsArgs{}, "t7_plaster_white t7_brick_clean_red t7_brick_worn_red t7_metal_grate t7_wood_planks"},
		{mapMaterialsArgs{Usage: "wall"}, "t7_plaster_white t7_brick_clean_red t7_brick_worn_red"},
		{mapMaterialsArgs{Usage: "Floor, exterior"}, "t7_brick_worn_red t7_metal_grate t7_wood_planks"},
		{mapMaterialsArgs{Surface: "brick", Usage: "interior"}, "t7_brick_clean_red"},
		{mapMaterialsArgs{Name: "red worn, grate"}, "t7_brick_worn_red t7_metal_grate"},
		{mapMaterialsArgs{Name: "brick_clean"}, "t7_brick_clean_red"},
		{mapMaterialsArgs{Name: " , "}, ""},
		{mapMaterialsArgs{Usage: "tools"}, ""},
		{mapMaterialsArgs{Usage: "tools", All: true}, "old_script_wall"},
		{mapMaterialsArgs{Limit: 2}, "t7_plaster_white t7_brick_clean_red"},
	} {
		if got := strings.Join(list(c.a), " "); got != c.want {
			t.Errorf("%+v: %q, want %q", c.a, got, c.want)
		}
	}
	out, _, _ := mapMaterials(context.Background(), mapMaterialsArgs{ToolsPath: root, Name: "plaster, script", All: true})
	if out.Found != 2 || out.Materials[0] != (materialOut{Name: "t7_plaster_white", Usage: "interior wall", Surface: "plaster", Tile: [2]float64{128, 128}, Faces: 12}) ||
		out.Materials[1] != (materialOut{Name: "old_script_wall", Usage: "tools", Surface: "concrete", Tile: [2]float64{64, 32}, DoNotUse: true}) {
		t.Errorf("answer %+v: the maps' tile for one they use 10 times or more, its GDT's tiling at 16 a unit else", out)
	}
	if _, _, err := mapMaterials(context.Background(), mapMaterialsArgs{ToolsPath: root, Limit: 200, Size: maxViewSize}); err != nil {
		t.Errorf("the most a list and an image take: %v", err)
	}
	for _, a := range []mapMaterialsArgs{{ToolsPath: root, Limit: -1}, {ToolsPath: root, Limit: 201}, {ToolsPath: root, Size: 2000}, {ToolsPath: root, Size: -1}} {
		if _, _, err := mapMaterials(context.Background(), a); err == nil {
			t.Errorf("%+v: no error", a)
		}
	}
}

// board lays the first 16 out in the Radiant running, its image beside the
// answer; with no install named, the Radiant's own is read.
func TestMapMaterialsBoardsTheFirstSixteenInRadiant(t *testing.T) {
	root := materialsInstall(t)
	b, err := os.ReadFile("../level/testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(42, filepath.Join(root, "map_source", "zm", "test_claude.map"))
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	onMachine(t, f)
	t.Setenv("TA_TOOLS_PATH", "")
	var many strings.Builder // eighteen floors, written before the install is first read
	for i := range 18 {
		fmt.Fprintf(&many, "\t\"m%02d\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"floor\"\r\n\t}\r\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "source_data", "many.gdt"), []byte("{\r\n"+many.String()+"}\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := mapMaterialsTool().Handler(context.Background(), mcpRequest("map_materials", map[string]any{"usage": "wall", "board": true}))
	if err != nil || res.IsError || len(res.Content) != 2 {
		t.Fatalf("%v %s", err, resultText(res))
	}
	var out mapMaterialsOut
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out); err != nil || len(out.Board) != 3 || out.W == 0 {
		t.Fatalf("answer %+v, %v", out, err)
	}
	for i, s := range out.Board {
		if s.Material != out.Materials[i].Name || s.Label != fmt.Sprint(i+1) || s.At == [2]int{} {
			t.Errorf("board %d: %+v", i, s)
		}
	}
	if png, err := base64.StdEncoding.DecodeString(res.Content[1].(mcp.ImageContent).Data); err != nil || len(png) < 8 {
		t.Errorf("image: %v", err)
	}
	// more than a board holds: the first sixteen, said so
	cmd := newMapCmd()
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	png := filepath.Join(t.TempDir(), "m.png")
	cmd.SetArgs([]string{"materials", "--name", "m0, m1", "--usage", "floor", "--limit", "30", "--board", "--out", png})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var many2 mapMaterialsOut
	if err := json.Unmarshal([]byte(stdout.String()), &many2); err != nil || len(many2.Board) != 16 || len(many2.Materials) != 18 ||
		!strings.Contains(strings.Join(many2.Notes, " "), "the board shows the first 16") {
		t.Errorf("%v %+v", err, many2)
	}
	if sixteen, _, err := mapMaterials(context.Background(), mapMaterialsArgs{Name: "m0, m1", Usage: "floor", Limit: 16, Board: true}); err != nil ||
		len(sixteen.Board) != 16 || len(sixteen.Notes) != 0 {
		t.Errorf("sixteen listed, all on the board, nothing to say: %+v, %v", sixteen, err)
	}
	if _, err := os.Stat(png); err != nil || !strings.Contains(stderr.String(), "wrote "+png) {
		t.Errorf("the image: %v, %s", err, stderr.String())
	}
}
