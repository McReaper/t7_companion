package level

import (
	"context"
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
		if s.Material != []string{"brick", "wood", "caulk"}[i] || s.Tile.W != 128 || len(s.ID) < 8 {
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
	if _, err := after.IDs.Resolve(b.Swatches[0].ID); err == nil {
		t.Error("the slab is still on the map")
	}
	for _, n := range []int{0, 17} {
		if _, err := ShowMaterials(context.Background(), c, snap, make([]string, n), nil, 0); err == nil || !strings.Contains(err.Error(), "a board shows 1 to 16") {
			t.Errorf("%d materials: %v", n, err)
		}
	}
}
