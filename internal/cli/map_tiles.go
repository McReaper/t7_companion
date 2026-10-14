package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/level"
)

// texelsPerUnit is the density a material's GDT tiling is laid at when the
// install's maps don't size it: 79 of the 96 materials with a set tiling that
// stock maps use often have it at 16 texels a unit.
const texelsPerUnit = 16

// minTileFaces is how many faces of the install's maps must have a material
// before the tile they have it at most is its default.
const minTileFaces = 10

var (
	tileUseMu sync.Mutex
	tileUse   = map[string]map[string]level.TileUse{} // lower-cased root -> material -> its use
)

// mapTiles sizes a material's tile for map_edit, in the install the map is
// in: the tile the install's maps have it at most, else its GDT's tiling at
// texelsPerUnit. Nil when the map is in no install.
func mapTiles(mapPath string) level.Tiles {
	return tilesIn(installOf(mapPath))
}

// tilesIn sizes a material's tile in an install; nil for no install.
func tilesIn(root string) level.Tiles {
	if root == "" {
		return nil
	}
	return func(m string) (level.Tile, error) {
		if u, ok := tileUses(root)[m]; ok && u.Faces >= minTileFaces {
			return level.Tile{W: u.W, H: u.H, From: fmt.Sprintf("the install's maps, on %d of its %d faces", u.N, u.Faces)}, nil
		}
		w, err := workspace(root)
		if err != nil {
			return level.Tile{}, err
		}
		t, err := w.MaterialTiling(m)
		if err != nil {
			return level.Tile{}, err
		}
		return level.Tile{W: float64(t.W) / texelsPerUnit, H: float64(t.H) / texelsPerUnit,
			From: fmt.Sprintf("its GDT's %d×%d texels at %d a unit", t.W, t.H, texelsPerUnit)}, nil
	}
}

// tileUses is the install's maps' use of each material, read once per root.
func tileUses(root string) map[string]level.TileUse {
	key := strings.ToLower(filepath.Clean(root))
	tileUseMu.Lock()
	defer tileUseMu.Unlock()
	if u, ok := tileUse[key]; ok {
		return u
	}
	u := level.ScanTiles(root)
	tileUse[key] = u
	return u
}

// installOf is the mod-tools root a map lives in: the nearest folder above it
// that holds deffiles, else $TA_TOOLS_PATH's.
func installOf(mapPath string) string {
	if mapPath != "" {
		for d := filepath.Dir(mapPath); ; d = filepath.Dir(d) {
			if fi, err := os.Stat(filepath.Join(d, "deffiles")); err == nil && fi.IsDir() {
				return d
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	return toolsRoot("")
}
