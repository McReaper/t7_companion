package level

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

func TestAnOutlineIsCleanedAndWoundCounterClockwise(t *testing.T) {
	poly, err := outlineOf([][2]float64{{0, 0}, {0, 100}, {0, 100}, {50, 100}, {100, 100}, {100, 0}, {0, 0}})
	if err != nil || fmt.Sprint(poly) != "[{100 0} {100 100} {0 100} {0 0}]" {
		t.Errorf("clockwise, a corner twice, one on a line, the first again at the end: %v, %v", poly, err)
	}
	for want, pts := range map[string][][2]float64{
		"three corners or more":   {{0, 0}, {100, 0}, {200, 0}},
		"crosses itself: edge 0":  {{0, 0}, {100, 100}, {100, 0}, {0, 100}},
		"three corners or more, ": {{0, 0}, {0, 0}},
	} {
		if _, err := outlineOf(pts); err == nil || !strings.Contains(err.Error(), strings.TrimSuffix(want, ", ")) {
			t.Errorf("%v: %v", pts, err)
		}
	}
}

func TestAnOutlineSplitsIntoFewConvexParts(t *testing.T) {
	area := func(ps [][]pt2) float64 {
		var a float64
		for _, p := range ps {
			a += signedArea(p)
		}
		return a
	}
	for name, c := range map[string]struct {
		pts   [][2]float64
		parts int
		area  float64
	}{
		"a square":               {[][2]float64{{0, 0}, {100, 0}, {100, 100}, {0, 100}}, 1, 10000},
		"a hexagon, at an angle": {[][2]float64{{50, 0}, {100, 25}, {100, 75}, {50, 100}, {0, 75}, {0, 25}}, 1, 7500},
		"an L":                   {[][2]float64{{0, 0}, {200, 0}, {200, 100}, {100, 100}, {100, 200}, {0, 200}}, 2, 30000},
		"a U":                    {[][2]float64{{0, 0}, {300, 0}, {300, 200}, {200, 200}, {200, 100}, {100, 100}, {100, 200}, {0, 200}}, 3, 50000},
		"a bent corridor":        {[][2]float64{{0, 0}, {256, 0}, {400, 144}, {400, 400}, {272, 400}, {272, 196}, {204, 128}, {0, 128}}, 4, 0}, // two reflex corners: 3 at best, merging finds 4
	} {
		poly, err := outlineOf(c.pts)
		if err != nil {
			t.Fatal(name, err)
		}
		parts, err := convexParts(poly)
		if err != nil || len(parts) > c.parts {
			t.Errorf("%s: %d parts, at most %d: %v", name, len(parts), c.parts, err)
		}
		for _, p := range parts {
			if !convex(p) || signedArea(p) <= 0 {
				t.Errorf("%s: a part not convex and counter-clockwise: %v", name, p)
			}
		}
		if want := signedArea(poly); math.Abs(area(parts)-want) > 1e-6 || (c.area > 0 && math.Abs(want-c.area) > 1e-6) {
			t.Errorf("%s: parts cover %v, the outline %v", name, area(parts), want)
		}
	}
}

// A prism brush reads back as a brush whose corners are the outline's at
// both heights: its faces point the way Radiant's do.
func TestAPrismBrushStandsOnItsOutline(t *testing.T) {
	hex := []pt2{{50, 0}, {100, 25}, {100, 75}, {50, 100}, {0, 75}, {0, 25}}
	m, err := mapfile.Parse([]byte(EntityText("info_volume", nil, prismBrush(hex, -8, 120, "volume", texNumbers(64, 64)))))
	if err != nil {
		t.Fatal(err)
	}
	br := m.Entities[0].Brushes()[0]
	faces, _ := br.Faces()
	planes := make([]plane, len(faces))
	for i, f := range faces {
		planes[i] = planeOf(f.Points)
	}
	vs := vertices(planes)
	bx := BrushBox(br)
	if len(faces) != 8 || len(vs) != 12 || bx.Distance(mapfile.Vec3{0, 0, -8}) > 1e-9 || bx.Distance(mapfile.Vec3{100, 100, 120}) > 1e-9 || bx.Distance(mapfile.Vec3{-0.001, 50, 0}) == 0 {
		t.Fatalf("%d faces, %d corners, bounds %+v", len(faces), len(vs), BrushBox(br))
	}
	if !inside(mapfile.Vec3{50, 50, 60}, planes) || inside(mapfile.Vec3{5, 5, 60}, planes) {
		t.Error("the middle is inside, a cut corner outside")
	}
}

// A shape is a solid on any outline: an L gives two world brushes, as a
// class gives one entity of both; a zone follows a hexagonal floor's own
// outline.
func TestAShapeStandsOnItsOutline(t *testing.T) {
	c, snap := zoning(t, 801)
	d, err := Edit(context.Background(), c, snap, ops(t, `[
		{"shape": {"outline": [[0, 0], [200, 0], [200, 100], [100, 100], [100, 200], [0, 200]], "z": [0, 112], "material": "brick"}},
		{"shape": {"outline": [[300, 0], [400, 0], [350, 80]], "z": [0, 64], "material": "clip", "class": "script_brushmodel", "keys": {"targetname": "rock"}}},
		{"shape": {"outline": [[650, 500], [700, 587], [650, 674], [550, 674], [500, 587], [550, 500]], "z": [-16, 0], "material": "wood"}},
		{"zone": {"name": "terrace_zone", "areas": [{"floors": ["@2"], "height": 100}], "spawners": 2}}
	]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var world, ents []Made
	for _, m := range d.Made {
		switch m.Op {
		case 0:
			world = append(world, m)
		case 1:
			ents = append(ents, m)
		}
	}
	if len(world) != 2 || world[0].Class != "" || len(ents) != 1 || ents[0].Class != "script_brushmodel" {
		t.Fatalf("made %+v %+v", world, ents)
	}
	var lo, hi mapfile.Vec3
	for i, m := range world {
		if i == 0 {
			lo, hi = m.Box.Min, m.Box.Max
		}
		for k := range 3 {
			lo[k], hi[k] = math.Min(lo[k], m.Box.Min[k]), math.Max(hi[k], m.Box.Max[k])
		}
	}
	if lo != (mapfile.Vec3{0, 0, 0}) || hi != (mapfile.Vec3{200, 200, 112}) {
		t.Errorf("the L's two brushes span %v to %v", lo, hi)
	}
	parts := partsOf(t, d)
	vol := parts["terrace_zone volume"]
	if len(vol) != 1 || len(parts["terrace_zone spawner"]) != 2 {
		t.Fatalf("parts %v", parts)
	}
	faces, _ := vol[0].Brushes()[0].Faces()
	if bx := EntityBox(vol[0]); len(faces) != 8 || math.Abs(bx.Min[2]) > 1e-6 || math.Abs(bx.Max[2]-100) > 1e-6 || bx.Min[0] < 500-1e-6 || bx.Max[0] > 700+1e-6 {
		t.Errorf("the zone over the hexagon: %d faces, bounds %+v", len(faces), bx)
	}
	for want, op := range map[string]string{
		"its top above its bottom":      `{"shape": {"outline": [[0,0],[1,0],[0,1]], "z": [10, 10], "material": "brick"}}`,
		`material "a b"`:                `{"shape": {"outline": [[0,0],[1,0],[0,1]], "z": [0, 10], "material": "a b"}}`,
		"keys belong to a brush entity": `{"shape": {"outline": [[0,0],[1,0],[0,1]], "z": [0, 10], "material": "brick", "keys": {"a": "b"}}}`,
		"shape: the outline crosses":    `{"shape": {"outline": [[0,0],[100,100],[100,0],[0,100]], "z": [0, 10], "material": "brick"}}`,
		`class "a b"`:                   `{"shape": {"outline": [[0,0],[1,0],[0,1]], "z": [0, 10], "material": "brick", "class": "a b"}}`,
	} {
		if _, err := Edit(context.Background(), c, snap, ops(t, "["+op+"]"), "", nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
}

// Where a shape's pieces meet, their faces are caulk, and only those: one on
// each piece, a world shape's or an entity's.
func TestAShapesPiecesCaulkWhereTheyMeet(t *testing.T) {
	c, snap := zoning(t, 802)
	for _, class := range []string{``, `, "class": "script_brushmodel"`} {
		d, err := Edit(context.Background(), c, snap, ops(t, `[{"shape": {"outline": [[0, 0], [200, 0], [200, 100], [100, 100], [100, 200], [0, 200]], "z": [0, 112], "material": "brick"`+class+`}}]`), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		var pieces []*mapfile.Brush
		for _, e := range d.After.Map.Entities {
			for _, br := range e.Brushes() {
				for _, m := range d.Made {
					if hexOf(br.Guid()) == hexOf(m.ID) || hexOf(e.Guid()) == hexOf(m.ID) {
						pieces = append(pieces, br)
					}
				}
			}
		}
		hidden := selfHiddenFaces(pieces)
		caulk := 0
		for k, br := range pieces {
			faces, err := br.Faces()
			if err != nil {
				t.Fatal(err)
			}
			for i, f := range faces {
				if f.Material == "caulk" {
					caulk++
					if !slices.Contains(hidden[k], i) {
						t.Errorf("%q: piece %d's face %d is caulk, and shows", class, k, i)
					}
				}
			}
		}
		if len(pieces) != 2 || caulk != 2 {
			t.Errorf("%q: %d pieces, %d faces caulk, want the 2 where the L's pieces meet", class, len(pieces), caulk)
		}
	}
}

// Outlines away from the origin: a corner given twice and the first given
// again at the end are dropped, and a corner a little off the line between
// its neighbours stays.
func TestOutlinesAwayFromTheOrigin(t *testing.T) {
	poly, err := outlineOf([][2]float64{{1000, 1000}, {1100, 1000}, {1100, 1100}, {1100, 1100}, {1000, 1100}, {1000, 1000}})
	if err != nil || fmt.Sprint(poly) != "[{1000 1000} {1100 1000} {1100 1100} {1000 1100}]" {
		t.Errorf("a corner twice, the first again: %v, %v", poly, err)
	}
	poly, err = outlineOf([][2]float64{{5000, 5000}, {5100, 5000.5}, {5200, 5000}, {5200, 5100}, {5000, 5100}})
	if err != nil || len(poly) != 5 {
		t.Errorf("half a unit off a line 200 long is a corner: %v, %v", poly, err)
	}
}
