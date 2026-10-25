package picture

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// FromBGRA is a capture's pixels as an image: BGRA, top row first.
func FromBGRA(b []byte, w, h int) (*image.RGBA, error) {
	if w <= 0 || h <= 0 || len(b) < w*h*4 {
		return nil, fmt.Errorf("a %d×%d capture needs %d bytes of pixels, the bridge sent %d", w, h, w*h*4, len(b))
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h*4; i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = b[i+2], b[i+1], b[i], 255
	}
	return img, nil
}

// Fit is the size an image takes with its longer side at most max, its
// aspect kept; an image already small enough keeps its size.
func Fit(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	if w >= h {
		return max, int(math.Max(1, math.Round(float64(h)*float64(max)/float64(w))))
	}
	return int(math.Max(1, math.Round(float64(w)*float64(max)/float64(h)))), max
}

// Shrink scales the image down to w×h, each pixel the mean of the source
// pixels under it, so thin lines and text fade rather than vanish.
func Shrink(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if w >= sw && h >= sh {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, n int
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					r, g, b, n = r+int(row[sx*4]), g+int(row[sx*4+1]), b+int(row[sx*4+2]), n+1
				}
			}
			i := y*dst.Stride + x*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}

// Label is an id drawn where its point landed in the image.
type Label struct {
	ID   string
	X, Y int
}

// glyphs are 5×7 hex digits, a row a byte, its five low bits the pixels
// left to right: ids are hex. They are the HD44780 LCD's, lower case: b and
// d have ascenders and 0 a slash, so none reads as 8, 0 or 6.
var glyphs = map[rune][glyphH]uint8{
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E}, '1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F}, '3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02}, '5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E}, '7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E}, '9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'a': {0x00, 0x00, 0x0E, 0x01, 0x0F, 0x11, 0x0F}, 'b': {0x10, 0x10, 0x16, 0x19, 0x11, 0x11, 0x1E},
	'c': {0x00, 0x00, 0x0E, 0x10, 0x10, 0x11, 0x0E}, 'd': {0x01, 0x01, 0x0D, 0x13, 0x11, 0x11, 0x0F},
	'e': {0x00, 0x00, 0x0E, 0x11, 0x1F, 0x10, 0x0E}, 'f': {0x06, 0x09, 0x08, 0x1C, 0x08, 0x08, 0x08},
}

const (
	glyphW, glyphH = 5, 7
	// px is the scale the glyphs are drawn at: a glyph is 10×14 pixels, 12
	// apart, a size an agent reads every id at on a 768-pixel image.
	px = 2
)

// textSize is the pixels text takes, its plate's margin left out.
func textSize(text string) (int, int) {
	return len(text)*(glyphW+1)*px - px, glyphH * px
}

var (
	ink    = color.RGBA{255, 255, 255, 255}
	plate  = color.RGBA{0, 0, 0, 255}
	marker = color.RGBA{255, 210, 0, 255}
)

// DrawLabels marks each label's point and writes its id beside it, white on
// a black plate kept inside the image: above right of the point, else below
// right, above left or below left, wherever it overlaps no plate drawn
// before and no marker, so a marker never hides a digit nor a plate a point;
// a label with no room left gets its marker only.
func DrawLabels(img *image.RGBA, labels []Label) {
	taken := make([]image.Rectangle, len(labels))
	for i, l := range labels {
		taken[i] = image.Rect(l.X-2, l.Y-2, l.X+3, l.Y+3)
		fill(img, taken[i], marker)
	}
	for _, l := range labels {
		x, y, ok := placeLabel(img.Rect, l, taken)
		if !ok {
			continue
		}
		taken = append(taken, writeText(img, x, y, l.ID))
	}
}

// writeText writes hex digits white on a black plate, the text's upper left
// at x, y, and returns the plate.
func writeText(img *image.RGBA, x, y int, text string) image.Rectangle {
	tw, th := textSize(text)
	r := image.Rect(x-2, y-2, x+tw+2, y+th+2)
	fill(img, r, plate)
	for i, c := range text {
		g, ok := glyphs[c]
		if !ok {
			continue
		}
		for row, bits := range g {
			for col := range glyphW {
				if bits&(1<<(glyphW-1-col)) != 0 {
					gx, gy := x+i*(glyphW+1)*px+col*px, y+row*px
					fill(img, image.Rect(gx, gy, gx+px, gy+px), ink)
				}
			}
		}
	}
	return r
}

// placeLabel is where a label's text goes: the first of its four places that
// overlaps none of the rectangles taken.
func placeLabel(b image.Rectangle, l Label, taken []image.Rectangle) (int, int, bool) {
	tw, th := textSize(l.ID)
	for _, at := range [4][2]int{{l.X + 5, l.Y - th - 4}, {l.X + 5, l.Y + 5}, {l.X - tw - 5, l.Y - th - 4}, {l.X - tw - 5, l.Y + 5}} {
		x := min(max(at[0], b.Min.X+2), b.Max.X-tw-2)
		y := min(max(at[1], b.Min.Y+2), b.Max.Y-th-2)
		r := image.Rect(x-2, y-2, x+tw+2, y+th+2)
		free := true
		for _, p := range taken {
			if r.Overlaps(p) {
				free = false
				break
			}
		}
		if free {
			return x, y, true
		}
	}
	return 0, 0, false
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// PNG encodes the image, fast: an agent's view is small.
func PNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
