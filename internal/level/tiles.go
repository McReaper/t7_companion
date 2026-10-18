package level

import (
	"bufio"
	"cmp"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Tile is the world units one repeat of a material spans on a face, across (u)
// and down (v), and where that size comes from.
type Tile struct {
	W, H float64
	From string
}

// Tiles gives a material's tile; an error leaves map_edit to assume one.
type Tiles func(material string) (Tile, error)

// assumed is the tile of a material nothing sizes: Radiant's own default.
var assumed = Tile{W: 64, H: 64, From: "assumed, no install to size it"}

// TileUse is the size the maps of an install use a material at most: N of its
// Faces faces have it.
type TileUse struct {
	W, H     float64
	N, Faces int
}

// ScanTiles reads every face of the .map files under root's map_source and
// returns, for each material, the tile its faces have most (signs dropped: a
// negative size mirrors the same tile). It reads face lines alone, not whole
// maps, so the install's 1,562 maps take about a second.
// A map or folder that can't be read is left out.
func ScanTiles(root string) map[string]TileUse {
	counts := map[string]map[[2]float64]int{}
	_ = filepath.WalkDir(filepath.Join(root, "map_source"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".map") {
			scanFaces(p, counts)
		}
		return nil
	})
	out := make(map[string]TileUse, len(counts))
	for m, c := range counts {
		sizes := slices.SortedFunc(maps.Keys(c), func(a, b [2]float64) int { // a tie goes to the smaller tile
			return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
		})
		var u TileUse
		for _, wh := range sizes {
			u.Faces += c[wh]
			if c[wh] > u.N {
				u.W, u.H, u.N = wh[0], wh[1], c[wh]
			}
		}
		out[m] = u
	}
	return out
}

// scanFaces counts a map's face lines by material and tile.
func scanFaces(path string, counts map[string]map[[2]float64]int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		l := strings.TrimLeft(sc.Text(), " \t")
		if !strings.HasPrefix(l, "( ") {
			continue
		}
		w := strings.Fields(l)
		if len(w) < 18 || w[4] != ")" || w[14] != ")" {
			continue
		}
		tw, e1 := strconv.ParseFloat(w[16], 64)
		th, e2 := strconv.ParseFloat(w[17], 64)
		if e1 != nil || e2 != nil || tw == 0 || th == 0 {
			continue
		}
		c := counts[w[15]]
		if c == nil {
			c = map[[2]float64]int{}
			counts[w[15]] = c
		}
		c[[2]float64{math.Abs(tw), math.Abs(th)}]++
	}
}
