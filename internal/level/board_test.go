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
		if s.Material != []string{"brick", "wood", "caulk"}[i] || s.Tile.W != 128 || s.Label != fmt.Sprint(i+1) {
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
	ops := boardOps(snap, []string{"a", "b", "c", "d", "e"})
	if len(ops) != 5 {
		t.Fatalf("%d ops", len(ops))
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

// The board fills the view and sits in its middle, the tilt aside.
func TestOverheadCentresAFlatBoxInTheView(t *testing.T) {
	l := Lens{W: 1648, H: 869, F: 1212.4}
	b := box(mapfile.Vec3{-400, 100, 500}, mapfile.Vec3{400, 600, 508})
	p := overhead(l, b)
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
	wide, deep := overhead(l, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{1648, 100, 8})), overhead(l, box(mapfile.Vec3{0, 0, 0}, mapfile.Vec3{100, 869, 8}))
	if math.Abs(wide.Origin[2]-(8+1212.4*boardMargin)) > 0.01 || math.Abs(deep.Origin[2]-wide.Origin[2]) > 0.01 {
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
