package cli

import (
	"fmt"
	"sort"

	"github.com/McReaper/t7_companion/internal/zone"
)

// The images' share of the upload, and what a mipBase step down saves
// (zone.Image says how it is estimated).

const (
	imagesMaxGDTs    = 10
	imagesMaxLargest = 10
)

type imageGroup struct {
	Images int   `json:"images"`
	Bytes  int64 `json:"bytes"`
}

type imageGDTOut struct {
	GDT string `json:"gdt"`
	imageGroup
	StepSaves int64 `json:"step_down_saves,omitempty"`
}

type imageOut struct {
	Image   string `json:"image"`
	Bytes   int64  `json:"bytes"`
	MipBase string `json:"mipBase"`
	GDT     string `json:"gdt"`
	AlsoIn  int    `json:"also_defined_in,omitempty"`
	Line    string `json:"line"`
}

type zoneImagesResult struct {
	Map string `json:"map"`
	zoneStatus
	Line      string        `json:"line,omitempty"`
	Yours     imageGroup    `json:"yours"`
	Stock     imageGroup    `json:"stock"`
	NoGDT     imageGroup    `json:"no_gdt"`
	StepSaves int64         `json:"step_down_saves"`
	ByGDT     []imageGDTOut `json:"by_gdt,omitempty"`
	MoreGDTs  int           `json:"more_gdts,omitempty"`
	Largest   []imageOut    `json:"largest,omitempty"`
	Advice    string        `json:"advice"`
}

const imagesAdvice = "step_down_saves estimates what one mipBase step down (1/1 -> 1/2 -> 1/4 -> 1/8, half the width " +
	"and height) on your images takes off the upload: image data doesn't compress in the .xpak, and a step leaves about " +
	"a quarter of an image's streamed bytes. largest lists those a step can still shrink. Set mipBase with gdt_edit " +
	"(a batch per GDT), relink, and look at them in game. Stock images aren't editable: what pulls them in is the lever. " +
	"An image with doNotResize set keeps its size, so it isn't counted."

// zoneImages sorts the images of a map's last link (or of one zone line) by
// who defines them, and says what a mipBase step down on yours would save.
func zoneImages(toolsPath, name, line string) (*zoneImagesResult, error) {
	z, err := openZone(toolsPath, name)
	if err != nil {
		return nil, err
	}
	assets := z.report.Assets
	res := &zoneImagesResult{Map: name, zoneStatus: z.status, Advice: imagesAdvice}
	if line != "" {
		ref := parseRef(line)
		if assets = z.report.Under(ref); len(assets) == 0 {
			return nil, fmt.Errorf("nothing in %s's last link comes from %q (try the asset name alone, or a .zpkg name)", name, line)
		}
		res.Line = ref.String()
	}
	w, err := workspace(toolsPath)
	if err != nil {
		return nil, err
	}
	var yours []zone.Image
	for _, p := range assets {
		if p.Type != "image" {
			continue
		}
		im, owner := zone.ClassifyImage(w, p)
		g := map[zone.Owner]*imageGroup{zone.Yours: &res.Yours, zone.Stock: &res.Stock, zone.NoGDT: &res.NoGDT}[owner]
		g.Images++
		g.Bytes += size(p)
		if owner == zone.Yours {
			yours = append(yours, im)
		}
	}
	res.ByGDT, res.MoreGDTs, res.StepSaves = imagesByGDT(yours)
	res.Largest = largestImages(yours)
	return res, nil
}

func imagesByGDT(yours []zone.Image) ([]imageGDTOut, int, int64) {
	by := map[string]*imageGDTOut{}
	var total int64
	for _, im := range yours {
		o := by[im.Loc.File]
		if o == nil {
			o = &imageGDTOut{GDT: im.Loc.File}
			by[im.Loc.File] = o
		}
		o.Images++
		o.Bytes += size(im.Packed)
		o.StepSaves += im.StepSaves()
		total += im.StepSaves()
	}
	out := make([]imageGDTOut, 0, len(by))
	for _, o := range by {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StepSaves != out[j].StepSaves {
			return out[i].StepSaves > out[j].StepSaves
		}
		return out[i].GDT < out[j].GDT
	})
	if len(out) > imagesMaxGDTs {
		return out[:imagesMaxGDTs], len(out) - imagesMaxGDTs, total
	}
	return out, 0, total
}

// largestImages lists the biggest of your images a step can still shrink.
func largestImages(yours []zone.Image) []imageOut {
	sort.Slice(yours, func(i, j int) bool {
		if a, b := size(yours[i].Packed), size(yours[j].Packed); a != b {
			return a > b
		}
		return yours[i].Name < yours[j].Name
	})
	var out []imageOut
	for _, im := range yours {
		if im.StepSaves() == 0 {
			continue
		}
		out = append(out, imageOut{Image: im.Name, Bytes: size(im.Packed), MipBase: im.MipBase, GDT: im.Loc.File,
			AlsoIn: im.Also, Line: im.Line().String()})
		if len(out) == imagesMaxLargest {
			break
		}
	}
	return out
}
