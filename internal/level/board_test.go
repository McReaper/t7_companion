package level

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// A board of three materials: slabs in two rows above the map, shot from
// straight above, fullbright, volumes hidden; then undone, the map as it was.
func TestABoardShowsMaterialsAndLeavesTheMapAsItWas(t *testing.T) {
	f, c, snap := viewing(t, 501)
	var before string
	f.Update(func(f *radianttest.Fake) { before = f.Text })
	tiles := func(m string) (Tile, error) { return Tile{W: 128, H: 64, From: "test"}, nil }
	b, err := ShowMaterials(context.Background(), c, snap, []string{"brick", "wood", "caulk"}, tiles, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Swatches) != 3 || b.Note != "" || b.Shot == nil || len(b.Shot.Labels) != 3 {
		t.Fatalf("board %+v", b)
	}
	for i, s := range b.Swatches {
		if s.Name != []string{"brick", "wood", "caulk"}[i] || s.Tile.W != 128 || s.Label != fmt.Sprint(i+1) {
			t.Errorf("swatch %d: %+v", i, s)
		}
	}
	if b.Shot.Pose == nil || b.Shot.Pose.Pitch != -89 || b.Shot.Pose.Yaw != 90 {
		t.Errorf("shot from %+v: straight above", b.Shot.Pose)
	}
	f.Update(func(f *radianttest.Fake) {
		if f.Text != before || f.UndoLabel != "" || f.Render != "Real Lighting" && f.Render != "" || f.Hidden != nil {
			t.Errorf("after: the map changed, undo %q, render %q, hidden %v", f.UndoLabel, f.Render, f.Hidden)
		}
	})
	var seq []string
	for _, r := range f.Requests() {
		if strings.HasPrefix(r, "edit ") || r == "command Edit/Undo" || strings.HasPrefix(r, "command View/Hide") || strings.HasPrefix(r, "command View/Render Method/Fullbright") {
			seq = append(seq, r)
		}
	}
	if strings.Join(seq, "|") != "edit t7kb: material board|command View/Hide Entities/Hide Selected|command View/Render Method/Fullbright|command Edit/Undo" {
		t.Errorf("requests %q", seq)
	}
}

func TestABoardsSlabsSitAboveTheMapInRows(t *testing.T) {
	_, _, snap := viewing(t, 502)
	var world Box
	for _, e := range snap.Map.Entities {
		world.Union(EntityBox(e))
	}
	items := materialItems(snap, []string{"a", "b", "c", "d", "e"})
	if len(items) != 5 {
		t.Fatalf("%d items", len(items))
	}
	ops := make([]Op, len(items))
	for i, it := range items {
		ops[i] = it.op
		if it.name != "abcde"[i:i+1] || it.box != box(it.op.Box.Min, it.op.Box.Max) || it.mark != cornerOf(it.box) {
			t.Errorf("item %d: %+v", i, it)
		}
	}
	// three columns, two rows; the first row at the top of the image (+y)
	first, second, fourth := ops[0].Box, ops[1].Box, ops[3].Box
	if first.Min[2] != world.Max[2]+512 || first.Max[2]-first.Min[2] != 8 || first.Max[0]-first.Min[0] != swatchSide {
		t.Errorf("first slab %+v over a map up to z %v", first, world.Max[2])
	}
	if second.Min[0]-first.Min[0] != swatchSide+swatchGap || second.Min[1] != first.Min[1] || first.Min[1]-fourth.Min[1] != swatchSide+swatchGap || fourth.Min[0] != first.Min[0] {
		t.Errorf("slabs %v %v %v: a row runs +x, the next one below it", first.Min, second.Min, fourth.Min)
	}
	mid := mapfile.Vec3{(first.Min[0] + ops[2].Box.Max[0]) / 2, (first.Max[1] + fourth.Min[1]) / 2}
	if c := world.Centre(); mid[0] != c[0] || mid[1] != c[1] {
		t.Errorf("the board's middle %v, the map's %v", mid, c)
	}
}

// When the undo menu's last entry is no longer the board's, the slabs are
// deleted instead; a board shows 1 to 16 materials.
func TestABoardIsDeletedWhenItCannotBeUndone(t *testing.T) {
	f, c, snap := viewing(t, 503)
	f.Handle("menus", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.OK("", []byte("Undo something of the user's\t1\t0\n"))
	})
	b, err := ShowMaterials(context.Background(), c, snap, []string{"brick"}, nil, 0)
	if err != nil || b.Note != "" {
		t.Fatalf("%+v, %v", b, err)
	}
	if got := sent(f, "edit "); len(got) != 2 || got[1] != "edit t7kb: material board removed" {
		t.Errorf("edits %q", got)
	}
	after, _ := Read(context.Background(), c)
	if len(GUIDs(after.Map)) != len(GUIDs(snap.Map)) {
		t.Errorf("%d GUIDs after, %d before: the slab is still on the map", len(GUIDs(after.Map)), len(GUIDs(snap.Map)))
	}
	if _, err := ShowMaterials(context.Background(), c, snap, make16(), nil, 0); err != nil {
		t.Errorf("16 materials, the most a board shows: %v", err)
	}
	for _, n := range []int{0, 17} {
		if _, err := ShowMaterials(context.Background(), c, snap, make([]string, n), nil, 0); err == nil || !strings.Contains(err.Error(), "a board shows 1 to 16") {
			t.Errorf("%d materials: %v", n, err)
		}
	}
}

// Framed from above, a board fills the view and sits in its middle, the
// tilt aside.
func TestFramingFromAboveCentresAFlatBoxInTheView(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	b := box(mapfile.Vec3{-400, 100, 500}, mapfile.Vec3{400, 600, 508})
	p := FrameBox(l, above, b)
	x, y, ok := l.Project(p, b.Centre())
	if !ok || math.Abs(x-824) > 1 || math.Abs(y-434.5) > 1 {
		t.Errorf("the board's middle at %v, %v", x, y)
	}
	for _, c := range b.Corners() {
		if x, y, ok := l.Project(p, c); !ok || x < 0 || x > 1648 || y < 0 || y > 869 {
			t.Errorf("corner %v at %v, %v: out of the view", c, x, y)
		}
	}
	if p.Pitch != -89 || p.Yaw != 90 {
		t.Errorf("pose %+v", p)
	}
	// It fills the view: its width or its depth spans most of the image's.
	var lo, hi [2]float64
	for i, c := range b.Corners() {
		x, y, _ := l.Project(p, c)
		if i == 0 {
			lo, hi = [2]float64{x, y}, [2]float64{x, y}
		}
		lo, hi = [2]float64{math.Min(lo[0], x), math.Min(lo[1], y)}, [2]float64{math.Max(hi[0], x), math.Max(hi[1], y)}
	}
	if fill := math.Max((hi[0]-lo[0])/1648, (hi[1]-lo[1])/869); fill < 0.85 {
		t.Errorf("the board fills %.0f%% of the view", fill*100)
	}
	// A wide board fits its width, a deep one its depth.
	wide, deep := FrameBox(l, above, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1648, 100, 8})), FrameBox(l, above, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{100, 869, 8}))
	if math.Abs(wide.Origin[2]-(4+1216.4*margin)) > 1 || math.Abs(deep.Origin[2]-wide.Origin[2]) > wide.Origin[2]/100 { // the 1° tilt aside
		t.Errorf("a board as wide as the view and one as deep are shot from %v and %v", wide.Origin[2], deep.Origin[2])
	}
	if c := cornerOf(box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{256, 256, 8})); c != (mapfile.Vec3{20, 236, 8}) {
		t.Errorf("a slab's label at %v: inside its -x, +y corner, on its top", c)
	}
}

func make16() []string {
	out := make([]string, 16)
	for i := range out {
		out[i] = fmt.Sprintf("m%d", i)
	}
	return out
}

// A model board places a misc_model in the middle of each place, modelRow
// to a row running +x, the next row behind (+y), centred over the map; each
// number in its place's front-left corner.
func TestAModelBoardsModelsStandInRowsAboveTheMap(t *testing.T) {
	_, _, snap := viewing(t, 503)
	var world Box
	for _, e := range snap.Map.Entities {
		world.Union(EntityBox(e))
	}
	names := []string{"p0", "p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8", "p9"}
	items := modelItems(snap, names)
	c, z := world.Centre(), world.Max[2]+512
	for i, it := range items {
		col, row, rows := float64(i%modelRow), float64(i/modelRow), float64((len(names)+modelRow-1)/modelRow)
		x, y := c[0]-float64(modelRow)*modelStep/2+col*modelStep, c[1]-rows*modelStep/2+row*modelStep
		want := mapfile.Vec3{x + modelStep/2, y + modelStep/2, z}
		e := it.op.Entity
		if e == nil || e.Class != "misc_model" || e.Keys["model"] != names[i] || it.name != names[i] || e.Origin != want {
			t.Errorf("model %d: %+v at %v, want at %v", i, e, e.Origin, want)
		}
		if it.mark != (mapfile.Vec3{x + 8, y + 8, z}) || it.box != box(mapfile.Vec3{x, y, z}, mapfile.Vec3{x + modelStep, y + modelStep, z + modelHeight}) {
			t.Errorf("model %d: mark %v, framed in %+v", i, it.mark, it.box)
		}
	}
	if one := modelItems(snap, names[:1]); one[0].op.Entity.Origin != (mapfile.Vec3{c[0], c[1], z}) {
		t.Errorf("one model stands over the map's centre: %v", one[0].op.Entity.Origin)
	}
	for _, it := range modelItems(snap, names[:modelRow]) { // a full row is one row, over the map's centre
		if it.op.Entity.Origin[1] != c[1] {
			t.Errorf("a row of %d: %v", modelRow, it.op.Entity.Origin)
		}
	}
}
