package level

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/camera"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// The cases below pin what the CI's mutation run found unchecked.

func TestShortIDsOfTheFirstGUIDsAndOfShortOnes(t *testing.T) {
	ids := NewIDs([]string{"{00000000-1111-0000-0000-000000000000}", "{00000000-2222-0000-0000-000000000000}", "{A}", "{AB}"})
	if s := ids.Short("{00000000-1111-0000-0000-000000000000}"); s != "000000001" {
		t.Errorf("the first sorted shares 8 digits with the second: %q", s)
	}
	if s := ids.Short("{00000000-2222-0000-0000-000000000000}"); s != "000000002" {
		t.Errorf("the second sorted shares 8 digits with the first: %q", s)
	}
	if ids.Short("{A}") != "a" || ids.Short("{AB}") != "ab" {
		t.Errorf("a GUID that starts another: %q, %q", ids.Short("{A}"), ids.Short("{AB}"))
	}
	if commonPrefix("ab", "a") != 1 || commonPrefix("a", "ab") != 1 {
		t.Error("commonPrefix")
	}
}

func TestSelectionLinesAndGlobsAtTheirEdges(t *testing.T) {
	if s := ParseSelection("{B} {E}\n{C}\n"); !s.Brush("{B}") || len(s.GUIDs) != 1 {
		t.Errorf("a line of two fields: %+v", s)
	}
	for _, tc := range []struct {
		p, s string
		want bool
	}{{"a*b*c", "abc", true}, {"a*b*b", "ab", false}, {"a*bc*d", "axbcd", true}} {
		if glob(tc.p, tc.s) != tc.want {
			t.Errorf("glob(%q, %q) != %v", tc.p, tc.s, tc.want)
		}
	}
}

func TestACaptureNeedsBothSides(t *testing.T) {
	for _, s := range []string{"w=0 h=5", "w=5 h=0"} {
		if _, err := parseSummary(s); err == nil {
			t.Errorf("%q has a size", s)
		}
	}
	if num(-0.0001) != "0" || num(1.23456) != "1.235" {
		t.Errorf("num: %q %q", num(-0.0001), num(1.23456))
	}
}

func TestAFailedShotWithNothingToPutBackSaysOnlyWhy(t *testing.T) {
	f, c, snap := viewing(t, 401)
	f.Update(func(f *radianttest.Fake) { f.Minimised = true })
	_, err := View(context.Background(), c, snap, ViewOptions{})
	if err == nil || err.Error() != "Radiant capture: minimised: Radiant is minimised: it draws nothing" {
		t.Errorf("err = %v", err)
	}
}

func TestAPointIsFramedWithItsRadius(t *testing.T) {
	_, c, snap := viewing(t, 402)
	at := mapfile.Vec3{100, 200, 0}
	scales := map[float64]float64{}
	for _, r := range []float64{0, 256, 64} {
		shot, err := View(context.Background(), c, snap, ViewOptions{View: "top", At: &at, Radius: r})
		if err != nil {
			t.Fatal(err)
		}
		if *shot.Centre != at {
			t.Errorf("radius %v: centre %v", r, *shot.Centre)
		}
		scales[r] = shot.Scale
	}
	// a 90×160 view: 512 units want 0.153 px/unit (9 steps out), 128 want 0.611 (3 out)
	if scales[0] != scales[256] || fmt.Sprintf("%.4f", scales[256]) != "0.1342" || fmt.Sprintf("%.4f", scales[64]) != "0.5120" {
		t.Errorf("scales %v: no radius is 256", scales)
	}
}

func TestOnlyTheThousandPointsNearestTheTargetGetLabels(t *testing.T) {
	var text strings.Builder
	text.WriteString("{\nguid \"{00000000-0000-0000-0000-0000000000F0}\"\n\"classname\" \"worldspawn\"\n}\n")
	for i := range 1000 {
		fmt.Fprintf(&text, "{\nguid \"{%08X-0000-0000-0000-000000000000}\"\n\"classname\" \"info_null\"\n\"origin\" \"%d %d 0\"\n}\n", 0x10000000+i, 3000+i%40, 3000+i/40)
	}
	// far past the target along the framing camera's view: it lands in the middle of the shot
	fwd, _, _ := camera.Looking.Basis()
	fmt.Fprintf(&text, "{\nguid \"{0FFFFFFF-0000-0000-0000-000000000000}\"\n\"classname\" \"info_null\"\n\"origin\" \"%s\"\n}\n",
		vecText(mapfile.Vec3{fwd[0] * 20000, fwd[1] * 20000, fwd[2] * 20000}))
	text.WriteString("{\nguid \"{0EEEEEEE-0000-0000-0000-000000000000}\"\n\"classname\" \"info_null\"\n\"origin\" \"0 0 0\"\n}\n")
	f2 := radianttest.New(403, "x.map")
	f2.Text = text.String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c2, err := radiant.NewClient(ctx, f2.Dial())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	snap, err := Read(context.Background(), c2)
	if err != nil {
		t.Fatal(err)
	}
	shot, err := View(context.Background(), c2, snap, ViewOptions{At: &mapfile.Vec3{}, Radius: 64, Labels: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range shot.Labels {
		if l.ID == "0fffffff" {
			t.Errorf("the 1002nd point nearest the target got a label: %+v", l)
		}
	}
	if len(shot.Labels) == 0 || shot.Labels[0].ID != "0eeeeeee" {
		t.Errorf("labels %+v: the target's own point first", shot.Labels)
	}
}

func TestAShotWhoseCornersAllFellBehindIsNotRetaken(t *testing.T) {
	f, c, snap := viewing(t, 404)
	f.Handle("capture", func(_ *radianttest.Fake, _ string, body []byte) radianttest.Reply {
		n := strings.Count(string(body), "\n")
		frame := []byte{0, 0, 0, 255, 9, 9, 9, 255, 0, 0, 0, 255, 0, 0, 0, 255}
		return radianttest.OK("w=2 h=2 origin=0,0,0 angles=0,0,0 game_view=0", append(frame, strings.Repeat("-\n", n)...))
	})
	if _, err := View(context.Background(), c, snap, ViewOptions{IDs: []string{"5ec8de49"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(sent(f, "capture 3d")); n != 1 {
		t.Errorf("%d captures: no focal to frame again with", n)
	}
}

func TestFitsAtItsEdges(t *testing.T) {
	for name, tc := range map[string]struct {
		at   [][2]float64
		want bool
	}{
		"corner to corner":     {[][2]float64{{0, 0}, {100, 50}}, true},
		"half the width only":  {[][2]float64{{25, 25}, {75, 25}}, false},
		"half the height only": {[][2]float64{{50, 0}, {50, 25}}, false},
	} {
		if camera.Fits(tc.at, 100, 50) != tc.want {
			t.Errorf("%s: fits != %v", name, tc.want)
		}
	}
}

func TestComposeScalesTheProjectionAndOrdersTheLabels(t *testing.T) {
	fr := &frame{W: 200, H: 100, Pixels: make([]byte, 200*100*4), Centre: mapfile.Vec3{1, 2, 3}, Scale: 2,
		At: [][2]float64{{0, 0}, {200, 10}, {199.9, 99.9}, {100, 100}, {100, 80}, {124, 50}, {math.NaN(), 0}, {76, 50}}}
	pts := []labelPoint{{id: "edge"}, {id: "right out"}, {id: "corner"}, {id: "bottom out"}, {id: "below"}, {id: "right"}, {id: "behind"}, {id: "left"}}
	shot, err := compose(fr, "xy", pts, 100)
	if err != nil {
		t.Fatal(err)
	}
	if shot.Scale != 1 || shot.Image.Rect.Dx() != 100 {
		t.Errorf("scale %v, width %d: the view shrank by half", shot.Scale, shot.Image.Rect.Dx())
	}
	var got []string
	for _, l := range shot.Labels {
		got = append(got, fmt.Sprintf("%s %d,%d", l.ID, l.X, l.Y))
	}
	// distances² to the centre (50,25): right and left 144 each, below 225, the corner 2977, the edge 3125
	if want := []string{"right 62,25", "left 38,25", "below 50,40", "corner 99,49", "edge 0,0"}; !slices.Equal(got, want) {
		t.Errorf("labels %q\nwant %q", got, want)
	}
}

func TestLabelsComeInIDOrderAndByDistance(t *testing.T) {
	m := testMap(t)
	snap := &Snapshot{Map: m, IDs: NewIDs(GUIDs(m))}
	pts := labelPoints(snap, ViewOptions{}, nil)
	ids := make([]string, len(pts))
	for i, p := range pts {
		ids[i] = p.id
	}
	if !slices.IsSorted(ids) || len(ids) != 4 {
		t.Errorf("label points %q: in id order, so ties at one distance keep it", ids)
	}
	// the nearer point first, whichever came first
	fr := &frame{W: 100, H: 50, Pixels: make([]byte, 100*50*4), At: [][2]float64{{62, 25}, {50, 40}}}
	shot, err := compose(fr, "3d", []labelPoint{{id: "right"}, {id: "below"}}, 100)
	if err != nil || len(shot.Labels) != 2 || shot.Labels[0].ID != "right" {
		t.Errorf("labels %+v, %v", shot.Labels, err)
	}
	fr.At = [][2]float64{{50, 40}, {62, 25}}
	shot, _ = compose(fr, "3d", []labelPoint{{id: "below"}, {id: "right"}}, 100)
	if shot.Labels[0].ID != "right" {
		t.Errorf("labels %+v", shot.Labels)
	}
}

func TestBatchesThatCountedToNothing(t *testing.T) {
	f, c, _ := viewing(t, 405)
	f.Update(func(f *radianttest.Fake) {
		f.Text += "{\nguid \"{0A000000-0000-0000-0000-000000000002}\"\n\"classname\" \"info_null\"\n\"origin\" \"0 0 0\"\n}\n"
	})
	for name, js := range map[string]string{
		"a delete and a replace": `[{"delete": ["76780cea"]}, {"set": {"id": "0a000000", "keys": {"a": "1"}}}]`,
		"a replace and a box":    `[{"set": {"id": "0a000000", "keys": {"b": "2"}}}, {"box": {"min": [0,0,0], "max": [8,8,8], "material": "caulk"}}]`,
	} {
		snap, err := Read(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Edit(context.Background(), c, snap, ops(t, js), "", nil); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	snap, _ := Read(context.Background(), c)
	b := newBatch(snap, nil)
	if err := b.apply(ops(t, `[{"delete": ["f2d51a52"]}]`)); err != nil {
		t.Fatal(err)
	}
	p, err := b.plan()
	if err != nil || p.body() != "delete {F2D51A52-C5B3-11F1-B542-00155DD185EA}\n" {
		t.Errorf("a delete alone imports nothing: %q, %v", p.body(), err)
	}
	if s := summary(ops(t, `[{"delete": ["x"]}]`)); s != "1 delete" {
		t.Errorf("summary = %q", s)
	}
}

func TestMovingAnEntityMovesAllItsBrushes(t *testing.T) {
	f, c, _ := viewing(t, 406)
	two := "{\nguid \"{0B100000-0000-0000-0000-000000000000}\"\n\"classname\" \"script_brushmodel\"\n\"origin\" \"0 0 0\"\n" +
		strings.Replace(BoxBrush(box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{8, 8, 8}), "caulk", ""), "{\n", "{\n guid \"{0B100000-0000-0000-0000-000000000001}\"\n", 1) +
		strings.Replace(BoxBrush(box(mapfile.Vec3{16, 0, 0}, mapfile.Vec3{24, 8, 8}), "caulk", ""), "{\n", "{\n guid \"{0B100000-0000-0000-0000-000000000002}\"\n", 1) + "}\n"
	f.Update(func(f *radianttest.Fake) { f.Text += two })
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Edit(context.Background(), c, snap, ops(t, `[{"move": {"ids": ["{0B100000-0000-0000-0000-000000000000}"], "by": [100, 0, 0]}}]`), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := d.Made[0]; m.Box != box(mapfile.Vec3{100, 0, 0}, mapfile.Vec3{124, 8, 8}) || m.Class != "script_brushmodel" {
		t.Errorf("moved %+v", m)
	}
	hits, _ := Find(d.After.Map, d.After.IDs, Query{Class: "script_brushmodel"}, Selection{})
	if o, _ := geom.Origin(hits[0].Entity); o != (mapfile.Vec3{100, 0, 0}) {
		t.Errorf("origin %v", o)
	}
}
