package level

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// wbrush is a brush a walls op made, flattened for the tests: its corners
// on the ground (sorted), its height, its materials and what it is.
type wbrush struct {
	part, contents string
	xy             []geom.P2
	z0, z1         float64
	caulk, faces   int
}

func wallsRun(t *testing.T, pid int, op string) ([]wbrush, error) {
	t.Helper()
	c, snap := zoning(t, pid)
	d, err := Edit(context.Background(), c, snap, ops(t, "["+op+"]"), "", nil)
	if err != nil {
		return nil, err
	}
	var out []wbrush
	for _, m := range d.Made {
		for _, e := range d.After.Map.Entities {
			for _, br := range e.Brushes() {
				if hexOf(br.Guid()) == hexOf(m.ID) {
					out = append(out, flatten(t, m.Part, br))
				}
			}
		}
	}
	return out, nil
}

func flatten(t *testing.T, part string, br *mapfile.Brush) wbrush {
	t.Helper()
	faces, err := br.Faces()
	if err != nil {
		t.Fatal(err)
	}
	planes := make([]geom.Plane, len(faces))
	w := wbrush{part: part, contents: br.Contents(), faces: len(faces), z0: math.Inf(1), z1: math.Inf(-1)}
	for i, f := range faces {
		planes[i] = geom.PlaneOf(f.Points)
		if f.Material == "caulk" {
			w.caulk++
		}
	}
	seen := map[[2]float64]bool{}
	for _, v := range geom.Vertices(planes) {
		k := [2]float64{math.Round(v[0]*100)/100 + 0, math.Round(v[1]*100)/100 + 0} // + 0: no -0
		if !seen[k] {
			seen[k] = true
			w.xy = append(w.xy, geom.P2{X: k[0], Y: k[1]})
		}
		w.z0, w.z1 = math.Min(w.z0, v[2]), math.Max(w.z1, v[2])
	}
	sort.Slice(w.xy, func(i, j int) bool {
		if w.xy[i].X != w.xy[j].X {
			return w.xy[i].X < w.xy[j].X
		}
		return w.xy[i].Y < w.xy[j].Y
	})
	return w
}

func corners(ps ...float64) []geom.P2 {
	var out []geom.P2
	for i := 0; i < len(ps); i += 2 {
		out = append(out, geom.P2{X: ps[i], Y: ps[i+1]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Y < out[j].Y
	})
	return out
}

func ofPart(bs []wbrush, part string) []wbrush {
	var out []wbrush
	for _, b := range bs {
		if b.part == part {
			out = append(out, b)
		}
	}
	return out
}

func xyText(ps []geom.P2) string {
	var s []string
	for _, p := range ps {
		s = append(s, fmt.Sprintf("(%g %g)", p.X, p.Y))
	}
	return strings.Join(s, " ")
}

func wantPieces(t *testing.T, got []wbrush, want ...[]geom.P2) {
	t.Helper()
	var g, w []string
	for _, b := range got {
		g = append(g, xyText(b.xy))
	}
	for _, p := range want {
		w = append(w, xyText(p))
	}
	sort.Strings(g)
	sort.Strings(w)
	if !slices.Equal(g, w) {
		t.Errorf("pieces\n %s\nwant\n %s", strings.Join(g, "\n "), strings.Join(w, "\n "))
	}
}

// Two walls meeting at a corner each end on the mitre: no overlap, one
// face each against the other, caulk.
func TestWallsMitreACorner(t *testing.T) {
	bs, err := wallsRun(t, 901, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}, {"points": [[256, 0], [256, 256]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	ws := ofPart(bs, "wall")
	wantPieces(t, ws, corners(0, -4, 0, 4, 252, 4, 260, -4), corners(252, 4, 260, -4, 260, 256, 252, 256))
	for _, w := range ws {
		if w.caulk != 1 || w.z0 != 0 || w.z1 != 112 {
			t.Errorf("%s: %d faces caulk, z %g to %g; want the mitre caulk, 0 to 112", xyText(w.xy), w.caulk, w.z0, w.z1)
		}
	}
}

// A wall ending on another's side stops on its face; the other runs on whole.
func TestWallsButtATee(t *testing.T) {
	bs, err := wallsRun(t, 902, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}, {"points": [[128, 0], [128, 200]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(0, -4, 0, 4, 256, 4, 256, -4), corners(124, 4, 132, 4, 132, 200, 124, 200))
}

// Walls crossing: the second is cut in two, each piece stopping on the
// first's faces.
func TestWallsSplitACrossing(t *testing.T) {
	bs, err := wallsRun(t, 903, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}, {"points": [[128, -128], [128, 128]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(0, -4, 0, 4, 256, 4, 256, -4),
		corners(124, -128, 132, -128, 132, -4, 124, -4), corners(124, 4, 132, 4, 132, 128, 124, 128))
}

// A door: 72 cut out with its edges on the grid, a lintel over it, jambs
// narrowing it to 56 for the eye (nonColliding) and for players
// (clip_player), a 2-unit trim proud of both faces.
func TestWallsCutADoor(t *testing.T) {
	bs, err := wallsRun(t, 904, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}], "doors": [{"at": [97, 3]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(0, -4, 0, 4, 64, 4, 64, -4), corners(136, -4, 136, 4, 256, 4, 256, -4))
	lintel := ofPart(bs, "door 0 lintel")
	wantPieces(t, lintel, corners(64, -4, 64, 4, 136, 4, 136, -4))
	if len(lintel) == 1 && (lintel[0].z0 != 96 || lintel[0].z1 != 112) {
		t.Errorf("lintel z %g to %g, want 96 to 112", lintel[0].z0, lintel[0].z1)
	}
	jambs, clips := ofPart(bs, "door 0 jamb"), ofPart(bs, "door 0 player clip")
	wantPieces(t, jambs, corners(64, -4, 64, 4, 72, 4, 72, -4), corners(128, -4, 128, 4, 136, 4, 136, -4))
	wantPieces(t, clips, corners(64, -4, 64, 4, 72, 4, 72, -4), corners(128, -4, 128, 4, 136, 4, 136, -4))
	for _, j := range jambs {
		if j.contents != "nonColliding" || j.z1 != 96 {
			t.Errorf("jamb contents %q, top %g", j.contents, j.z1)
		}
	}
	trims := ofPart(bs, "door 0 trim")
	wantPieces(t, trims, corners(72, -6, 72, 6, 74, 6, 74, -6), corners(126, -6, 126, 6, 128, 6, 128, -6), corners(72, -6, 72, 6, 128, 6, 128, -6))
	for _, tr := range trims {
		if tr.contents != "weaponClip detail" {
			t.Errorf("trim contents %q", tr.contents)
		}
	}
}

// A door's width, height and seen width are its own; seen equal to width
// narrows nothing.
func TestWallsDoorSizes(t *testing.T) {
	bs, err := wallsRun(t, 905, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}], "doors": [{"at": [128, 0], "width": 64, "height": 80, "seen": 64}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ofPart(bs, "door 0 jamb")) + len(ofPart(bs, "door 0 player clip")); n != 0 {
		t.Errorf("%d jambs or clips for a door seen as wide as it is", n)
	}
	wantPieces(t, ofPart(bs, "door 0 lintel"), corners(96, -4, 96, 4, 160, 4, 160, -4))
	if l := ofPart(bs, "door 0 lintel"); len(l) == 1 && l[0].z0 != 80 {
		t.Errorf("lintel from %g, want 80", l[0].z0)
	}
}

// Corners snap to the grid; a line a few units off an axis runs on it.
func TestWallsSnapAndStraighten(t *testing.T) {
	bs, err := wallsRun(t, 906, `{"walls": {"material": "brick", "lines": [{"points": [[1, 2], [250, 7]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(0, 4, 0, 12, 248, 12, 248, 4))
	bs, err = wallsRun(t, 907, `{"walls": {"material": "brick", "grid": 0, "lines": [{"points": [[1, 2], [250, 2]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(1, -2, 1, 6, 250, 6, 250, -2))
}

// The two sides of a gap on nearly one line share it.
func TestWallsGapSidesShareALine(t *testing.T) {
	bs, err := wallsRun(t, 908, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [96, 0]]}, {"points": [[176, 6], [304, 6]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	wantPieces(t, ofPart(bs, "wall"), corners(0, -4, 0, 4, 96, 4, 96, -4), corners(176, -4, 176, 4, 304, 4, 304, -4))
}

// An axis wall ending near a slanted one slides along its own axis onto
// it, staying straight, and stops on its face.
func TestWallsJointLandsOnASlantedWall(t *testing.T) {
	bs, err := wallsRun(t, 909, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [400, 40]]}, {"points": [[200, 200], [200, 30]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	ws := ofPart(bs, "wall")
	if len(ws) != 2 {
		t.Fatalf("%d pieces", len(ws))
	}
	v := ws[0]
	if v.xy[0].X != 196 {
		v = ws[1]
	}
	for _, p := range v.xy {
		if p.X != 196 && p.X != 204 {
			t.Errorf("the upright wall left x 196..204: %s", xyText(v.xy))
		}
		if face := p.X/10 + 4/math.Cos(math.Atan(0.1)); p.Y < face-0.01 { // the slanted wall's face, above its centreline y = x/10
			t.Errorf("the upright wall reaches into the slanted one at %g: %s", p.X, xyText(v.xy))
		}
	}
	assertNoOverlap(t, ws)
}

// Three walls at one point: the two turning widest mitre, the third is cut
// by both their faces; none overlaps another.
func TestWallsThreeAtAPoint(t *testing.T) {
	bs, err := wallsRun(t, 910, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}, {"points": [[0, 0], [-48, -200]]}, {"points": [[0, 0], [0, -200]]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	ws := ofPart(bs, "wall")
	if len(ws) != 3 {
		t.Fatalf("%d pieces", len(ws))
	}
	assertNoOverlap(t, ws)
}

// A closed line mitres all round; a door opens it.
func TestWallsClosedLineWithADoor(t *testing.T) {
	bs, err := wallsRun(t, 911, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [200, 0], [200, 200], [0, 200]], "closed": true}], "doors": [{"at": [100, 0]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	ws := ofPart(bs, "wall")
	if len(ws) != 5 || len(ofPart(bs, "door 0 lintel")) != 1 {
		t.Fatalf("%d wall pieces (want 5: the door splits the bottom), %d lintels", len(ws), len(ofPart(bs, "door 0 lintel")))
	}
	assertNoOverlap(t, ws)
	bs, err = wallsRun(t, 912, `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [200, 0], [200, 200], [0, 200]], "closed": true}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ofPart(bs, "wall") {
		if len(w.xy) != 4 || w.caulk != 2 {
			t.Errorf("a ring's side %s: %d faces caulk, want both mitres", xyText(w.xy), w.caulk)
		}
	}
	assertNoOverlap(t, ofPart(bs, "wall"))
}

// A plan with every joint and doors: no two wall pieces overlap, no jamb
// overlaps a wall.
func TestWallsNeverOverlap(t *testing.T) {
	bs, err := wallsRun(t, 913, `{"walls": {"material": "brick", "lines": [
		{"points": [[0, 0], [512, 0], [512, 384], [0, 384]], "closed": true},
		{"points": [[256, 0], [256, 384]]},
		{"points": [[0, 192], [512, 192]]},
		{"points": [[512, 100], [700, 130], [700, 300], [512, 300]]},
		{"points": [[60, 384], [60, 520], [200, 560]]}
	], "doors": [{"at": [128, 0]}, {"at": [256, 290]}, {"at": [380, 192]}, {"at": [700, 220]}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	var solid []wbrush
	for _, b := range bs {
		if b.part == "wall" || strings.HasSuffix(b.part, " jamb") {
			solid = append(solid, b)
		}
	}
	assertNoOverlap(t, solid)
	if n := len(ofPart(bs, "door 3 lintel")); n != 1 {
		t.Errorf("the door on the slanted part: %d lintels", n)
	}
}

func TestWallsRefuse(t *testing.T) {
	for want, op := range map[string]string{
		"no wall within 48":  `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}], "doors": [{"at": [128, 100]}]}}`,
		"a 72 door needs 80": `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [64, 0]]}], "doors": [{"at": [32, 0]}]}}`,
		"seen 80 wider":      `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}], "doors": [{"at": [128, 0], "seen": 80}]}}`,
		"height 112: under":  `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [256, 0]]}], "doors": [{"at": [128, 0], "height": 112}]}}`,
		"line 0: two points": `{"walls": {"material": "brick", "lines": [{"points": [[0, 0]]}]}}`,
		`material "a b"`:     `{"walls": {"material": "a b", "lines": [{"points": [[0, 0], [256, 0]]}]}}`,
		"its top above":      `{"walls": {"material": "brick", "z": [100, 50], "lines": [{"points": [[0, 0], [256, 0]]}]}}`,
		"walls: no lines":    `{"walls": {"material": "brick", "lines": []}}`,
		`trim "x y"`:         `{"walls": {"material": "brick", "trim": "x y", "lines": [{"points": [[0, 0], [256, 0]]}]}}`,
		"neither below 0":    `{"walls": {"material": "brick", "thickness": -8, "lines": [{"points": [[0, 0], [256, 0]]}]}}`,
		"three, closed":      `{"walls": {"material": "brick", "lines": [{"points": [[0, 0], [100, 0]], "closed": true}]}}`,
	} {
		if _, err := wallsRun(t, 920, op); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
}

func polyArea(p []geom.P2) float64 {
	s := 0.0
	for i := range p {
		q := p[(i+1)%len(p)]
		s += p[i].X*q.Y - q.X*p[i].Y
	}
	return s / 2
}

// hull orders a brush's ground corners round their middle.
func hull(ps []geom.P2) []geom.P2 {
	var c geom.P2
	for _, p := range ps {
		c.X, c.Y = c.X+p.X/float64(len(ps)), c.Y+p.Y/float64(len(ps))
	}
	out := slices.Clone(ps)
	sort.Slice(out, func(i, j int) bool {
		return math.Atan2(out[i].Y-c.Y, out[i].X-c.X) < math.Atan2(out[j].Y-c.Y, out[j].X-c.X)
	})
	return out
}

func overlapArea(a, b []geom.P2) float64 {
	out := hull(a)
	clipper := hull(b)
	for i := range clipper {
		p, q := clipper[i], clipper[(i+1)%len(clipper)]
		f := wface{at: p, dir: geom.Unit2(p, q), side: 1}
		out = clipHalf(out, f)
		if len(out) < 3 {
			return 0
		}
	}
	return math.Abs(polyArea(out))
}

func assertNoOverlap(t *testing.T, bs []wbrush) {
	t.Helper()
	for i := range bs {
		for j := i + 1; j < len(bs); j++ {
			if a := overlapArea(bs[i].xy, bs[j].xy); a > 0.5 {
				t.Errorf("%s (%s) and %s (%s) overlap by %.1f", bs[i].part, xyText(bs[i].xy), bs[j].part, xyText(bs[j].xy), a)
			}
		}
	}
}

// The joints a plan draws less often: a turn too sharp to mitre keeps a
// half-wall overlap; an end on another line's corner butts on the segment
// there it meets squarely (a closed line's first corner too); four ends at
// one point keep their overlaps; a closed line crossed is cut where crossed;
// a slanted wall's end lands on the nearest point of its wall.
func TestWallsRarerJoints(t *testing.T) {
	for name, c := range map[string]struct {
		op     string
		pieces int
		clean  bool // no wall piece overlaps another
	}{
		"a sharp turn":            {`{"points": [[0, 0], [256, 0], [0, 40]]}`, 2, false},
		"on a corner":             {`{"points": [[0, 0], [256, 0], [256, 256]]}, {"points": [[400, 0], [256, 0]]}`, 3, true},
		"on a ring's first one":   {`{"points": [[0, 0], [200, 0], [200, 200], [0, 200]], "closed": true}, {"points": [[-200, 0], [0, 0]]}`, 5, true},
		"four at a point":         {`{"points": [[0, 0], [128, 0]]}, {"points": [[0, 0], [-128, 0]]}, {"points": [[0, 0], [0, 128]]}, {"points": [[0, 0], [0, -128]]}`, 4, false},
		"a ring crossed":          {`{"points": [[-64, 104], [264, 104]]}, {"points": [[0, 0], [200, 0], [200, 200], [0, 200]], "closed": true}`, 7, true},
		"a slanted end on a wall": {`{"points": [[0, 0], [256, 0]]}, {"points": [[96, 200], [140, 6]]}`, 2, true},
	} {
		bs, err := wallsRun(t, 930, `{"walls": {"material": "brick", "lines": [`+c.op+`]}}`)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		ws := ofPart(bs, "wall")
		if len(ws) != c.pieces {
			t.Errorf("%s: %d pieces, want %d", name, len(ws), c.pieces)
		}
		if c.clean {
			assertNoOverlap(t, ws)
		}
	}
}
