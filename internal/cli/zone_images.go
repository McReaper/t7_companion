package cli

import (
	"fmt"
	"sort"

	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// Images are most of a map's upload, and the part a modder can shrink without
// cutting content: an image's mipBase (1/1, 1/2, 1/4, 1/8) halves its width
// and height per step. Image data doesn't compress in the .xpak, so the
// streamed bytes the report gives are what the upload carries; a step leaves
// a quarter of them, each streamed part rounded up to 64 KiB.

const (
	imagesMaxGDTs    = 10
	imagesMaxLargest = 10
	streamPart       = 64 << 10
)

var mipSteps = []string{"1/1", "1/2", "1/4", "1/8"}

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

// yourImage is one image a non-stock GDT defines, as the build packed it.
type yourImage struct {
	zone.Packed
	loc     gdt.Location
	also    int
	mipBase string
	locked  bool
}

// saves estimates what one mipBase step down takes off its streamed bytes.
func (im yourImage) saves() int64 {
	if im.locked || im.mipBase == mipSteps[len(mipSteps)-1] {
		return 0
	}
	after := (im.Streamed/4 + streamPart - 1) / streamPart * streamPart
	return max(0, im.Streamed-after)
}

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
	var yours []yourImage
	for _, p := range assets {
		if p.Type != "image" {
			continue
		}
		im, group := classifyImage(w, p)
		g := map[string]*imageGroup{"yours": &res.Yours, "stock": &res.Stock, "no_gdt": &res.NoGDT}[group]
		g.Images++
		g.Bytes += size(p)
		if group == "yours" {
			yours = append(yours, im)
		}
	}
	res.ByGDT, res.MoreGDTs, res.StepSaves = imagesByGDT(yours)
	res.Largest = largestImages(yours)
	return res, nil
}

// classifyImage finds who defines an image: a non-stock GDT (with its mipBase),
// a stock one, or none (images the link makes, or engine ones).
func classifyImage(w *gdt.Workspace, p zone.Packed) (yourImage, string) {
	locs, err := w.FindTyped(p.Name, "image")
	if err != nil || len(locs) == 0 {
		return yourImage{}, "no_gdt"
	}
	if locs[0].Stock {
		return yourImage{}, "stock"
	}
	im := yourImage{Packed: p, loc: locs[0], also: len(locs) - 1, mipBase: mipSteps[0]}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return im, "yours"
	}
	for _, a := range f.Assets {
		if a.Name != p.Name {
			continue
		}
		if _, fields, err := w.Resolved(f, a); err == nil {
			for _, fl := range fields {
				switch {
				case fl.Key == "mipBase" && fl.Value != "":
					im.mipBase = fl.Value
				case fl.Key == "doNotResize":
					im.locked = fl.Value == "1"
				}
			}
		}
		break
	}
	return im, "yours"
}

func imagesByGDT(yours []yourImage) ([]imageGDTOut, int, int64) {
	by := map[string]*imageGDTOut{}
	var total int64
	for _, im := range yours {
		o := by[im.loc.File]
		if o == nil {
			o = &imageGDTOut{GDT: im.loc.File}
			by[im.loc.File] = o
		}
		o.Images++
		o.Bytes += size(im.Packed)
		o.StepSaves += im.saves()
		total += im.saves()
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
func largestImages(yours []yourImage) []imageOut {
	sort.Slice(yours, func(i, j int) bool {
		if a, b := size(yours[i].Packed), size(yours[j].Packed); a != b {
			return a > b
		}
		return yours[i].Name < yours[j].Name
	})
	var out []imageOut
	for _, im := range yours {
		if im.saves() == 0 {
			continue
		}
		out = append(out, imageOut{Image: im.Name, Bytes: size(im.Packed), MipBase: im.mipBase, GDT: im.loc.File,
			AlsoIn: im.also, Line: im.Line().String()})
		if len(out) == imagesMaxLargest {
			break
		}
	}
	return out
}
