package level

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// world is a snapshot of worldspawn holding the brushes, each given the GUID
// {0D0000nn-…} by its index, and the entities after it.
func world(t *testing.T, brushes []string, entities ...string) *Snapshot {
	t.Helper()
	var body []string
	for i, b := range brushes {
		body = append(body, strings.Replace(b, "{\n", fmt.Sprintf("{\n guid \"{0D0000%02X-0000-0000-0000-000000000000}\"\n", i), 1))
	}
	m, err := mapfile.Parse([]byte(EntityText("worldspawn", nil, body...) + strings.Join(entities, "")))
	if err != nil {
		t.Fatal(err)
	}
	return &Snapshot{Map: m, IDs: NewIDs(GUIDs(m))}
}

// slab is a box brush of a material, its faces at tex ("W H u v rot 0").
func slab(lo, hi mapfile.Vec3, material, tex string) string {
	return boxBrush(box(lo, hi), material, "", tex+" lightmap_gray 16384 16384 0 0 0 0")
}

var checkInfo = map[string]MaterialInfo{
	"brick": {Known: true, Opaque: true, Drawn: true, Tile: Tile{W: 64, H: 64}},
	"wood":  {Known: true, Opaque: true, Drawn: true, Tile: Tile{W: 128, H: 32}},
	"caulk": {Known: true, Opaque: true},
	"glass": {Known: true, Drawn: true, Tile: Tile{W: 64, H: 64}},
	"clip":  {Known: true},
}

func check(t *testing.T, snap *Snapshot, ids ...string) []string {
	t.Helper()
	found, err := CheckTextures(snap, ids, func(m string) MaterialInfo { return checkInfo[m] })
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range found {
		out = append(out, f.Kind+" "+f.ID+" "+f.Face+": "+f.Detail)
	}
	sort.Strings(out)
	return out
}

const plain = "64 64 0 0 0 0"

func TestAFaceAgainstAnOpaqueBrushIsHidden(t *testing.T) {
	floor := slab(mapfile.Vec3{0, 0, -16}, mapfile.Vec3{128, 128, 0}, "brick", plain)
	wall := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 128, 112}, "brick", plain)
	got := check(t, world(t, []string{floor, wall}))
	if len(got) != 1 || got[0] != "hidden 0d000001 -z: against 0d000000 +z" {
		t.Errorf("findings %q: the wall's bottom alone, the floor's top shows around it", got)
	}
	// Glass is seen through, clip too; a caulked bottom draws nothing to hide.
	for _, under := range []string{"glass", "clip"} {
		if got := check(t, world(t, []string{slab(mapfile.Vec3{0, 0, -16}, mapfile.Vec3{128, 128, 0}, under, plain), wall})); len(got) != 0 {
			t.Errorf("on %s: %q", under, got)
		}
	}
	if got := check(t, world(t, []string{floor, slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 128, 112}, "caulk", plain)})); len(got) != 0 {
		t.Errorf("a caulk wall: %q", got)
	}
	// A wall wider than the floor under it shows its bottom past the edge.
	if got := check(t, world(t, []string{floor, slab(mapfile.Vec3{-8, 0, 0}, mapfile.Vec3{8, 128, 112}, "brick", plain)})); len(got) != 0 {
		t.Errorf("a wall over the edge: %q", got)
	}
}

func TestABrushBuriedInAnotherIsHiddenWhole(t *testing.T) {
	big := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{256, 256, 256}, "brick", plain)
	small := slab(mapfile.Vec3{64, 64, 64}, mapfile.Vec3{96, 96, 96}, "wood", "128 32 0 0 0 0")
	got := check(t, world(t, []string{big, small}))
	if len(got) != 6 || !strings.HasPrefix(got[0], "hidden 0d000001 ") || !strings.HasSuffix(got[0], ": inside 0d000000") {
		t.Errorf("findings %q: the six faces of the buried one", got)
	}
	// Half out, its other faces show: only the one inside is hidden.
	if got := check(t, world(t, []string{big, slab(mapfile.Vec3{240, 64, 64}, mapfile.Vec3{272, 96, 96}, "wood", "128 32 0 0 0 0")})); len(got) != 1 || got[0] != "hidden 0d000001 -x: inside 0d000000" {
		t.Errorf("half out: %q", got)
	}
	// Inside a brush of glass, it is seen.
	if got := check(t, world(t, []string{slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{256, 256, 256}, "glass", plain), small})); len(got) != 0 {
		t.Errorf("in glass: %q", got)
	}
}

func TestTouchingCoplanarFacesWhoseTilesDontMeetAreASeam(t *testing.T) {
	a := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{128, 8, 112}, "brick", plain)
	for name, c := range map[string]struct {
		b    string
		seam bool
	}{
		"world-aligned":       {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", plain), false},
		"a tile further":      {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 64 -64 360 0"), false},
		"shifted":             {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 16 0 0 0"), true},
		"scaled":              {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "128 128 0 0 0 0"), true},
		"turned":              {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 0 0 90 0"), true},
		"mirrored":            {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "-64 64 0 0 0 0"), true},
		"another material":    {slab(mapfile.Vec3{128, 0, 0}, mapfile.Vec3{256, 8, 112}, "wood", "64 64 16 0 0 0"), false},
		"apart":               {slab(mapfile.Vec3{136, 0, 0}, mapfile.Vec3{256, 8, 112}, "brick", "64 64 16 0 0 0"), false},
		"corner to corner":    {slab(mapfile.Vec3{128, 8, 0}, mapfile.Vec3{256, 16, 112}, "brick", "64 64 16 0 0 0"), false},
		"overlapping, on top": {slab(mapfile.Vec3{64, 0, 112}, mapfile.Vec3{192, 8, 120}, "brick", "64 64 0 8 0 0"), true},
	} {
		got := check(t, world(t, []string{a, c.b}))
		var seams []string
		for _, g := range got {
			if strings.HasPrefix(g, "seam") {
				seams = append(seams, g)
			}
		}
		if c.seam != (len(seams) > 0) {
			t.Errorf("%s: %q", name, got)
		}
		if c.seam && name != "overlapping, on top" && (len(seams) != 4 || seams[0] != "seam 0d000000 +y: 0d000001 +y") {
			t.Errorf("%s: %q: each of the four planes' pair once, from the first brush", name, seams)
		}
	}
}

func TestStretchedTilesAndUnknownMaterialsAndWhatIsChecked(t *testing.T) {
	stretched := slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{64, 64, 64}, "brick", "64 16 0 0 0 0")
	tall := slab(mapfile.Vec3{100, 0, 0}, mapfile.Vec3{164, 64, 64}, "wood", "128 64 0 0 0 0")
	unknown := slab(mapfile.Vec3{200, 0, 0}, mapfile.Vec3{264, 64, 64}, "missing_mat", plain)
	trigger := EntityText("trigger_multiple", nil, slab(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 8, 8}, "missing_mat", plain))
	snap := world(t, []string{stretched, tall, unknown}, trigger)
	got := check(t, snap)
	if len(got) != 18 {
		t.Fatalf("%d findings %q: the 6 faces of each of three brushes, none of the trigger's", len(got), got)
	}
	want := []string{
		"stretched 0d000000 +x: 64x16, the material's 64x64",
		"stretched 0d000001 +x: 128x64, the material's 128x32",
		"unknown 0d000002 +x: ",
	}
	for _, w := range want {
		if !strings.Contains(strings.Join(got, "\n"), w) {
			t.Errorf("no %q in %q", w, got)
		}
	}
	if got := check(t, snap, "0d000001"); len(got) != 6 || !strings.HasPrefix(got[0], "stretched 0d000001") {
		t.Errorf("one brush's findings: %q", got)
	}
	if _, err := CheckTextures(snap, []string{"zz"}, nil); err == nil {
		t.Error("an id that names nothing")
	}
	if s := stretch(&cface{tex: [6]float64{64, 48}}, Tile{W: 64, H: 64}); s != "" {
		t.Errorf("a third off is no stretch: %q", s)
	}
}

func TestHullsCoverAndTouch(t *testing.T) {
	sq := convexHull([]pt2{{0, 0}, {2, 0}, {2, 2}, {0, 2}, {1, 1}, {2, 2}})
	if len(sq) != 4 {
		t.Fatalf("hull %v: the square's four corners, a point inside and a repeat dropped", sq)
	}
	inner := convexHull([]pt2{{0.5, 0.5}, {1, 0.5}, {1, 1}})
	if !covers(sq, inner) || covers(inner, sq) || !covers(sq, sq) {
		t.Error("covers")
	}
	cross := convexHull([]pt2{{-1, 0.5}, {3, 0.5}, {3, 1.5}, {-1, 1.5}})
	far := convexHull([]pt2{{3, 3}, {4, 3}, {4, 4}})
	edge := convexHull([]pt2{{2, 0}, {3, 0}, {2, 2}})
	corner := convexHull([]pt2{{2, 0}, {3, 0}, {3, 2}})
	if !touch(sq, cross) || !touch(sq, inner) || touch(sq, far) || !touch(sq, edge) || touch(sq, corner) || !touch(sq, sq) {
		t.Error("touch: a band across, one inside, one apart, one sharing an edge, one a corner only, itself")
	}
	if convexHull([]pt2{{0, 0}, {1, 1}}) != nil || len(convexHull([]pt2{{0, 0}, {1, 1}, {2, 2}})) > 2 {
		t.Error("fewer than three points, or three in a line, is no hull")
	}
}
