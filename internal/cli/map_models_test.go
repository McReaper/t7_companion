package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// modelsInstall is materialsInstall with props: xmodels, and maps placing
// some of them.
func modelsInstall(t *testing.T) string {
	t.Helper()
	root := materialsInstall(t)
	put := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var gdt strings.Builder
	for _, m := range []string{"p7_barrel_blue", "p7_barrel_red", "p7_chair_wood", "p7_chair_metal", "p7_table_wood"} {
		fmt.Fprintf(&gdt, "\t\"%s\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t}\r\n", m)
	}
	put("model_export/props.gdt", "{\r\n"+gdt.String()+"}\r\n")
	ent := func(m string) string { return "{\n\"classname\" \"misc_model\"\n\"model\" \"" + m + "\"\n}\n" }
	put("map_source/zm/b.map", ent("p7_chair_wood")+ent("p7_chair_wood")+ent("p7_barrel_red")+ent("p7_table_wood")+ent("p7_table_wood")+ent("p7_table_wood"))
	return root
}

func TestMapModelsListsTheModelsMostPlacedFirst(t *testing.T) {
	root := modelsInstall(t)
	for _, c := range []struct {
		a    mapModelsArgs
		want string
	}{
		{mapModelsArgs{}, "p7_table_wood:3 p7_chair_wood:2 p7_barrel_red:1 p7_barrel_blue:0 p7_chair_metal:0"},
		{mapModelsArgs{Name: "barrel"}, "p7_barrel_red:1 p7_barrel_blue:0"},
		{mapModelsArgs{Name: "wood chair, table"}, "p7_table_wood:3 p7_chair_wood:2"},
		{mapModelsArgs{Name: "sofa"}, ""},
		{mapModelsArgs{Limit: 2}, "p7_table_wood:3 p7_chair_wood:2"},
	} {
		c.a.ToolsPath = root
		out, png, err := mapModels(context.Background(), c.a)
		if err != nil || png != nil {
			t.Fatalf("%+v: %v", c.a, err)
		}
		var got []string
		for _, m := range out.Models {
			got = append(got, fmt.Sprintf("%s:%d", m.Name, m.Uses))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%+v: %q, want %q", c.a, strings.Join(got, " "), c.want)
		}
	}
	if out, _, _ := mapModels(context.Background(), mapModelsArgs{ToolsPath: root, Limit: 2}); out.Found != 5 {
		t.Errorf("found %d: the five props, before the limit", out.Found)
	}
	if _, _, err := mapModels(context.Background(), mapModelsArgs{ToolsPath: root, Limit: maxMaterialsLimit, Size: maxViewSize}); err != nil {
		t.Errorf("the most a list and an image take: %v", err)
	}
	for _, a := range []mapModelsArgs{{ToolsPath: root, Limit: -1}, {ToolsPath: root, Limit: 201}, {ToolsPath: root, Size: 2000}, {ToolsPath: root, Size: -1}} {
		if _, _, err := mapModels(context.Background(), a); err == nil {
			t.Errorf("%+v: no error", a)
		}
	}
}

// board places the models in the Radiant running and answers its shot, which
// label is which model; the board is taken off the map again.
func TestMapModelsShowsThemInRadiant(t *testing.T) {
	root := modelsInstall(t)
	b, err := os.ReadFile("../level/testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(43, filepath.Join(root, "map_source", "zm", "test_claude.map"))
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	onMachine(t, f)
	t.Setenv("TA_TOOLS_PATH", "")
	res, err := mapModelsTool().Handler(context.Background(), mcpRequest("map_models", map[string]any{"name": "chair", "board": true}))
	if err != nil || res.IsError || len(res.Content) != 2 {
		t.Fatalf("%v %s", err, resultText(res))
	}
	var out mapModelsOut
	if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &out); err != nil || len(out.Board) != 2 || out.W == 0 {
		t.Fatalf("answer %+v, %v", out, err)
	}
	for i, l := range out.Board {
		if l.Model != out.Models[i].Name || l.Label != fmt.Sprint(i+1) {
			t.Errorf("board %d: %+v", i, l)
		}
	}
	if f.Text != string(b)[strings.Index(string(b), "// entity 0"):] {
		t.Error("the board was left on the map")
	}
	if many, _, err := mapModels(context.Background(), mapModelsArgs{Name: "p7", Board: true}); err != nil || len(many.Board) != 5 || len(many.Notes) != 0 {
		t.Errorf("five shown, all on the board, nothing to say: %+v, %v", many, err)
	}
	// the CLI: the image written where --out says
	cmd := newMapCmd()
	var stdout, stderr strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	png := filepath.Join(t.TempDir(), "m.png")
	cmd.SetArgs([]string{"models", "--name", "barrel", "--board", "--out", png})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(png); err != nil || !strings.Contains(stderr.String(), "wrote "+png) || !strings.Contains(stdout.String(), `"model": "p7_barrel_red"`) {
		t.Errorf("%v %s %s", err, stderr.String(), stdout.String())
	}
}

// The map tools refuse an argument they don't take, naming the ones they do:
// map_materials' show is a sheet, map_models has none, and a show sent to it
// must not answer the list as if it had been heard.
func TestMapToolsRefuseAnArgumentTheyDontTake(t *testing.T) {
	root := modelsInstall(t)
	var models server.ServerTool
	for _, tool := range mapToolset().tools {
		if tool.Tool.Name == "map_models" {
			models = tool
		}
	}
	res, err := models.Handler(context.Background(), mcpRequest("map_models", map[string]any{"name": "barrel", "show": true, "tools_path": root}))
	if err != nil || !res.IsError || resultText(res) != `map_models takes no "show": it takes board, limit, name, pid, size, tools_path` {
		t.Errorf("%v %s", err, resultText(res))
	}
	res, err = models.Handler(context.Background(), mcpRequest("map_models", map[string]any{"name": "barrel", "tools_path": root}))
	if err != nil || res.IsError || !strings.Contains(resultText(res), `"p7_barrel_red"`) {
		t.Errorf("the arguments it takes go through: %v %s", err, resultText(res))
	}
}
