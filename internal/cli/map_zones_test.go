package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// zonesInstall is an install holding zm_x's script, which already links the
// house to the yard.
func zonesInstall(t *testing.T, gsc string) (root, mapPath string) {
	t.Helper()
	root = t.TempDir()
	for rel, body := range map[string]string{"deffiles/material.awi": "", "usermaps/zm_x/scripts/zm/zm_x.gsc": gsc} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if body != "-" {
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root, filepath.Join(root, "map_source", "zm", "zm_x.map")
}

const zoneGSC = "function main()\r\n{\r\n\tlevel.zone_manager_init_func =&usermap_test_zone_init;\r\n}\r\n\r\nfunction usermap_test_zone_init()\r\n{\r\n\tlevel flag::init( \"always_on\" );\r\n\tzm_zonemgr::add_adjacent_zone(\"yard_zone\",\"house_zone\", \"always_on\");\r\n}\r\n"

// map_edit's answer says which add_adjacent_zone calls the map's script
// lacks, for the function its main makes the zone init, and notes a room in
// no zone.
func TestMapEditListsTheZoneLinksTheScriptLacks(t *testing.T) {
	_, mapPath := zonesInstall(t, zoneGSC)
	b, err := os.ReadFile("../level/testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(44, mapPath)
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	onMachine(t, f)
	ops, err := decodeOps([]byte(`[{"layout": {"material": "brick", "rooms": [
		{"name": "house", "min": [0, 0], "max": [256, 256], "zone": "house_zone"},
		{"name": "yard", "min": [0, -256], "max": [256, 0], "outdoor": true, "zone": "yard_zone"},
		{"name": "garden", "min": [256, -256], "max": [512, 0], "outdoor": true, "zone": "garden_zone"},
		{"name": "shed", "min": [256, 0], "max": [384, 128]}],
		"openings": [{"room": "house", "to": "yard"}, {"room": "yard", "to": "garden"}]}}]`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := mapEdit(context.Background(), ops, nil, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	s := out.Script
	if s == nil || s.File != "usermaps/zm_x/scripts/zm/zm_x.gsc" || s.Function != "usermap_test_zone_init" || s.Present != 1 || s.Note != "" ||
		strings.Join(s.Add, "|") != `zm_zonemgr::add_adjacent_zone( "yard_zone", "garden_zone", "always_on" );` {
		t.Errorf("script %+v", s)
	}
	if strings.Join(out.Notes, "|") != "rooms shed are in no zone: a player there is killed within 3 seconds" {
		t.Errorf("notes %q", out.Notes)
	}
	parts := map[string]int{}
	for _, m := range out.Made {
		parts[m.Part]++
	}
	if parts["house_zone volume"] != 1 || parts["garden_zone spawner"] != 4 || parts["yard_zone pathnode"] != 1 {
		t.Errorf("parts %v", parts)
	}
}

func TestZoneScriptSaysWhatItCantFind(t *testing.T) {
	links := []level.Link{{A: "a_zone", B: "b_zone", Flag: "enter_b", OneWay: true}}
	_, mapPath := zonesInstall(t, "-")
	if s := zoneScript(mapPath, links); !strings.HasPrefix(s.Note, "no script at usermaps/zm_x/scripts/zm/zm_x.gsc") || len(s.Add) != 1 ||
		s.Add[0] != `zm_zonemgr::add_adjacent_zone( "a_zone", "b_zone", "enter_b", true );` {
		t.Errorf("no script: %+v", s)
	}
	_, mapPath = zonesInstall(t, "function main()\n{\n}\n")
	if s := zoneScript(mapPath, links); !strings.HasPrefix(s.Note, "the script sets no level.zone_manager_init_func") || s.Function != "" {
		t.Errorf("no zone init: %+v", s)
	}
	// a one-way link is the other zone's only in its own order
	_, mapPath = zonesInstall(t, zoneGSC+`add_adjacent_zone("b_zone", "a_zone", "enter_b");`)
	if s := zoneScript(mapPath, links); s.Present != 0 || len(s.Add) != 1 {
		t.Errorf("one way, written the other way round: %+v", s)
	}
	links[0].OneWay = false
	if s := zoneScript(mapPath, links); s.Present != 1 || len(s.Add) != 0 {
		t.Errorf("two way: %+v", s)
	}
}
