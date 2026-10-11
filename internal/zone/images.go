package zone

import "github.com/McReaper/t7_companion/internal/gdt"

// Images are most of a map's upload, and the part a modder can shrink without
// cutting content: an image's mipBase (1/1, 1/2, 1/4, 1/8) halves its width
// and height per step. Image data doesn't compress in the .xpak, so the
// streamed bytes the report gives are what the upload carries; a step leaves
// a quarter of them, each streamed part rounded up to 64 KiB.

// MipSteps are mipBase's values, full size first.
var MipSteps = []string{"1/1", "1/2", "1/4", "1/8"}

const streamPart = 64 << 10

// Owner says who defines a packed image.
type Owner string

const (
	Yours Owner = "yours"  // a non-stock GDT: its mipBase is the user's to change
	Stock Owner = "stock"  // a stock GDT
	NoGDT Owner = "no_gdt" // none: an image the link makes, or an engine one
)

// Image is a packed image a non-stock GDT defines.
type Image struct {
	Packed
	Loc     gdt.Location // its first definition
	Also    int          // further definitions
	MipBase string
	Locked  bool // doNotResize: it keeps its size
}

// StepSaves estimates what one mipBase step down takes off its streamed bytes.
func (im Image) StepSaves() int64 {
	if im.Locked || im.MipBase == MipSteps[len(MipSteps)-1] {
		return 0
	}
	after := (im.Streamed/4 + streamPart - 1) / streamPart * streamPart
	return max(0, im.Streamed-after)
}

// ClassifyImage finds who defines a packed image in w and, when it is the
// user's, its mipBase and doNotResize.
func ClassifyImage(w *gdt.Workspace, p Packed) (Image, Owner) {
	locs, err := w.FindTyped(p.Name, "image")
	if err != nil || len(locs) == 0 {
		return Image{}, NoGDT
	}
	if locs[0].Stock {
		return Image{}, Stock
	}
	im := Image{Packed: p, Loc: locs[0], Also: len(locs) - 1, MipBase: MipSteps[0]}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return im, Yours
	}
	for _, a := range f.Assets {
		if a.Name != p.Name {
			continue
		}
		if _, fields, err := w.Resolved(f, a); err == nil {
			for _, fl := range fields {
				switch {
				case fl.Key == "mipBase" && fl.Value != "":
					im.MipBase = fl.Value
				case fl.Key == "doNotResize":
					im.Locked = fl.Value == "1"
				}
			}
		}
		break
	}
	return im, Yours
}
