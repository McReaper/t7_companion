package level

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// ops reads ops as map_edit's JSON gives them.
func ops(t *testing.T, s string) []Op {
	t.Helper()
	var out []Op
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func made(d *Done, op int) Made {
	for _, m := range d.Made {
		if m.Op == op {
			return m
		}
	}
	return Made{Op: -1}
}

// The batch the live run sent: boxes, entities, a moved world brush, a
// renamed probe, a retextured curve, a delete.
func TestABatchIsOneEditAndItsResultsAreReadBack(t *testing.T) {
	f, c, snap := viewing(t, 201)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"box": {"min": [-512, -512, -80], "max": [-256, -256, -64], "material": "aac_wood_mahogany_01"}},
		{"box": {"min": [0, 0, 0], "max": [64, 64, 64], "material": "trigger", "contents": "nonColliding", "class": "trigger_multiple", "keys": {"targetname": "t1"}}},
		{"entity": {"class": "light", "origin": [-384, -384, 32], "keys": {"radius": "256"}}},
		{"entity": {"class": "info_player_start", "origin": [-384, -384, -64], "angles": [0, 90, 0]}},
		{"move": {"ids": ["5ec8de49"], "by": [0, 0, 8]}},
		{"set": {"id": "5ec8de46", "keys": {"name": "probe_renamed", "box": null}}},
		{"material": {"ids": ["af3c9c64"], "material": "caulk"}},
		{"delete": ["76780cea"]}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := sent(f, "edit"); len(got) != 1 || got[0] != "edit t7kb: 2 box, 2 entity, 1 delete, 1 set, 1 move, 1 material" {
		t.Errorf("edits = %q", got)
	}
	if d.Label != "t7kb: 2 box, 2 entity, 1 delete, 1 set, 1 move, 1 material" || d.Deleted != 4 || len(d.Made) != 7 {
		t.Errorf("done = %+v", d)
	}
	for i, m := range d.Made {
		if m.ID == "" || (i > 0 && d.Made[i-1].Op > m.Op) {
			t.Errorf("made %+v: every op found back, in op order", m)
		}
	}
	if m := made(d, 4); m.Was != "{5EC8DE49-C58E-11F1-B542-00155DD185EA}" || m.Box != box(mapfile.Vec3{-216, -232, -56}, mapfile.Vec3{248, 256, -8}) {
		t.Errorf("the moved floor: %+v", m)
	}
	if m := made(d, 5); m.Class != "reflection_probe" || m.Was == "" {
		t.Errorf("a probe is rebuilt, not replaced: %+v", m)
	}
	if m := made(d, 1); m.Class != "trigger_multiple" || m.Box != box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}) {
		t.Errorf("the trigger: %+v", m)
	}
	after := d.After
	probe, _ := Find(after.Map, after.IDs, Query{Class: "reflection_probe", Key: "name=probe_renamed"}, Selection{})
	if len(probe) != 1 {
		t.Fatalf("renamed probe: %v", probe)
	}
	if _, has := probe[0].Entity.Key("box"); has {
		t.Error("a null value removes the key")
	}
	world := after.Map.Entities[0]
	var curve *mapfile.Brush
	for _, b := range world.Brushes() {
		if b.Kind() == mapfile.KindCurve {
			curve = b
		}
	}
	if curve == nil || Material(curve) != "caulk" {
		t.Errorf("the curve's material: %v", curve)
	}
	if _, err := after.IDs.Resolve("76780cea"); err == nil {
		t.Error("the deleted probe is still there")
	}
}

func TestAPointEntityIsReplacedInPlaceAndKeepsItsID(t *testing.T) {
	f, c, _ := viewing(t, 202)
	f.Update(func(f *radianttest.Fake) {
		f.Text += "{\nguid \"{0A000000-0000-0000-0000-000000000001}\"\n\"classname\" \"info_null\"\n\"origin\" \"8 8 8\"\n}\n"
	})
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"set": {"id": "0a000000", "keys": {"targetname": "spot"}}},
		{"move": {"ids": ["0a000000"], "by": [1, 2, 3]}}
	]`), "move the spot", nil)
	if err != nil {
		t.Fatal(err)
	}
	body := f.Requests()[len(f.Requests())-4] // edit, then the read back's state, entities, selection
	if !strings.HasPrefix(body, "edit t7kb: move the spot") || len(d.Made) != 1 {
		t.Fatalf("requests %q, made %+v", f.Requests(), d.Made)
	}
	m := d.Made[0]
	if m.ID != "{0A000000-0000-0000-0000-000000000001}" || m.Was != "" || m.Box != box(mapfile.Vec3{9, 10, 11}, mapfile.Vec3{9, 10, 11}) {
		t.Errorf("made %+v", m)
	}
	hits, _ := Find(d.After.Map, d.After.IDs, Query{IDs: []string{"0a000000"}}, Selection{})
	if v, _ := hits[0].Entity.Key("targetname"); len(hits) != 1 || v != "spot" {
		t.Errorf("after: %+v", hits)
	}
}

func TestOpsOnABrushEntityRebuildIt(t *testing.T) {
	_, c, snap := viewing(t, 203)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"move": {"ids": ["6e73936d"], "by": [10, 0, 0]}},
		{"material": {"ids": ["6e73936c"], "material": "clip"}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := d.Made[0]
	if len(d.Made) != 1 || m.Op != 1 || m.Class != "volume_sun" || m.Was != "{6E73936D-C58E-11F1-B542-00155DD185EA}" ||
		m.Box != box(mapfile.Vec3{-262, -264, -144}, mapfile.Vec3{306, 320, 176}) {
		t.Errorf("made %+v", d.Made)
	}
	hits, _ := Find(d.After.Map, d.After.IDs, Query{Class: "volume_sun"}, Selection{})
	if len(hits) != 1 || Material(hits[0].Entity.Brushes()[0]) != "clip" {
		t.Errorf("the volume after: %+v", hits)
	}
	snap = d.After
	d, err = Edit(context.Background(), c, snap, ops(t, `[{"delete": ["`+snap.IDs.Short(hits[0].Entity.Brushes()[0].Guid())+`"]}]`), "", nil)
	if err != nil || len(d.Made) != 1 || d.Made[0].Class != "volume_sun" || d.Made[0].Box.Ok {
		t.Errorf("deleting an entity's one brush rebuilds it without: %+v, %v", d, err)
	}
}

func TestEditRefusals(t *testing.T) {
	_, c, snap := viewing(t, 204)
	for js, want := range map[string]string{
		`[]`: "no ops",
		`[{"box": {"min": [0,0,0], "max": [1,1,1], "material": "x"}, "delete": ["5ec8de46"]}]`: "op 0: an op is exactly one of",
		`[{}]`: "this one has 0",
		`[{"box": {"min": [0,0,0], "max": [0,1,1], "material": "x"}}]`:                                      "max [0 1 1] must exceed min",
		`[{"box": {"min": [0,0,0], "max": [1,1,1], "material": "two words"}}]`:                              `material "two words"`,
		`[{"box": {"min": [0,0,0], "max": [1,1,1], "material": "x", "keys": {"a": "b"}}}]`:                  "keys belong to a brush entity",
		`[{"box": {"min": [0,0,0], "max": [1,1,1], "material": "x", "contents": "a\"b"}}]`:                  "contents",
		`[{"box": {"min": [0,0,0], "max": [1,1,1], "material": "x", "class": "t", "keys": {"guid": "x"}}}]`: `key "guid"`,
		`[{"entity": {"class": "", "origin": [0,0,0]}}]`:                                                    `class ""`,
		`[{"entity": {"class": "light", "origin": [0,0,0], "keys": {"a": "b\"c"}}}]`:                        `key "a"`,
		`[{"set": {"id": "5ec8de46", "keys": {}}}]`:                                                         "set: no keys",
		`[{"set": {"id": "5ec8de46", "keys": {"classname": "light"}}}]`:                                     `"classname" cannot be set`,
		`[{"set": {"id": "5ec8de46", "keys": {"a": "b\nc"}}}]`:                                              "double quote or a line break",
		`[{"set": {"id": "5ec8de49", "keys": {"a": "b"}}}]`:                                                 "is a world brush",
		`[{"set": {"id": "8d452111", "keys": {"a": "b"}}}]`:                                                 "is worldspawn",
		`[{"delete": []}]`: "delete: no ids",
		`[{"delete": ["76780cea"]}, {"move": {"ids": ["76780cea"], "by": [1,0,0]}}]`: "deleted by an earlier op",
		`[{"move": {"ids": ["00000001"], "by": [1,0,0]}}]`:                           "no entity or brush 00000001",
		`[{"move": {"ids": [], "by": [1,0,0]}}]`:                                     "no ids",
		`[{"material": {"ids": ["5ec8de46"], "material": "caulk"}}]`:                 "has no brush to give a material",
		`[{"material": {"ids": ["5ec8de49"], "material": ""}}]`:                      `material ""`,
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", js, err, want)
		}
	}
}

func TestEditPassesOnTheBridgesAnswer(t *testing.T) {
	f, c, snap := viewing(t, 205)
	f.Handle("edit", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.Err("busy", "Entity Info")
	})
	_, err := Edit(context.Background(), c, snap, ops(t, `[{"delete": ["76780cea"]}]`), "", nil)
	if err == nil || !strings.Contains(err.Error(), "busy: Entity Info") {
		t.Errorf("busy: %v", err)
	}
	f.Handle("edit", func(f *radianttest.Fake, _ string, _ []byte) radianttest.Reply {
		f.Dialog = "Save As"
		return radianttest.OK("deleted=1", nil)
	})
	_, err = Edit(context.Background(), c, snap, ops(t, `[{"delete": ["76780cea"]}]`), "", nil)
	if err == nil || !strings.Contains(err.Error(), "the edit is done (one Ctrl+Z undoes it), but the map could not be read back") {
		t.Errorf("no read back: %v", err)
	}
}

func TestWhatMapEditWrites(t *testing.T) {
	b := BoxBrush(box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 32, 16}), "caulk", "detail;")
	m, err := mapfile.Parse([]byte("{\n\"classname\" \"worldspawn\"\n" + b + "}\n"))
	if err != nil {
		t.Fatal(err)
	}
	br := m.Entities[0].Brushes()[0]
	if geom.BrushBox(br) != box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 32, 16}) || br.Contents() != "detail" || Material(br) != "caulk" {
		t.Errorf("box brush:\n%s", b)
	}
	if got := EntityText("light", map[string]string{"z": "1", "a": "2"}); got != "{\n\"classname\" \"light\"\n\"a\" \"2\"\n\"z\" \"1\"\n}\n" {
		t.Errorf("entity text %q", got)
	}
	e, _ := mapfile.Parse([]byte("{\r\nguid \"{G}\"\r\n\"classname\" \"x\"\r\n  \"a\" \"1\"\r\n{\r\n ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\r\n}\r\n}\r\n"))
	one, two := "9", "2"
	setKeys(e.Entities[0], map[string]*string{"a": &one, "b": &two})
	setKeys(e.Entities[0], map[string]*string{"a": nil})
	if got := string(e.Bytes()); !strings.Contains(got, "\"classname\" \"x\"\r\n\"b\" \"2\"\n{\r\n") || strings.Contains(got, "\"a\"") {
		t.Errorf("keys set and removed:\n%q", got)
	}
	setKeys(e.Entities[0], map[string]*string{"b": &one})
	if got := string(e.Bytes()); !strings.Contains(got, "\"b\" \"9\"\n") {
		t.Errorf("a key replaced: %q", got)
	}
	if keyName(`"origin" "1 2 3"`) != "origin" || keyName(`guid "{G}"`) != "" || keyName(`"broken`) != "" {
		t.Error("keyName")
	}
}

func TestTranslateAndRetextureKeepEachLinesShape(t *testing.T) {
	m := testMap(t)
	floor, curve := m.Entities[0].Brushes()[0], m.Entities[0].Brushes()[1]
	if err := translate(floor, mapfile.Vec3{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if geom.BrushBox(floor) != box(mapfile.Vec3{-215, -230, -61}, mapfile.Vec3{249, 258, -13}) || !strings.HasPrefix(floor.Body[1], " ( 249 258 -61 )") {
		t.Errorf("floor %+v\n%q", geom.BrushBox(floor), floor.Body[1])
	}
	if err := translate(curve, mapfile.Vec3{0, 0, 100}); err != nil {
		t.Fatal(err)
	}
	if geom.BrushBox(curve).Max[2] != 300 || !strings.Contains(strings.Join(curve.Body, ""), "\tv -96 -64 84 t 0 0 0 0\n") {
		t.Errorf("curve:\n%s", strings.Join(curve.Body, ""))
	}
	if err := retexture(curve, "caulk"); err != nil {
		t.Fatal(err)
	}
	if Material(curve) != "caulk" || !strings.Contains(strings.Join(curve.Body, ""), "  toolFlags;\n   caulk\n   lightmap_gray\n") {
		t.Errorf("curve's material:\n%s", strings.Join(curve.Body, ""))
	}
	if err := retexture(floor, "clip"); err != nil || Material(floor) != "clip" {
		t.Errorf("floor's material %q, %v", Material(floor), err)
	}
	bad, _ := mapfile.Parse([]byte("{\n{\n ( 0 0 0 ) short\n}\n}\n"))
	if err := translate(bad.Entities[0].Brushes()[0], mapfile.Vec3{}); err == nil {
		t.Error("a short face line")
	}
	nan, _ := mapfile.Parse([]byte("{\n{\n ( x 0 0 ) ( 1 0 0 ) ( 0 1 0 ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n}\n}\n"))
	if err := translate(nan.Entities[0].Brushes()[0], mapfile.Vec3{}); err == nil || !strings.Contains(err.Error(), `"x" is no number`) {
		t.Errorf("not a number: %v", err)
	}
}
