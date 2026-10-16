package level

import (
	"context"
	"fmt"
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
	}
	sort.Strings(at)
	if strings.Join(at, " ") != "[224 256 0] [32 32 0] [32 480 0] [480 32 0]" { // three ends, then the elbow farthest from them
		t.Errorf("spawners at %s: spread to the L's ends", strings.Join(at, " "))
	}
	if o, _ := Origin(parts["yard_zone pathnode"][0]); o[2] != 2 {
		t.Errorf("pathnode at %v: on the floor", o)
	}
	if fmt.Sprint(d.Links) != "[{yard_zone start_zone always_on false}]" || len(d.Notes) != 1 ||
		d.Notes[0] != "zones yard_zone and start_zone are linked but 1000 units apart: a player crossing between them is in neither, and dies there; grow one to meet the other" {
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
	if strings.Join(d.Notes, "\n") != "zone hall_zone overlaps zone start_zone (their bounds): a player inside both counts in both; shrink or delete one's volumes" {
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
