package level

import (
	"fmt"
	"math"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// patchTexels is how far a patch's texture coordinates run across a tile of
// its material for each unit of the tile: the stock mesh patches lay 16 a unit.
const patchTexels = 16

// checkPatches are the patches' findings: a material no GDT defines, or a
// texture that repeats more than once a unit along the patch, which cod2map
// warns of ("repeats more than once per inch") and Radiant's terrain triangle
// writes until its texture is capped.
func checkPatches(snap *Snapshot, checked func(string) bool, info func(string) MaterialInfo) []Finding {
	var out []Finding
	for _, e := range snap.Map.Entities {
		for _, b := range e.Brushes() {
			if k := b.Kind(); (k != mapfile.KindCurve && k != mapfile.KindMesh) || !checked(b.Guid()) {
				continue
			}
			p, err := b.Patch()
			if err != nil {
				continue
			}
			f := Finding{ID: snap.IDs.Short(b.Guid()), Material: p.Material}
			mi := info(p.Material)
			if !mi.Known {
				f.Kind = "unknown"
				out = append(out, f)
				continue
			}
			side := 64.0 // a tile's smaller side, in units
			if mi.Tile.W > 0 && mi.Tile.H > 0 {
				side = math.Min(mi.Tile.W, mi.Tile.H)
			}
			if r := densest(p) / (patchTexels * side); r > 1 {
				f.Kind, f.Detail = "stretched", fmt.Sprintf("its texture repeats %.3g times a unit: cod2map warns past once an inch", r)
				out = append(out, f)
			}
		}
	}
	return out
}

// densest is how fast a patch's texture coordinates run at most, per unit,
// between control points next to each other in a row or a column.
func densest(p *mapfile.Patch) float64 {
	best := 0.0
	step := func(a, b mapfile.Vertex) {
		d := math.Hypot(math.Hypot(b.Pos[0]-a.Pos[0], b.Pos[1]-a.Pos[1]), b.Pos[2]-a.Pos[2])
		if d < 0.01 {
			return
		}
		best = math.Max(best, math.Hypot(b.UV[0]-a.UV[0], b.UV[1]-a.UV[1])/d)
	}
	for r, row := range p.Rows {
		for c, v := range row {
			if c+1 < len(row) {
				step(v, row[c+1])
			}
			if r+1 < len(p.Rows) && c < len(p.Rows[r+1]) {
				step(v, p.Rows[r+1][c])
			}
		}
	}
	return best
}
