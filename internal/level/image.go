package level

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

// glyphs are 3×5 hex digits, a row a byte, its three low bits the pixels
// left to right: ids are hex.
var glyphs = map[rune][5]uint8{
	'0': {7, 5, 5, 5, 7}, '1': {2, 6, 2, 2, 7}, '2': {7, 1, 7, 4, 7}, '3': {7, 1, 3, 1, 7},
	'4': {5, 5, 7, 1, 1}, '5': {7, 4, 7, 1, 7}, '6': {7, 4, 7, 5, 7}, '7': {7, 1, 1, 2, 2},
	'8': {7, 5, 7, 5, 7}, '9': {7, 5, 7, 1, 7}, 'a': {2, 5, 7, 5, 5}, 'b': {6, 5, 6, 5, 6},
	'c': {3, 4, 4, 4, 3}, 'd': {6, 5, 5, 5, 6}, 'e': {7, 4, 6, 4, 7}, 'f': {7, 4, 6, 4, 4},
}

// px is the scale the glyphs are drawn at: a 3×5 glyph is 6×10 pixels.
const px = 2

var (
	ink    = color.RGBA{255, 255, 255, 255}
	plate  = color.RGBA{0, 0, 0, 255}
	marker = color.RGBA{255, 210, 0, 255}
)

// DrawLabels marks each label's point and writes its id beside it, white on
// a black plate, kept inside the image.
func DrawLabels(img *image.RGBA, labels []Label) {
	b := img.Rect
	for _, l := range labels {
		fill(img, image.Rect(l.X-2, l.Y-2, l.X+3, l.Y+3), marker)
		tw, th := len(l.ID)*4*px-px, 5*px
		x := min(max(l.X+5, b.Min.X+2), b.Max.X-tw-2)
		y := min(max(l.Y-th-4, b.Min.Y+2), b.Max.Y-th-2)
		fill(img, image.Rect(x-2, y-2, x+tw+2, y+th+2), plate)
		for i, r := range l.ID {
			g, ok := glyphs[r]
			if !ok {
				continue
			}
			for row, bits := range g {
				for col := range 3 {
					if bits&(4>>col) != 0 {
						gx, gy := x+i*4*px+col*px, y+row*px
						fill(img, image.Rect(gx, gy, gx+px, gy+px), ink)
					}
				}
			}
		}
	}
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
