package cli

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// testRadiant is a fake Radiant with internal/level's test map open: a
// worldspawn (a floor brush and an arch curve), a volume_sun and three probes.
func testRadiant(t *testing.T) *radianttest.Fake {
	t.Helper()
	b, err := os.ReadFile("../level/testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(40, `C:\bo3\map_source\test_claude.map`)
	text := string(b)
	f.Text = text[strings.Index(text, "// entity 0"):] // entity text has no header
	onMachine(t, f)
	return f
}

func find(t *testing.T, a mapFindArgs) *mapFindOut {
	t.Helper()
	out, err := mapFind(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ids(out *mapFindOut) []string {
	var s []string
	for _, f := range out.Found {
		s = append(s, f.ID)
	}
	return s
}

func TestMapFindListsEveryEntityWithItsIDAndBounds(t *testing.T) {
	testRadiant(t)
	out := find(t, mapFindArgs{})
	if out.Map != "test_claude.map" || out.Entities != 5 || out.Brushes != 3 || out.Matched != 5 || out.More != "" {
		t.Errorf("summary = %+v", out)
	}
	if got := ids(out); !slices.Equal(got, []string{"8d452111", "5ec8de46", "6e73936d", "76780cea", "f2d51a52"}) {
		t.Errorf("ids = %q", got)
	}
	world, probe, sun := out.Found[0], out.Found[1], out.Found[2]
	if world.Class != "worldspawn" || world.Count != 2 || !slices.Equal(world.Bounds, []float64{-216, -232, -64, 248, 256, 200}) || world.Keys != nil {
		t.Errorf("worldspawn = %+v", world)
	}
	if probe.Class != "reflection_probe" || !slices.Equal(probe.Origin, []float64{44, 108, 4}) || probe.Bounds != nil ||
		len(probe.Keys) != 1 || probe.Keys["name"] != "probe" || probe.List != nil {
		t.Errorf("probe = %+v", probe)
	}
	if sun.Count != 1 || !slices.Equal(sun.Bounds, []float64{-272, -264, -144, 296, 320, 176}) {
		t.Errorf("volume_sun = %+v", sun)
	}
}

func TestMapFindFilters(t *testing.T) {
	testRadiant(t)
	for name, tc := range map[string]struct {
		a    mapFindArgs
		want []string
	}{
		"class glob":          {mapFindArgs{Class: "REFLECTION*"}, []string{"5ec8de46", "76780cea", "f2d51a52"}},
		"key with value":      {mapFindArgs{Key: "name=probe_*"}, []string{"76780cea", "f2d51a52"}},
		"key alone":           {mapFindArgs{Key: "streamLighting"}, []string{"6e73936d"}},
		"near, nearest first": {mapFindArgs{Class: "reflection_probe", Near: "150,120,0", Radius: 250}, []string{"f2d51a52", "5ec8de46", "76780cea"}},
		"near default radius": {mapFindArgs{Class: "reflection_probe", Near: "150 120 30"}, []string{"f2d51a52", "5ec8de46", "76780cea"}},
		"near, small radius":  {mapFindArgs{Near: "150 120 30", Radius: 10, Class: "reflection_probe"}, []string{"f2d51a52"}},
		"ids":                 {mapFindArgs{IDs: []string{"76780CEA", "{F2D51A52-C5B3"}}, []string{"76780cea", "f2d51a52"}},
		"all filters hold":    {mapFindArgs{Class: "volume_sun", Key: "name"}, nil},
	} {
		if got := ids(find(t, tc.a)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: ids = %q, want %q", name, got, tc.want)
		}
	}
}

func TestMapFindListsBrushes(t *testing.T) {
	testRadiant(t)
	out := find(t, mapFindArgs{Class: "worldspawn", Brushes: true})
	list := out.Found[0].List
	if len(list) != 2 {
		t.Fatalf("brush list = %+v", list)
	}
	if f := list[0]; f.ID != "5ec8de49" || f.Material != "aac_wood_mahogany_01" || f.Kind != "" || f.Contents != "" || f.Selected ||
		!slices.Equal(f.Bounds, []float64{-216, -232, -64, 248, 256, -16}) {
		t.Errorf("floor = %+v", list[0])
	}
	if list[1].ID != "af3c9c64" || list[1].Kind != "curve" || !slices.Equal(list[1].Bounds, []float64{-96, -64, -16, 96, 64, 200}) {
		t.Errorf("arch = %+v", list[1])
	}
	named := find(t, mapFindArgs{IDs: []string{"af3c9c64"}})
	if len(named.Found) != 1 || named.Found[0].ID != "8d452111" || len(named.Found[0].List) != 1 || named.Found[0].List[0].ID != "af3c9c64" {
		t.Errorf("a brush by id = %+v", named.Found)
	}
}

func TestMapFindSeesTheUsersSelection(t *testing.T) {
	f := testRadiant(t)
	f.Selection = []string{"{6E73936C-C58E-11F1-B542-00155DD185EA}"} // the volume_sun's brush
	out := find(t, mapFindArgs{Selected: true, Brushes: true})
	if out.Selected != 1 || len(out.Found) != 1 || out.Found[0].ID != "6e73936d" || !out.Found[0].Selected || !out.Found[0].List[0].Selected {
		t.Errorf("selected = %+v", out)
	}
}

func TestMapFindKeysAndLimit(t *testing.T) {
	testRadiant(t)
	all := find(t, mapFindArgs{IDs: []string{"5ec8de46"}, Keys: "all"})
	if k := all.Found[0].Keys; len(k) != 19 || k["resolution"] != "4x" || k["origin"] != "" || k["classname"] != "" {
		t.Errorf("all keys = %v", k)
	}
	some := find(t, mapFindArgs{IDs: []string{"5ec8de46"}, Keys: "resolution, box,nope"})
	if k := some.Found[0].Keys; len(k) != 2 || k["box"] != "1" {
		t.Errorf("listed keys = %v", k)
	}
	limited := find(t, mapFindArgs{Limit: 2})
	if len(limited.Found) != 2 || limited.Matched != 5 || limited.More != "3 more: narrow the query, or raise limit" {
		t.Errorf("limit 2 = %+v", limited)
	}
}

func TestMapFindErrors(t *testing.T) {
	testRadiant(t)
	for a, want := range map[*mapFindArgs]string{
		{Near: "1 2"}:           `near "1 2": give a point`,
		{IDs: []string{"abc"}}:  "no entity or brush abc",
		{IDs: []string{"wall"}}: `"wall" is no id`,
	} {
		if _, err := mapFind(context.Background(), *a); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: err = %v, want %q", *a, err, want)
		}
	}
	onMachine(t)
	if _, err := mapFind(context.Background(), mapFindArgs{}); err != radiant.ErrNoRadiant {
		t.Errorf("no Radiant: err = %v", err)
	}
}

func TestMapFindUnderADialogSaysWhichOne(t *testing.T) {
	f := testRadiant(t)
	f.Dialog = "Entity Info"
	if _, err := mapFind(context.Background(), mapFindArgs{}); err == nil || !strings.Contains(err.Error(), `the dialog "Entity Info" is open`) {
		t.Errorf("err = %v", err)
	}
}

func TestMapFindCommandAndTool(t *testing.T) {
	testRadiant(t)
	cmd := newMapCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"find", "--class", "reflection_probe", "--near", "150,120,30", "--radius", "10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out mapFindOut
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil || len(out.Found) != 1 || out.Found[0].ID != "f2d51a52" {
		t.Errorf("command: %v\n%s", err, buf.String())
	}
	if got := splitIDs(" a,b  c,,"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("splitIDs = %q", got)
	}
	req := mcpRequest("map_find", map[string]any{"class": "volume_sun", "brushes": true})
	res, err := mapFindTool().Handler(context.Background(), req)
	if err != nil || res.IsError || !strings.Contains(resultText(res), `"brush_list":[{"id":"6e73936c","rev":"65e5c5a7","material":"sun_volume"`) {
		t.Errorf("tool: %v %s", err, resultText(res))
	}
}

func mcpRequest(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = name, args
	return req
}
