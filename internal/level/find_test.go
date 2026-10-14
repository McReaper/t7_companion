package level

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

func classes(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Entity.Class()+" "+h.Entity.Guid()[1:9])
	}
	return out
}

func TestFindFilters(t *testing.T) {
	m := testMap(t)
	ids := NewIDs(GUIDs(m))
	sel := ParseSelection("{6E73936C-C58E-11F1-B542-00155DD185EA} {6E73936D-C58E-11F1-B542-00155DD185EA} 0 0 0 1 1 1\n\nbad\n")
	near := mapfile.Vec3{150, 120, 30}
	for name, tc := range map[string]struct {
		q    Query
		want []string
	}{
		"everything":      {Query{}, []string{"worldspawn 8D452111", "reflection_probe 5EC8DE46", "volume_sun 6E73936D", "reflection_probe 76780CEA", "reflection_probe F2D51A52"}},
		"class":           {Query{Class: "Reflection_Probe"}, []string{"reflection_probe 5EC8DE46", "reflection_probe 76780CEA", "reflection_probe F2D51A52"}},
		"class glob":      {Query{Class: "*sun"}, []string{"volume_sun 6E73936D"}},
		"key":             {Query{Key: "name"}, []string{"reflection_probe 5EC8DE46", "reflection_probe 76780CEA", "reflection_probe F2D51A52"}},
		"key=value":       {Query{Key: " name = probe "}, []string{"reflection_probe 5EC8DE46"}},
		"key=glob":        {Query{Key: "name=*t7kb"}, []string{"reflection_probe F2D51A52"}},
		"near":            {Query{Near: &near, Radius: 120}, []string{"worldspawn 8D452111", "reflection_probe 5EC8DE46", "volume_sun 6E73936D", "reflection_probe F2D51A52"}},
		"near, exactly":   {Query{Near: &near, Radius: 0}, []string{"worldspawn 8D452111", "volume_sun 6E73936D", "reflection_probe F2D51A52"}},
		"selected":        {Query{Selected: true}, []string{"volume_sun 6E73936D"}},
		"an entity by id": {Query{IDs: []string{"76780cea"}}, []string{"reflection_probe 76780CEA"}},
		"a brush by id":   {Query{IDs: []string{"af3c9c64"}}, []string{"worldspawn 8D452111"}},
		"ids and class":   {Query{IDs: []string{"76780cea", "6e73936d"}, Class: "volume_sun"}, []string{"volume_sun 6E73936D"}},
	} {
		hits, err := Find(m, ids, tc.q, sel)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := classes(hits); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q\nwant %q", name, got, tc.want)
		}
	}
	hits, _ := Find(m, ids, Query{IDs: []string{"af3c9c64"}}, sel)
	if len(hits[0].Named) != 1 || hits[0].Named[0].Guid() != "{AF3C9C64-C5B3-11F1-B542-00155DD185EA}" || hits[0].Box.Max[2] != 200 {
		t.Errorf("a brush by id: %+v", hits[0])
	}
	if _, err := Find(m, ids, Query{IDs: []string{"5ec8de4"}}, sel); err == nil {
		t.Error("an ambiguous id finds something")
	}
	if !sel.Brush("{6e73936c-c58e-11f1-b542-00155dd185ea}") || sel.Brush("{5EC8DE49-C58E-11F1-B542-00155DD185EA}") || len(sel.Brushes) != 1 {
		t.Errorf("selection = %+v", sel)
	}
}

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, s string
		want       bool
	}{
		{"light", "light", true}, {"light", "lights", false}, {"LIGHT", "light", true},
		{"light*", "light_spot", true}, {"*spot", "light_spot", true}, {"l*t*t", "light_spot", true},
		{"ab*b", "ab", false}, {"a*a", "a", false}, {"*", "", true}, {"s*_*e", "script_model", false},
	} {
		if got := glob(tc.pattern, tc.s); got != tc.want {
			t.Errorf("glob(%q, %q) = %v", tc.pattern, tc.s, got)
		}
	}
}

func TestMaterialKindAndCounts(t *testing.T) {
	m := testMap(t)
	world := m.Entities[0].Brushes()
	if Material(world[0]) != "aac_wood_mahogany_01" || Material(world[1]) != "aac_wood_mahogany_01" {
		t.Errorf("materials %q, %q", Material(world[0]), Material(world[1]))
	}
	mixed := brushOf(t,
		strings.Replace(face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}), "caulk", "clip", 1),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}),
		face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{0, 1, 0}, mapfile.Vec3{0, 0, 1}))
	if Material(mixed) != "caulk" {
		t.Errorf("the material most faces use: %q", Material(mixed))
	}
	tie := brushOf(t,
		strings.Replace(face(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1, 0, 0}, mapfile.Vec3{0, 1, 0}), "caulk", "clip", 1),
		face(mapfile.Vec3{0, 0, 64}, mapfile.Vec3{0, 1, 64}, mapfile.Vec3{1, 0, 64}))
	if Material(tie) != "clip" {
		t.Errorf("a tie goes to the first: %q", Material(tie))
	}
	for k, want := range map[mapfile.Kind]string{mapfile.KindFaces: "brush", mapfile.KindCurve: "curve", mapfile.KindMesh: "mesh", mapfile.KindEmpty: "empty"} {
		if KindName(k) != want {
			t.Errorf("KindName(%d) = %q", k, KindName(k))
		}
	}
	if e, b := Counts(m); e != 5 || b != 3 {
		t.Errorf("counts %d entities, %d brushes", e, b)
	}
	if g := GUIDs(m); len(g) != 8 || g[1] != "{5EC8DE49-C58E-11F1-B542-00155DD185EA}" {
		t.Errorf("guids = %q", g)
	}
}

func TestReadTakesASnapshotOfTheOpenMap(t *testing.T) {
	b, err := os.ReadFile("testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(3, `C:\m\test_claude.map`)
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	f.Selection = []string{"{5EC8DE49-C58E-11F1-B542-00155DD185EA}"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := radiant.NewClient(ctx, f.Dial())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snap, err := Read(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if snap.State.Map != `C:\m\test_claude.map` || len(snap.Map.Entities) != 5 || snap.IDs.Short("{AF3C9C64-C5B3-11F1-B542-00155DD185EA}") != "af3c9c64" ||
		!snap.Sel.Brush("{5EC8DE49-C58E-11F1-B542-00155DD185EA}") || !snap.Sel.Entities["8d452111c59211f1b54200155dd185ea"] {
		t.Errorf("snapshot = %+v", snap)
	}
	f.Update(func(f *radianttest.Fake) { f.Dialog = "Save As" })
	if _, err := Read(ctx, c); err == nil || !strings.Contains(err.Error(), `the dialog "Save As" is open`) {
		t.Errorf("under a dialog: %v", err)
	}
	f.Update(func(f *radianttest.Fake) { f.Dialog, f.Text = "", "{\n" })
	if _, err := Read(ctx, c); err == nil || !strings.Contains(err.Error(), "the map text Radiant gave") {
		t.Errorf("text that doesn't parse: %v", err)
	}
}
