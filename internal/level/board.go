package level

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/camera"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// A board shows materials or models as Radiant draws them: a slab of each
// material, or each model placed, side by side above the map, shot with a
// number on each; then the edit that built it is undone, so the map is as it
// was (Radiant's redo holds the board).

// Swatch is one thing a board shows: the material or model, the label it
// has in the shot (1, 2…), and for a material the tile it was laid at.
type Swatch struct {
	Name  string
	Label string
	Tile  Tile
}

// Board is a shot of materials or models and what each label in it shows.
type Board struct {
	Shot     *Shot
	Swatches []Swatch
	Note     string // set when the board could not be taken back off the map
}

// swatchSide is a slab's side, wide enough for two repeats of most tiles, so
// the materials' scales compare; swatchGap is the floor between them.
// modelStep is the room a model has, a prop's: a building's overlaps its
// neighbours'. modelRow models stand in a row, as wide as the view is.
const (
	swatchSide  = 256
	swatchGap   = 32
	modelStep   = 128
	modelRow    = 4
	modelHeight = 128 // the height a model board is framed for
	maxSwatches = 16  // four rows of four stay readable at 768 pixels
)

// above is the direction a board is shot from: straight down, +y up the image.
var above = camera.Pose{Pitch: -89, Yaw: 90} // at -90 Radiant drops the yaw: the board comes out turned

// boardItem is one thing on a board: the op that puts it there, the box the
// shot frames, and where its number goes.
type boardItem struct {
	name string
	op   Op
	box  geom.Box
	mark mapfile.Vec3
}

// boardShot is how a board is shot: the edit's label, the render, and the
// way its box is looked at.
type boardShot struct {
	label  string
	render string
	look   camera.Pose
}

// ShowMaterials lays the materials out on a board above the map, shoots it
// from straight above, fullbright, and takes the board off the map again.
func ShowMaterials(ctx context.Context, c *radiant.Client, snap *Snapshot, materials []string, tiles Tiles, size int) (*Board, error) {
	if len(materials) == 0 || len(materials) > maxSwatches {
		return nil, fmt.Errorf("%d materials: a board shows 1 to %d", len(materials), maxSwatches)
	}
	return showBoard(ctx, c, snap, materialItems(snap, materials), tiles, size, boardShot{"material board", "fullbright", above})
}

// ShowModels places the models above the map, a misc_model each in rows,
// shoots them from the front under fake lighting (their shapes read,
// unbaked), and takes them off the map again.
func ShowModels(ctx context.Context, c *radiant.Client, snap *Snapshot, models []string, size int) (*Board, error) {
	if len(models) == 0 || len(models) > maxSwatches {
		return nil, fmt.Errorf("%d models: a board shows 1 to %d", len(models), maxSwatches)
	}
	return showBoard(ctx, c, snap, modelItems(snap, models), nil, size, boardShot{"model board", "fake", modelLook})
}

// modelLook is the way a model board is looked at: from the front (-y),
// a little above, so a second row shows behind the first.
var modelLook = camera.Pose{Pitch: -35, Yaw: 90}

// showBoard puts the items on the map, shoots them numbered, and takes them
// back.
func showBoard(ctx context.Context, c *radiant.Client, snap *Snapshot, items []boardItem, tiles Tiles, size int, how boardShot) (*Board, error) {
	ops := make([]Op, len(items))
	for i, it := range items {
		ops[i] = it.op
	}
	done, err := Edit(ctx, c, snap, ops, how.label, tiles)
	if err != nil {
		return nil, err
	}
	b := &Board{}
	var marks []Mark
	var board geom.Box
	for _, m := range done.Made {
		it := items[m.Op]
		board.Union(it.box)
		label := fmt.Sprint(m.Op + 1)
		marks = append(marks, Mark{label, it.mark})
		b.Swatches = append(b.Swatches, Swatch{Name: it.name, Label: label, Tile: done.Tiles[it.name]})
	}
	if board.Ok { // framed as View frames a target: shot again with the lens it measures when the guess was off
		b.Shot, err = View(ctx, c, done.After, ViewOptions{Frame: &board, Look: &how.look, Render: how.render, Labels: true, Marks: marks, Hide: "volumes,sky", Size: size})
	} else {
		err = fmt.Errorf("nothing of the board was found in the map read back")
	}
	b.Note = takeBack(ctx, c, done, how.label)
	if err != nil {
		if b.Note != "" {
			err = fmt.Errorf("%w; and %s", err, b.Note)
		}
		return nil, err
	}
	return b, nil
}

// cornerOf is where a slab's label goes: inside its upper-left corner (-x,
// +y, the image's), on its top, clear of most of its texture.
func cornerOf(b geom.Box) mapfile.Vec3 {
	return mapfile.Vec3{b.Min[0] + 20, b.Max[1] - 20, b.Max[2]}
}

// grid is a board's places, step apart in rows, centred over the map and 512
// units above its top: each place's upper-left corner (the first row at the
// top of the image), and the height.
func grid(snap *Snapshot, n int, step, gap float64) ([]mapfile.Vec3, float64) {
	var world geom.Box
	for _, e := range snap.Map.Entities {
		world.Union(geom.EntityBox(e))
	}
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	rows := (n + cols - 1) / cols
	c, z := world.Centre(), world.Max[2]+512
	x0 := c[0] - (float64(cols)*step-gap)/2
	y0 := c[1] + (float64(rows)*step-gap)/2
	at := make([]mapfile.Vec3, n)
	for i := range at {
		at[i] = mapfile.Vec3{x0 + float64(i%cols)*step, y0 - float64(i/cols)*step, z}
	}
	return at, z
}

// materialItems are a slab for each material.
func materialItems(snap *Snapshot, materials []string) []boardItem {
	at, z := grid(snap, len(materials), swatchSide+swatchGap, swatchGap)
	items := make([]boardItem, len(materials))
	for i, m := range materials {
		b := geom.Box{Min: mapfile.Vec3{at[i][0], at[i][1] - swatchSide, z}, Max: mapfile.Vec3{at[i][0] + swatchSide, at[i][1], z + 8}, Ok: true}
		items[i] = boardItem{m, Op{Box: &BoxOp{Min: b.Min, Max: b.Max, Material: m}}, b, cornerOf(b)}
	}
	return items
}

// modelItems are a misc_model for each model, modelRow to a row running +x
// and the next row behind it (+y), centred over the map 512 units above its
// top; each in the middle of its place, its number in the place's front
// left corner, clear of the model.
func modelItems(snap *Snapshot, models []string) []boardItem {
	var world geom.Box
	for _, e := range snap.Map.Entities {
		world.Union(geom.EntityBox(e))
	}
	cols := min(len(models), modelRow)
	rows := (len(models) + modelRow - 1) / modelRow
	c, z := world.Centre(), world.Max[2]+512
	x0, y0 := c[0]-float64(cols)*modelStep/2, c[1]-float64(rows)*modelStep/2
	items := make([]boardItem, len(models))
	for i, m := range models {
		x, y := x0+float64(i%modelRow)*modelStep, y0+float64(i/modelRow)*modelStep
		o := mapfile.Vec3{x + modelStep/2, y + modelStep/2, z}
		b := geom.Box{Min: mapfile.Vec3{x, y, z}, Max: mapfile.Vec3{x + modelStep, y + modelStep, z + modelHeight}, Ok: true}
		items[i] = boardItem{m, Op{Entity: &EntityOp{Class: "misc_model", Origin: o, Keys: map[string]string{"model": m}}}, b, mapfile.Vec3{x + 8, y + 8, z}}
	}
	return items
}

// takeBack undoes the board's edit, when the undo menu's last entry is still
// it; otherwise it deletes what it made (one more entry in the undo menu). It
// says what it could not do, "" when the board is gone.
func takeBack(ctx context.Context, c *radiant.Client, done *Done, label string) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	if undoEntry(ctx, c) == done.Label {
		if err := call(ctx, c, "command Edit/Undo"); err == nil {
			return ""
		}
	}
	var ids []string
	for _, m := range done.Made {
		if m.ID != "" {
			ids = append(ids, done.After.IDs.Short(m.ID))
		}
	}
	if len(ids) == 0 {
		return ""
	}
	if _, err := Edit(ctx, c, done.After, []Op{{Delete: ids}}, label+" removed", nil); err != nil {
		return fmt.Sprintf("the board (%s) is still on the map: %v", strings.Join(ids, ", "), err)
	}
	return ""
}
