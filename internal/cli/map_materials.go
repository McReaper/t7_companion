package cli

import (
	"cmp"
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapMaterialsArgs are map_materials' filters, and how to show the materials
// found: their color images on a sheet, or a board in Radiant.
type mapMaterialsArgs struct {
	Usage, Surface, Name string // comma-separated alternatives; a name's words must all be in it
	Limit                int
	All                  bool // DO NOT USE materials too
	Show                 bool // a sheet of their color images, read from the install
	Board                bool // a board of them in Radiant, shot in its 3D view
	Size                 int
	ToolsPath            string
	PID                  int
}

// mapMaterialsOut is a map_materials answer: the materials found, most used
// by the install's maps first, and with show, which label of the image is
// which.
type mapMaterialsOut struct {
	Found     int           `json:"found"` // how many match, before the limit
	Materials []materialOut `json:"materials"`
	Sheet     []string      `json:"sheet,omitempty"` // the material numbered i+1 on the sheet
	Board     []swatchOut   `json:"board,omitempty"`
	W         int           `json:"w,omitempty"`
	H         int           `json:"h,omitempty"`
	Notes     []string      `json:"notes,omitempty"`
}

type materialOut struct {
	Name     string     `json:"name"`
	Usage    string     `json:"usage"`
	Surface  string     `json:"surface,omitempty"`
	Tile     [2]float64 `json:"tile"`            // world units one repeat spans, as map_edit lays it
	Faces    int        `json:"faces,omitempty"` // faces of the install's maps in it
	DoNotUse bool       `json:"do_not_use,omitempty"`
	color    string     // its color image's asset, what the sheet shows
}

type swatchOut struct {
	Label    string `json:"label"` // its slab's label in the image, in the slab's upper-left corner
	Material string `json:"material"`
	At       [2]int `json:"at"` // the label's pixel
}

// Limits of a list: the default, and the most one answer holds.
const (
	materialsLimit    = 20
	maxMaterialsLimit = 200
)

// mapMaterials lists the materials Radiant's browser shows that match; with
// show, a sheet of the first 16's color images; with board, the first 16 laid
// out in Radiant and shot.
func mapMaterials(ctx context.Context, a mapMaterialsArgs) (*mapMaterialsOut, []byte, error) {
	if a.Limit < 0 || a.Limit > maxMaterialsLimit {
		return nil, nil, fmt.Errorf("limit %d: at most %d", a.Limit, maxMaterialsLimit)
	}
	if a.Size < 0 || a.Size > maxViewSize {
		return nil, nil, fmt.Errorf("size %d: at most %d pixels", a.Size, maxViewSize)
	}
	root := toolsRoot(a.ToolsPath)
	var c *radiant.Client
	var snap *level.Snapshot
	if a.Board || root == "" { // the Radiant's own install, and the map to show them in
		var err error
		if c, err = radiant.Connect(ctx, radiantMachine, a.PID); err != nil {
			return nil, nil, err
		}
		defer c.Close()
		if snap, err = level.Read(ctx, c); err != nil {
			return nil, nil, err
		}
		root = installOf(snap.State.Map)
	}
	w, err := workspace(root)
	if err != nil {
		return nil, nil, err
	}
	out := listMaterials(w, root, a)
	switch {
	case len(out.Materials) == 0:
		return out, nil, nil
	case a.Board:
		return showMaterials(ctx, c, snap, root, out)
	case a.Show:
		return sheetMaterials(w, out, a.Size)
	}
	return out, nil, nil
}

// listMaterials filters the catalog, most used first, and sizes each tile.
func listMaterials(w *gdt.Workspace, root string, a mapMaterialsArgs) *mapMaterialsOut {
	uses := tileUses(root)
	var found []gdt.EditorMaterial
	for _, m := range w.EditorMaterials() {
		if (a.All || !m.DoNotUse) && anyOf(m.Usage, a.Usage) && anyOf(m.Surface, a.Surface) && nameOf(m.Name, a.Name) {
			found = append(found, m)
		}
	}
	slices.SortFunc(found, func(x, y gdt.EditorMaterial) int {
		return cmp.Or(cmp.Compare(uses[y.Name].Faces, uses[x.Name].Faces), cmp.Compare(x.Name, y.Name))
	})
	limit := a.Limit
	if limit == 0 {
		limit = materialsLimit
	}
	out := &mapMaterialsOut{Found: len(found), Materials: []materialOut{}}
	tiles := tilesIn(root)
	for _, m := range found[:min(limit, len(found))] {
		t, err := tiles(m.Name)
		if err != nil {
			t = level.Tile{W: 64, H: 64}
		}
		out.Materials = append(out.Materials, materialOut{Name: m.Name, Usage: m.Usage, Surface: m.Surface,
			Tile: [2]float64{t.W, t.H}, Faces: uses[m.Name].Faces, DoNotUse: m.DoNotUse, color: m.ColorMap})
	}
	return out
}

// showMaterials lays the first materials listed out on a board in Radiant.
func showMaterials(ctx context.Context, c *radiant.Client, snap *level.Snapshot, root string, out *mapMaterialsOut) (*mapMaterialsOut, []byte, error) {
	var names []string
	for _, m := range out.Materials[:min(len(out.Materials), shownLimit)] {
		names = append(names, m.Name)
	}
	b, err := level.ShowMaterials(ctx, c, snap, names, tilesIn(root), 0)
	if err != nil {
		return nil, nil, err
	}
	png, err := level.PNG(b.Shot.Image)
	if err != nil {
		return nil, nil, err
	}
	out.W, out.H, out.Notes = b.Shot.Image.Rect.Dx(), b.Shot.Image.Rect.Dy(), b.Shot.Notes
	at := map[string][2]int{}
	for _, l := range b.Shot.Labels {
		at[l.ID] = [2]int{l.X, l.Y}
	}
	for _, s := range b.Swatches {
		out.Board = append(out.Board, swatchOut{Label: s.Label, Material: s.Name, At: at[s.Label]})
	}
	if len(out.Materials) > len(names) {
		out.Notes = append(out.Notes, fmt.Sprintf("the board shows the first %d", len(names)))
	}
	return out, png, nil
}

// shownLimit is how many materials a sheet or a board shows.
const shownLimit = 16

// sheetMaterials lays the first materials' color images out as Radiant's
// texture browser shows them, read from the install's texture files: no
// Radiant, nothing added to the map. The paths are resolved one by one (the
// workspace caches as it reads), the images decoded at once.
func sheetMaterials(w *gdt.Workspace, out *mapMaterialsOut, size int) (*mapMaterialsOut, []byte, error) {
	shown := out.Materials[:min(len(out.Materials), shownLimit)]
	cell := cmp.Or(size, level.DefaultSize) / min(len(shown), 4)
	paths := make([]string, len(shown))
	errs := make([]error, len(shown))
	for i, m := range shown {
		if m.color == "" {
			errs[i] = fmt.Errorf("no colorMap")
			continue
		}
		paths[i], errs[i] = w.ImageFile(m.color)
	}
	thumbs := make([]*image.RGBA, len(shown))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4) // a 2048² texture takes 16 MB decoded
	for i := range shown {
		if errs[i] != nil {
			continue
		}
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			thumbs[i], errs[i] = level.Thumb(paths[i], cell-2)
		})
	}
	wg.Wait()
	for i, m := range shown {
		out.Sheet = append(out.Sheet, m.Name)
		if errs[i] != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("%d, %s: %v", i+1, m.Name, errs[i]))
		}
	}
	img := level.Sheet(thumbs, cell)
	png, err := level.PNG(img)
	if err != nil {
		return nil, nil, err
	}
	out.W, out.H = img.Rect.Dx(), img.Rect.Dy()
	if len(out.Materials) > len(shown) {
		out.Notes = append(out.Notes, fmt.Sprintf("the sheet shows the first %d", len(shown)))
	}
	return out, png, nil
}

// anyOf tells whether a field holds one of a comma-separated list of texts,
// ignoring case; an empty list takes any.
func anyOf(field, list string) bool {
	if strings.TrimSpace(list) == "" {
		return true
	}
	for _, alt := range strings.Split(list, ",") {
		if alt = strings.TrimSpace(alt); alt != "" && strings.Contains(strings.ToLower(field), strings.ToLower(alt)) {
			return true
		}
	}
	return false
}

// nameOf tells whether a name holds every word of one of a comma-separated
// list of word groups, ignoring case ("brick red, plaster" takes
// t7_brick_worn_dirty_red_wet and any plaster).
func nameOf(name, list string) bool {
	if strings.TrimSpace(list) == "" {
		return true
	}
	name = strings.ToLower(name)
	for _, group := range strings.Split(list, ",") {
		words := strings.FieldsFunc(strings.ToLower(group), func(r rune) bool { return r == ' ' || r == '_' })
		if len(words) > 0 && !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(name, w) }) {
			return true
		}
	}
	return false
}
