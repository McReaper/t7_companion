package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// mapSamples measures the map tools on the stock ZM Mod Level template's map,
// what a map made by create holds before any work, served by a fake Radiant
// with the views measured on a real one (a 1648×869 3D view of focal 1212.4,
// a 693×1165 2D view). Images count as Claude prices them, width × height /
// 750 tokens; the rest as text.
func mapSamples(t *testing.T, root string, add func(key, s string), addImage func(key string, w, h, size int)) {
	t.Helper()
	path := filepath.Join(root, "rex", "templates", "ZM Mod Level", "map_source", "zm", "template.map")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Logf("map tools: %v", err)
		return
	}
	text := string(b)
	f := radianttest.New(7, path)
	f.Text = text[strings.Index(text, "// entity 0"):] // Radiant's entities give no header
	f.W3, f.H3, f.Focal, f.W2, f.H2 = 1648, 869, 1212.4, 693, 1165
	f.Menus = map[string]string{"Selection/CSG": "CSG Hollow\t1\t0\nCSG Merge\t1\t0\nCSG Subtract\t1\t0\nAuto Caulk\t1\t0\n"}
	old := radiantMachine
	radiantMachine = radianttest.NewMachine(f)
	defer func() { radiantMachine = old }()
	ctx := context.Background()
	run := func(key string, v any, err error) {
		if err != nil {
			t.Logf("%s: %v", key, err)
			return
		}
		add(key, mcpJSON(t, v))
	}

	var v any
	v, err = radiantOpen(ctx, root, path, 0, false)
	run("radiant_open|the map already open", v, err)
	all, err := mapFind(ctx, mapFindArgs{})
	run("map_find|every entity (the template: 33)", all, err)
	if err != nil {
		return
	}
	zone, start := "", ""
	for _, e := range all.Found {
		switch e.Class {
		case "info_volume":
			zone = e.ID
		case "info_player_start":
			start = e.ID
		}
	}
	v, err = mapFind(ctx, mapFindArgs{Class: "script_struct"})
	run("map_find|class script_struct (15)", v, err)
	v, err = mapFind(ctx, mapFindArgs{Near: "0 -448 40", Radius: 256})
	run("map_find|near the player start, 256 units", v, err)
	v, err = mapFind(ctx, mapFindArgs{Class: "worldspawn", Brushes: true})
	run("map_find|worldspawn's 20 brushes", v, err)
	v, err = mapFind(ctx, mapFindArgs{IDs: []string{zone}, Keys: "all"})
	run("map_find|the start zone, all keys", v, err)

	for _, s := range []struct {
		what string
		a    mapViewArgs
	}{
		{"top, framed on the start zone", mapViewArgs{View: "top", IDs: []string{zone}}},
		{"3d, framed on the player start, render fake", mapViewArgs{IDs: []string{start}, Render: "fake"}},
		{"3d as it is, size 1568", mapViewArgs{Size: maxViewSize}},
	} {
		out, png, err := mapView(ctx, s.a)
		if err != nil {
			t.Logf("map_view %s: %v", s.what, err)
			continue
		}
		add("map_view|"+s.what+": the JSON", mcpJSON(t, out))
		addImage("map_view|"+s.what+fmt.Sprintf(": the %d×%d image", out.W, out.H), out.W, out.H, len(png))
	}

	room := `[` +
		`{"box": {"min": [-1400, -1400, -16], "max": [-900, -900, 0], "material": "script_floor_ceiling"}},` +
		`{"box": {"min": [-1400, -1400, 0], "max": [-1392, -900, 112], "material": "script_wall"}},` +
		`{"box": {"min": [-908, -1400, 0], "max": [-900, -900, 112], "material": "script_wall"}},` +
		`{"box": {"min": [-1392, -1400, 0], "max": [-908, -1392, 112], "material": "script_wall"}},` +
		`{"box": {"min": [-1392, -908, 0], "max": [-908, -900, 112], "material": "script_wall"}},` +
		`{"box": {"min": [-1400, -1400, 112], "max": [-900, -900, 128], "material": "script_floor_ceiling"}},` +
		`{"entity": {"class": "light", "origin": [-1150, -1150, 96], "keys": {"radius": "400"}}}]`
	add("map_edit|the ops sent: a room (floor, 4 walls, ceiling, a light)", room)
	ops, err := decodeOps([]byte(room))
	if err != nil {
		t.Fatal(err)
	}
	edit, err := mapEdit(ctx, ops, "a room", 0)
	run("map_edit|its answer", edit, err)
	moved, _ := decodeOps([]byte(`[{"move": {"ids": ["` + start + `"], "by": [0, 64, 0]}}]`))
	v, err = mapEdit(ctx, moved, "", 0)
	run("map_edit|move the player start (a point entity, in place)", v, err)
	textured := `[` +
		`{"box": {"min": [900, -1400, -16], "max": [1400, -900, 0], "material": "t7_wood_plank_floor_02"}},` +
		`{"box": {"min": [900, -1400, 0], "max": [908, -900, 112], "material": "t7_brick_worn_dirty_red_wet"}},` +
		`{"box": {"min": [1392, -1400, 0], "max": [1400, -900, 112], "material": "t7_brick_worn_dirty_red_wet"}},` +
		`{"texture": {"ids": ["@1", "@2"], "faces": "top,bottom", "material": "caulk"}},` +
		`{"box": {"min": [1000, -1150, 0], "max": [1032, -1118, 112], "material": "t7_concrete_poured_bunker_dirty_01_wet"}},` +
		`{"copy": {"ids": ["@4"], "by": [96, 0, 0], "count": 3}}]`
	add("map_edit|the ops sent: a floor, 2 walls caulked top and bottom, a pillar copied 3 times", textured)
	tops, err := decodeOps([]byte(textured))
	if err != nil {
		t.Fatal(err)
	}
	v, err = mapEdit(ctx, tops, "textures and copies", 0)
	run("map_edit|its answer (with each material's tile)", v, err)
	layoutOp := `[{"layout": {"material": "t7_brick_worn_dirty_red_wet", "floor": "t7_wood_plank_floor_02", "rooms": [` +
		`{"name": "hall", "min": [-1400, 900], "max": [-1144, 1284]}, {"name": "kitchen", "min": [-1144, 900], "max": [-888, 1284]}], ` +
		`"openings": [{"room": "hall", "to": "kitchen"}, {"room": "kitchen", "side": "+x", "sill": 32}]}}]`
	add("map_edit|the op sent: a layout, two rooms with a door between and a window", layoutOp)
	rooms, err := decodeOps([]byte(layoutOp))
	if err != nil {
		t.Fatal(err)
	}
	v, err = mapEdit(ctx, rooms, "two rooms", 0)
	run("map_edit|its answer (17 pieces and 2 probes)", v, err)
	zoneOp := `[{"zone": {"name": "house_zone", "areas": [{"outline": [[-1400, 900], [-888, 900], [-888, 1284], [-1144, 1284], ` +
		`[-1144, 1092], [-1400, 1092]]}], "links": [{"zone": "start_zone"}]}}]`
	add("map_edit|the op sent: a zone over an L outline, linked to the start zone", zoneOp)
	zoned, err := decodeOps([]byte(zoneOp))
	if err != nil {
		t.Fatal(err)
	}
	v, err = mapEdit(ctx, zoned, "a zone", 0)
	run("map_edit|its answer (a volume, 4 spawners, a pathnode, the script's call, a note)", v, err)
	v, err = mapCheck(ctx, nil, "", 0, 0)
	run("map_check|the template and the rooms above (50 listed)", v, err)
	v, err = mapCheck(ctx, nil, "seam,stretched,unknown", 0, 0)
	run("map_check|seams, stretch and unknown materials only", v, err)

	v, err = mapMaterials0(ctx, mapMaterialsArgs{ToolsPath: root, Usage: "wall", Surface: "brick"})
	run("map_materials|brick walls (20 listed)", v, err)
	for _, how := range []struct {
		name string
		a    mapMaterialsArgs
	}{
		{"on a sheet", mapMaterialsArgs{Usage: "floor", Name: "wood", Limit: 6, Show: true}},
		{"on a board", mapMaterialsArgs{Usage: "floor", Name: "wood", Limit: 6, Board: true}},
	} {
		if out, png, err := mapMaterials(ctx, how.a); err == nil {
			add("map_materials|6 wood floors "+how.name+": the JSON", mcpJSON(t, out))
			addImage(fmt.Sprintf("map_materials|6 wood floors %s: the %d×%d image", how.name, out.W, out.H), out.W, out.H, len(png))
		} else {
			t.Logf("map_materials %s: %v", how.name, err)
		}
	}

	v, err = mapCommand(ctx, "", nil, "Selection/CSG", 0)
	run("map_command|list Selection/CSG", v, err)
	box, _ := decodeOps([]byte(`[{"box": {"min": [900, 900, 0], "max": [1400, 1400, 128], "material": "script_wall"}}]`))
	f.Handle("command", hollow) // the views above needed the fake's own commands
	if made, err := mapEdit(ctx, box, "a box", 0); err == nil && len(made.Made) == 1 {
		v, err = mapCommand(ctx, "Selection/CSG/CSG Hollow", []string{made.Made[0].ID}, "", 0)
		run("map_command|CSG Hollow on a box (six walls)", v, err)
	}
}

// hollow is the fake's CSG Hollow, as Radiant does it at grid 8: the
// selected world brush gives way to six 8-unit walls inside its bounds, left
// selected.
func hollow(f *radianttest.Fake, args string, _ []byte) radianttest.Reply {
	if args != "Selection/CSG/CSG Hollow" || len(f.Selection) != 1 {
		return radianttest.Err("missing", "no menu entry "+args)
	}
	m, err := mapfile.Parse([]byte(f.Text))
	if err != nil {
		return radianttest.Err("refused", err.Error())
	}
	world := m.Entities[0]
	for i, it := range world.Items {
		if it.Brush == nil || !strings.EqualFold(it.Brush.Guid(), f.Selection[0]) {
			continue
		}
		b, mat := level.BrushBox(it.Brush), level.Material(it.Brush)
		world.Items = slices.Delete(world.Items, i, i+1)
		lo, hi := b.Min, b.Max
		walls := [6][2]mapfile.Vec3{
			{lo, {hi[0], hi[1], lo[2] + 8}}, {{lo[0], lo[1], hi[2] - 8}, hi},
			{lo, {lo[0] + 8, hi[1], hi[2]}}, {{hi[0] - 8, lo[1], lo[2]}, hi},
			{lo, {hi[0], lo[1] + 8, hi[2]}}, {{lo[0], hi[1] - 8, lo[2]}, hi},
		}
		var text strings.Builder
		f.Selection = nil
		for k, w := range walls {
			g := fmt.Sprintf("{0E%06X-0000-0000-0000-000000000000}", k)
			text.WriteString(strings.Replace(level.BoxBrush(level.Box{Min: w[0], Max: w[1], Ok: true}, mat, ""), "{\n", "{\n guid \""+g+"\"\n", 1))
			f.Selection = append(f.Selection, g)
		}
		wm, _ := mapfile.Parse([]byte("{\n" + text.String() + "}\n"))
		for _, wb := range wm.Entities[0].Brushes() {
			world.Items = append(world.Items, mapfile.Item{Brush: wb})
		}
		f.Text, f.UndoLabel, f.Modified = string(m.Bytes()), "hollow", true
		return radianttest.OK("", nil)
	}
	return radianttest.Err("disabled", args+" is disabled")
}

// mapMaterials0 is mapMaterials' answer alone, for a list with no image.
func mapMaterials0(ctx context.Context, a mapMaterialsArgs) (*mapMaterialsOut, error) {
	out, _, err := mapMaterials(ctx, a)
	return out, err
}
