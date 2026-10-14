package level

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// A board shows materials as Radiant draws them: a slab of each, side by side
// above the map, shot from straight above, fullbright, volumes hidden; then
// the edit that built it is undone, so the map is as it was (Radiant's redo
// holds the board).

// Swatch is one material of a board: the slab that shows it, by the short id
// its label has in the shot, and the tile it was laid at.
type Swatch struct {
	Material string
	ID       string
	Tile     Tile
}

// Board is a shot of materials and what each label in it shows.
type Board struct {
	Shot     *Shot
	Swatches []Swatch
	Note     string // set when the board could not be taken back off the map
}

// swatchSide is a slab's side, wide enough for two repeats of most tiles, so
// the materials' scales compare; swatchGap is the floor between them.
const (
	swatchSide  = 256
	swatchGap   = 32
	maxSwatches = 16 // four rows of four stay readable at 768 pixels
)

// above is the direction a board is shot from: straight down, +y up the image.
var above = Pose{Pitch: -89, Yaw: 90}

// ShowMaterials lays the materials out on a board above the map, shoots it,
// and takes the board off the map again.
func ShowMaterials(ctx context.Context, c *radiant.Client, snap *Snapshot, materials []string, tiles Tiles, size int) (*Board, error) {
	if len(materials) == 0 || len(materials) > maxSwatches {
		return nil, fmt.Errorf("%d materials: a board shows 1 to %d", len(materials), maxSwatches)
	}
	ops := boardOps(snap, materials)
	done, err := Edit(ctx, c, snap, ops, "material board", tiles)
	if err != nil {
		return nil, err
	}
	b := &Board{}
	ids := make([]string, 0, len(done.Made))
	for _, m := range done.Made {
		if m.ID == "" {
			continue
		}
		id := done.After.IDs.Short(m.ID)
		ids = append(ids, id)
		b.Swatches = append(b.Swatches, Swatch{Material: materials[m.Op], ID: id, Tile: done.Tiles[materials[m.Op]]})
	}
	if len(ids) > 0 {
		b.Shot, err = View(ctx, c, done.After, ViewOptions{IDs: ids, Look: &above, Render: "fullbright", Labels: true, Hide: "volumes", Size: size})
	} else {
		err = fmt.Errorf("the board's slabs were not found in the map read back")
	}
	b.Note = takeBack(ctx, c, done)
	if err != nil {
		if b.Note != "" {
			err = fmt.Errorf("%w; and %s", err, b.Note)
		}
		return nil, err
	}
	return b, nil
}

// boardOps are a slab for each material, in rows, centred over the map and
// 512 units above its top.
func boardOps(snap *Snapshot, materials []string) []Op {
	var world Box
	for _, e := range snap.Map.Entities {
		world.Union(EntityBox(e))
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(materials)))))
	rows := (len(materials) + cols - 1) / cols
	step := float64(swatchSide + swatchGap)
	c, z := world.Centre(), world.Max[2]+512
	x0 := c[0] - (float64(cols)*step-swatchGap)/2
	y0 := c[1] + (float64(rows)*step-swatchGap)/2 // the first row at the top of the image
	ops := make([]Op, len(materials))
	for i, m := range materials {
		x, y := x0+float64(i%cols)*step, y0-float64(i/cols)*step
		ops[i] = Op{Box: &BoxOp{Min: mapfile.Vec3{x, y - swatchSide, z}, Max: mapfile.Vec3{x + swatchSide, y, z + 8}, Material: m}}
	}
	return ops
}

// takeBack undoes the board's edit, when the undo menu's last entry is still
// it; otherwise it deletes the slabs (one more entry in the undo menu). It
// says what it could not do, "" when the board is gone.
func takeBack(ctx context.Context, c *radiant.Client, done *Done) string {
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
	if _, err := Edit(ctx, c, done.After, []Op{{Delete: ids}}, "material board removed", nil); err != nil {
		return fmt.Sprintf("the board (%s) is still on the map: %v", strings.Join(ids, ", "), err)
	}
	return ""
}
