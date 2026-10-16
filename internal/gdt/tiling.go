package gdt

import (
	"fmt"
	"strconv"
	"strings"
)

// Tiling is the texels one repeat of a material spans, across and down.
type Tiling struct {
	W, H int
	From string // "tiling", "images", or both when each axis has its own source
}

// MaterialTiling reads a material's tiling from its GDT: its tilingWidth and
// tilingHeight, or, on an axis left <auto>, the largest of its images' sizes
// on that axis after each image's mipBase (APE's rule, from material.awi's
// tooltip; a material and its tiling agree that way in the install, as
// t7_plastic_ceiling_corrugated_panel's 256×1024 tiling over a 512×2048 image
// at mipBase 1/2).
func (w *Workspace) MaterialTiling(name string) (Tiling, error) {
	fields, err := w.resolvedTyped(name, "material")
	if err != nil {
		return Tiling{}, err
	}
	t := Tiling{W: tilingSize(field(fields, "tilingWidth")), H: tilingSize(field(fields, "tilingHeight"))}
	from := map[string]bool{}
	if t.W > 0 || t.H > 0 {
		from["tiling"] = true
	}
	if t.W == 0 || t.H == 0 {
		iw, ih := w.largestImage(fields)
		if t.W == 0 {
			t.W = iw
		}
		if t.H == 0 {
			t.H = ih
		}
		from["images"] = true
	}
	if t.W == 0 || t.H == 0 {
		return Tiling{}, fmt.Errorf("material %q: tiling <auto> and no image of it on disk to size it", name)
	}
	switch {
	case from["tiling"] && from["images"]:
		t.From = "tiling and images"
	case from["tiling"]:
		t.From = "tiling"
	default:
		t.From = "images"
	}
	return t, nil
}

// largestImage is the largest width and height among a material's images,
// each image's texels after its mipBase; images whose texture can't be read
// are left out.
func (w *Workspace) largestImage(fields []Field) (int, int) {
	var mw, mh int
	for _, r := range w.Refs("material", fields) {
		if r.Type != "image" || strings.HasPrefix(r.Target, "$") {
			continue
		}
		iw, ih, err := w.imageTexels(r.Target)
		if err != nil {
			continue
		}
		mw, mh = max(mw, iw), max(mh, ih)
	}
	return mw, mh
}

// imageTexels is an image asset's size as the game loads it: its texture's
// pixels divided by its mipBase.
func (w *Workspace) imageTexels(name string) (int, int, error) {
	path, fields, err := w.imageFile(name)
	if err != nil {
		return 0, 0, err
	}
	iw, ih, err := imageSize(path)
	if err != nil {
		return 0, 0, err
	}
	d := 1
	if n, ok := strings.CutPrefix(field(fields, "mipBase"), "1/"); ok {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			d = v
		}
	}
	return max(iw/d, 1), max(ih/d, 1), nil
}

// ImageFile is an image asset's texture on disk, its baseImage: what Radiant's
// Textures pane shows a material by, through its colorMap.
func (w *Workspace) ImageFile(name string) (string, error) {
	path, _, err := w.imageFile(name)
	return path, err
}

func (w *Workspace) imageFile(name string) (string, []Field, error) {
	fields, err := w.resolvedTyped(name, "image")
	if err != nil {
		return "", nil, err
	}
	base := field(fields, "baseImage")
	if base == "" {
		return "", nil, fmt.Errorf("image %q has no baseImage", name)
	}
	return w.Abs(strings.ReplaceAll(base, `\`, "/")), fields, nil
}

// resolvedTyped is the effective fields of the first definition of an asset
// of that type.
func (w *Workspace) resolvedTyped(name, typ string) ([]Field, error) {
	locs, err := w.FindTyped(name, typ)
	if err != nil {
		return nil, err
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("no GDT defines %s %q", typ, name)
	}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return nil, err
	}
	a := f.AtLine(locs[0].Line)
	if a == nil {
		return nil, fmt.Errorf("%s %q: no asset at %s:%d", typ, name, locs[0].File, locs[0].Line)
	}
	_, fields, err := w.Resolved(f, a)
	return fields, err
}

// field is a field's unquoted value, "" when the asset has none.
func field(fields []Field, key string) string {
	for _, f := range fields {
		if f.Key == key {
			return Unquote(f.Value)
		}
	}
	return ""
}

// tilingSize is a tilingWidth or tilingHeight in texels, 0 for <auto>.
func tilingSize(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
