package level

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// zoning is the test map in a fake Radiant with a start zone over x and y
// -2000 to -1000, as a template's start room has one.
func zoning(t *testing.T, pid int) (*radiant.Client, *Snapshot) {
	t.Helper()
	b, err := os.ReadFile("testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)[strings.Index(string(b), "// entity 0"):]
	n := strings.Count(text, "// entity ")
	start := strings.Replace(EntityText("info_volume", map[string]string{"targetname": "start_zone", "target": "start_zone_spawners", "script_noteworthy": "player_volume"},
		BoxBrush(Box{Min: mapfile.Vec3{-2000, -2000, 0}, Max: mapfile.Vec3{-1000, -1000, 256}, Ok: true}, "volume", "")),
		"{\n", "{\nguid \"{0D00AA01-0000-0000-0000-000000000000}\"\n", 1)
	f := radianttest.New(pid, `C:\m\test_claude.map`)
	f.Text = text + fmt.Sprintf("// entity %d\n", n) + strings.Replace(start, "BoxBrush", "", 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := radiant.NewClient(ctx, f.Dial())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	snap, err := Read(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	return c, snap
}

// what is the entities of a part, read back from the map after the edit.
func partsOf(t *testing.T, d *Done) map[string][]*mapfile.Entity {
	t.Helper()
	out := map[string][]*mapfile.Entity{}
	for _, m := range d.Made {
		for _, e := range d.After.Map.Entities {
			if hexOf(e.Guid()) == hexOf(m.ID) {
				out[m.Part] = append(out[m.Part], e)
			}
		}
	}
	return out
}

// An L-shaped zone over a floor with a pillar in it: one volume of two
// brushes, riser spawners spread on the floor clear of the pillar, a
// pathnode near its middle, and its link to the start zone for the script.
func TestAZoneCoversItsOutlineAndPutsItsSpawnersOnTheFloor(t *testing.T) {
	c, snap := zoning(t, 701)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [0, 0, -16], "max": [512, 512, 0], "material": "brick"}},
		{"box": {"min": [96, 96, 0], "max": [160, 160, 128], "material": "brick"}},
		{"zone": {"name": "yard_zone", "areas": [{"outline": [[0, 0], [512, 0], [512, 256], [256, 256], [256, 512], [0, 512]], "height": 160}],
			"links": [{"zone": "start_zone"}]}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, d)
	vol := parts["yard_zone volume"]
	if len(vol) != 1 || len(vol[0].Brushes()) != 2 || len(parts["yard_zone spawner"]) != 4 || len(parts["yard_zone pathnode"]) != 1 {
		t.Fatalf("parts %v", parts)
	}
	for k, want := range map[string]string{"classname": "info_volume", "targetname": "yard_zone", "target": "yard_zone_spawners", "script_noteworthy": "player_volume"} {
		if v, _ := vol[0].Key(k); v != want {
			t.Errorf("the volume's %s: %q", k, v)
		}
	}
	if bx := EntityBox(vol[0]); bx.Min != (mapfile.Vec3{0, 0, 0}) || bx.Max != (mapfile.Vec3{512, 512, 160}) {
		t.Errorf("volume bounds %+v", bx)
	}
	var at []string
	for _, e := range parts["yard_zone spawner"] {
		o, _ := Origin(e)
		k := map[string]string{}
		for _, n := range []string{"targetname", "script_noteworthy", "script_string"} {
			k[n], _ = e.Key(n)
		}
		if fmt.Sprint(k) != "map[script_noteworthy:riser_location script_string:find_flesh targetname:yard_zone_spawners]" || o[2] != 0 {
			t.Errorf("spawner %v %v", o, k)
		}
		if o[0] > 256 && o[1] > 256 || o[0] >= 96-halfWidth && o[0] <= 160+halfWidth && o[1] >= 96-halfWidth && o[1] <= 160+halfWidth {
			t.Errorf("a spawner at %v: outside the L, or against the pillar", o)
		}
		at = append(at, fmt.Sprint(o))
		if a, _ := e.Key("angles"); o[0] == 32 && o[1] == 32 && a != "0 45 0" {
			t.Errorf("the corner spawner faces %q: the L's middle, on its diagonal, is at 45 degrees", a)
		}
	}
	sort.Strings(at)
	if strings.Join(at, " ") != "[224 256 0] [32 32 0] [32 480 0] [480 32 0]" { // three ends, then the elbow farthest from them
		t.Errorf("spawners at %s: spread to the L's ends", strings.Join(at, " "))
	}
	if o, _ := Origin(parts["yard_zone pathnode"][0]); o[2] != 2 {
		t.Errorf("pathnode at %v: on the floor", o)
	}
	if fmt.Sprint(d.Links) != "[{yard_zone start_zone always_on false}]" || len(d.Notes) != 1 ||
		d.Notes[0] != "zones yard_zone and start_zone are linked but 1414 units apart: a player crossing between them is in neither, and dies there; grow one to meet the other" { // corner to corner
		t.Errorf("links %v, notes %v", d.Links, d.Notes)
	}
}

// A zone the map already has keeps its spawners' name, the one its first
// volume targets; zones whose bounds overlap are noted.
func TestAZoneAddsToTheMapsZoneAndNotesOverlaps(t *testing.T) {
	c, snap := zoning(t, 702)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [-2000, -2000, -16], "max": [0, 0, 0], "material": "brick"}},
		{"zone": {"name": "start_zone", "areas": [{"min": [-1000, -2000, 0], "max": [-500, -1000, 128]}], "spawners": 1, "pathnode": false}},
		{"zone": {"name": "hall_zone", "areas": [{"min": [-1200, -1800, 0], "max": [-800, -1200, 128]}], "spawners": 0, "pathnode": false}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, d)
	if v, _ := parts["start_zone spawner"][0].Key("targetname"); v != "start_zone_spawners" || len(parts["hall_zone spawner"]) != 0 || len(parts["start_zone pathnode"]) != 0 {
		t.Errorf("parts %v", parts)
	}
	if strings.Join(d.Notes, "\n") != "zone hall_zone overlaps zone start_zone: a player inside both counts in both; shrink or delete one's volumes" {
		t.Errorf("notes %q", d.Notes)
	}
}

// Floors give a zone their own outline: here a wedge, the zone's bottom at
// its top's lowest, its top height over its highest.
func TestAZoneFollowsItsFloors(t *testing.T) {
	c, snap := zoning(t, 703)
	b := newBatch(snap, nil)
	wedge := prismBrush([]pt2{{0, 0}, {300, 0}, {0, 300}}, -16, 0, "brick", texNumbers(64, 64))
	if err := b.made(0, EntityText("worldspawn", nil, wedge), true); err != nil {
		t.Fatal(err)
	}
	if err := b.zone(1, &ZoneOp{Name: "wedge_zone", Areas: []ZoneArea{{Floors: []string{"@0"}, Height: 100}}}); err != nil {
		t.Fatal(err)
	}
	a := b.zones[0].areas[0]
	if len(a.parts) != 1 || len(a.parts[0]) != 3 || a.z0 != 0 || a.z1 != 100 {
		t.Errorf("area %+v", a)
	}
	if _, err := b.plan(); err != nil {
		t.Error(err)
	}
	_ = c
}

func TestAZoneRefusesWhatItCantBuild(t *testing.T) {
	c, snap := zoning(t, 704)
	floor := `{"box": {"min": [0, 0, -16], "max": [512, 512, 0], "material": "brick"}},`
	for want, op := range map[string]string{
		`name "two words": one word`:   `{"zone": {"name": "two words", "areas": [{"min": [0,0,0], "max": [64,64,64]}]}}`,
		"no areas":                     `{"zone": {"name": "z"}}`,
		"spawners 100: 0 to 64":        `{"zone": {"name": "z", "spawners": 100, "areas": [{"min": [0,0,0], "max": [64,64,64]}]}}`,
		"one of min and max":           `{"zone": {"name": "z", "areas": [{"min": [0,0,0], "max": [64,64,64], "outline": [[0,0],[1,0],[0,1]]}]}}`,
		"a box takes both min and max": `{"zone": {"name": "z", "areas": [{"min": [0,0,0]}]}}`,
		"must exceed min":              `{"zone": {"name": "z", "areas": [{"min": [0,0,0], "max": [64,0,64]}]}}`,
		"height -5: above 0":           `{"zone": {"name": "z", "areas": [{"outline": [[0,0],[100,0],[0,100]], "height": -5}]}}`,
		"crosses itself":               `{"zone": {"name": "z", "areas": [{"outline": [[0,0],[100,100],[100,0],[0,100]]}]}}`,
		"no floor under its areas":     `{"zone": {"name": "z", "areas": [{"min": [2000,2000,0], "max": [2100,2100,64]}]}}`,
		"link to cellar_zone: no zone": floor + `{"zone": {"name": "z", "areas": [{"min": [0,0,0], "max": [512,512,64]}], "links": [{"zone": "cellar_zone"}]}}`,
		"another zone's name":          floor + `{"zone": {"name": "z", "areas": [{"min": [0,0,0], "max": [512,512,64]}], "links": [{"zone": "z"}]}}`,
		"a flag of one word":           floor + `{"zone": {"name": "z", "areas": [{"min": [0,0,0], "max": [512,512,64]}], "links": [{"zone": "start_zone", "flag": "a b"}]}}`,
		"floors are brushes":           `{"entity": {"class": "light", "origin": [0,0,0]}}, {"zone": {"name": "z", "areas": [{"floors": ["@0"]}]}}`,
	} {
		_, err := Edit(context.Background(), c, snap, ops(t, "["+strings.TrimSuffix(op, ",")+"]"), "", nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
}

// A layout's rooms name their zones: a zone over each name's rooms, linked
// where a door joins two; a room in none is noted.
func TestALayoutsRoomsMakeTheirZones(t *testing.T) {
	c, snap := zoning(t, 705)
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"layout": {"material": "brick", "probes": false, "spawners": 2, "rooms": [
		{"name": "hall", "min": [0, 0], "max": [256, 256], "zone": "house_zone"},
		{"name": "kitchen", "min": [256, 0], "max": [512, 256], "zone": "house_zone"},
		{"name": "yard", "min": [0, -256], "max": [512, 0], "outdoor": true, "zone": "yard_zone"},
		{"name": "shed", "min": [512, -256], "max": [640, 0]}],
		"openings": [{"room": "hall", "to": "kitchen"}, {"room": "hall", "to": "yard"}, {"room": "kitchen", "to": "yard"}, {"room": "yard", "to": "shed"}]}}]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	parts := partsOf(t, d)
	if len(parts["house_zone volume"]) != 2 || len(parts["yard_zone volume"]) != 1 || len(parts["house_zone spawner"]) != 2 || len(parts["yard_zone pathnode"]) != 1 {
		t.Fatalf("parts %v", parts)
	}
	if bx := EntityBox(parts["yard_zone volume"][0]); bx.Min != (mapfile.Vec3{0, -256, 0}) || bx.Max != (mapfile.Vec3{512, 0, 128}) {
		t.Errorf("the yard's zone, to its walls' top: %+v", bx)
	}
	rooms := map[bool]int{}
	node, _ := Origin(parts["house_zone pathnode"][0])
	for _, e := range append(parts["house_zone spawner"], parts["yard_zone spawner"]...) {
		o, _ := Origin(e)
		if o[2] != 0 || o[0] == node[0] && o[1] == node[1] {
			t.Errorf("a spawner off the floor, or on the pathnode %v: %v", node, o)
		}
		if o[1] > 0 {
			rooms[o[0] < 256]++
		}
	}
	if rooms[true] != 1 || rooms[false] != 1 {
		t.Errorf("the house's two spawners, one in each of its rooms: %v", rooms)
	}
	if fmt.Sprint(d.Links) != "[{house_zone yard_zone always_on false}]" || strings.Join(d.Notes, "|") != "rooms shed are in no zone: a player there is killed within 3 seconds" {
		t.Errorf("links %v, notes %q", d.Links, d.Notes)
	}
}

// Zones are compared by their pieces, not their bounds: two Ls that wind
// round each other, their bounds overlapping, only touch, and linked they
// meet; a zone over another, a storey up, touches it from above.
func TestZonesThatOnlyTouchArentNotedAsOverlapping(t *testing.T) {
	c, snap := zoning(t, 706)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [0, 0, -16], "max": [512, 512, 0], "material": "brick"}},
		{"zone": {"name": "a_zone", "areas": [{"outline": [[0, 0], [512, 0], [512, 128], [128, 128], [128, 512], [0, 512]]}], "spawners": 0, "pathnode": false,
			"links": [{"zone": "b_zone"}, {"zone": "c_zone"}]}},
		{"zone": {"name": "b_zone", "areas": [{"outline": [[128, 128], [512, 128], [512, 512], [128, 512]]}], "spawners": 0, "pathnode": false}},
		{"zone": {"name": "c_zone", "areas": [{"min": [0, 0, 128], "max": [128, 128, 256]}], "spawners": 0, "pathnode": false}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notes) != 0 {
		t.Errorf("notes %q", d.Notes)
	}
	if bx := (zpart{rect(0, 0, 10, 10), 0, 10}); apart([]zpart{bx}, []zpart{{rect(13, 14, 20, 20), 0, 10}}) != 5 || apart([]zpart{bx}, []zpart{{rect(0, 0, 10, 10), 22, 30}}) != 12 {
		t.Error("3 across and 4 up is 5 apart; 12 up is 12")
	}
}

// zoningWith is zoning's map with more entities in it.
func zoningWith(t *testing.T, pid int, extra string) (*radiant.Client, *Snapshot) {
	t.Helper()
	b, err := os.ReadFile("testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)[strings.Index(string(b), "// entity 0"):]
	f := radianttest.New(pid, `C:\m\test_claude.map`)
	f.Text = text + fmt.Sprintf("// entity %d\n", strings.Count(text, "// entity ")) + extra
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	c, err := radiant.NewClient(ctx, f.Dial())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	snap, err := Read(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	return c, snap
}

// A zone the map has keeps the name its first volume gives its spawners,
// whatever it is.
func TestAZoneKeepsItsSpawnersOwnName(t *testing.T) {
	yard := strings.Replace(EntityText("info_volume", map[string]string{"targetname": "yard_zone", "target": "yard_risers", "script_noteworthy": "player_volume"},
		BoxBrush(Box{Min: mapfile.Vec3{-2000, -2000, 0}, Max: mapfile.Vec3{-1000, -1000, 256}, Ok: true}, "volume", "")),
		"{\n", "{\nguid \"{0D00AA02-0000-0000-0000-000000000000}\"\n", 1)
	c, snap := zoningWith(t, 707, yard)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [0, 0, -16], "max": [256, 256, 0], "material": "brick"}},
		{"zone": {"name": "yard_zone", "areas": [{"min": [0, 0, 0], "max": [256, 256, 128]}], "spawners": 1, "pathnode": false}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := partsOf(t, d)["yard_zone spawner"][0].Key("targetname"); v != "yard_risers" {
		t.Errorf("the spawner is %q's", v)
	}
}

// Spawners: the grid reaches an area's far side; a room gets its spawner
// before a doorway; a room with no floor gets none and the others still
// spread; three rooms, one far bigger, get one each.
func TestSpawnersSpreadOverAZonesRooms(t *testing.T) {
	c, snap := zoning(t, 708)
	spawners := func(js string) []mapfile.Vec3 {
		t.Helper()
		d, err := Edit(context.Background(), c, snap, ops(t, js), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		var out []mapfile.Vec3
		for _, e := range partsOf(t, d)["z spawner"] {
			o, _ := Origin(e)
			out = append(out, o)
		}
		sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
		return out
	}
	floor := `{"box": {"min": [0, 0, -16], "max": [2000, 2000, 0], "material": "brick"}},`
	at := spawners(`[` + floor + `{"zone": {"name": "z", "areas": [{"min": [0, 0, 0], "max": [500, 64, 128]}], "spawners": 2, "pathnode": false}}]`)
	if fmt.Sprint(at) != "[[32 32 0] [480 32 0]]" {
		t.Errorf("a strip 500 long: %v, its ends", at)
	}
	at = spawners(`[` + floor + `{"zone": {"name": "z", "areas": [{"min": [0, 0, 0], "max": [96, 96, 128]}, {"min": [96, 0, 0], "max": [1000, 600, 128]}], "spawners": 1, "pathnode": false}}]`)
	if len(at) != 1 || at[0][0] < 96 {
		t.Errorf("a doorway and a room, one spawner: %v, in the room", at)
	}
	at = spawners(`[{"box": {"min": [0, 0, -16], "max": [256, 256, 0], "material": "brick"}},` +
		`{"zone": {"name": "z", "areas": [{"min": [3000, 3000, 0], "max": [3256, 3256, 128]}, {"min": [0, 0, 0], "max": [256, 256, 128]}], "spawners": 2, "pathnode": false}}]`)
	if len(at) != 2 || at[0][0] > 256 || at[1][0] > 256 || flat(at[0], at[1]) < 250 {
		t.Errorf("a room with no floor beside one with: %v, both in the floored one, apart", at)
	}
	at = spawners(`[` + floor + `{"zone": {"name": "z", "areas": [{"min": [0, 0, 0], "max": [128, 128, 128]}, {"min": [128, 0, 0], "max": [256, 128, 128]}, ` +
		`{"min": [0, 128, 0], "max": [1500, 1500, 128]}], "spawners": 3, "pathnode": false}}]`)
	rooms := map[int]int{}
	for _, p := range at {
		switch {
		case p[1] > 128:
			rooms[2]++
		case p[0] < 128:
			rooms[0]++
		default:
			rooms[1]++
		}
	}
	if fmt.Sprint(rooms) != "map[0:1 1:1 2:1]" {
		t.Errorf("two small rooms and a big one: %v, one each", at)
	}
}

// A layout whose rooms are all zoned says nothing of them; a floor brush on
// a slope gives its zone the slope's height, from its lowest to its
// highest.
func TestZonesOfALayoutAndASlope(t *testing.T) {
	c, snap := zoning(t, 709)
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"layout": {"material": "brick", "probes": false, "spawners": 1, "rooms": [
		{"name": "a", "min": [0, 0], "max": [256, 256], "zone": "a_zone"}, {"name": "b", "min": [256, 0], "max": [512, 256], "zone": "a_zone"}]}}]`), "", nil)
	if err != nil || len(d.Notes) != 0 {
		t.Errorf("%v, notes %q", err, d.Notes)
	}
	a, ok := floorArea(mustBrush(t, wedge("brick", 0, 64, mapfile.Vec3{1000, 0, 0})), 100)
	if !ok || math.Abs(a.z0-0.32) > 1e-6 || math.Abs(a.z1-163.68) > 1e-6 {
		t.Errorf("over a ramp 64 high: %+v, %v", a, ok)
	}
}

func mustBrush(t *testing.T, text string) *mapfile.Brush {
	t.Helper()
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, text)))
	if err != nil {
		t.Fatal(err)
	}
	return m.Entities[0].Brushes()[0]
}

func TestInOutlineIsInsideNotOnTheEdge(t *testing.T) {
	l := []pt2{{0, 0}, {200, 0}, {200, 100}, {100, 100}, {100, 200}, {0, 200}}
	for p, want := range map[pt2]bool{
		{50, 50}: true, {150, 50}: true, {50, 150}: true, // the L's three squares
		{150, 150}: false, {250, 50}: false, {-1, 50}: false, // its notch, and outside
		{0, 50}: false, {100, 150}: false, {150, 100}: false, {200, 50}: false, // on its edges
		{100, 100}: false, {0, 0}: false, // on its corners
		{50, 100}: true, {150, 99}: true, // level with a corner, inside
	} {
		if got := inOutline(l, p); got != want {
			t.Errorf("%v: %v, want %v", p, got, want)
		}
	}
}

// A point's distance to a segment: to its nearest point inside it, or to an
// end; a segment of no length is its point. A volume's brush is a piece of
// its zone, from its bottom to its top.
func TestSegmentDistanceAndBrushParts(t *testing.T) {
	for _, c := range []struct {
		p, a, b pt2
		want    float64
	}{
		{pt2{5, 3}, pt2{1, 1}, pt2{9, 1}, 2},  // over its middle
		{pt2{3, 9}, pt2{3, 1}, pt2{3, 5}, 4},  // past its end
		{pt2{-3, 1}, pt2{1, 1}, pt2{9, 1}, 4}, // before its start
		{pt2{4, 5}, pt2{1, 1}, pt2{1, 1}, 5},  // no length
		{pt2{7, 7}, pt2{2, 2}, pt2{12, 2}, 5}, // over it, off the origin
	} {
		if got := segmentDistance(c.p, c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%v to %v-%v: %v, want %v", c.p, c.a, c.b, got, c.want)
		}
	}
	p, ok := brushPart(mustBrush(t, BoxBrush(Box{Min: mapfile.Vec3{10, 20, -30}, Max: mapfile.Vec3{40, 60, 50}, Ok: true}, "volume", "")))
	if !ok || p.z0 != -30 || p.z1 != 50 || len(p.hull) != 4 {
		t.Errorf("a box's piece: %+v, %v", p, ok)
	}
}
