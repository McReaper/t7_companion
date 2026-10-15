package level

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png" // the install's textures are TIFF and PNG
	"os"
	"path/filepath"
	"strconv"

	_ "golang.org/x/image/tiff"
)

// sheetCols is how many thumbnails a sheet has across.
const sheetCols = 4

var (
	sheetGap   = color.RGBA{24, 24, 24, 255}
	sheetEmpty = color.RGBA{96, 96, 96, 255}
)

// Thumb reads a texture file, TIFF or PNG, and shrinks it to fit a square of
// side pixels.
func Thumb(path string, side int) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	b := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Rect, src, b.Min, draw.Src)
	w, h := Fit(b.Dx(), b.Dy(), side)
	return Shrink(rgba, w, h), nil
}

// Sheet lays thumbnails out as Radiant's texture browser does, sheetCols
// across in squares of cell pixels, each centred in its square and numbered
// from 1 in its upper-left corner; a nil one leaves its square grey.
func Sheet(thumbs []*image.RGBA, cell int) *image.RGBA {
	cols := min(len(thumbs), sheetCols)
	rows := (len(thumbs) + sheetCols - 1) / sheetCols
	img := image.NewRGBA(image.Rect(0, 0, cols*cell, rows*cell))
	fill(img, img.Rect, sheetGap)
	for i, t := range thumbs {
		x, y := i%sheetCols*cell, i/sheetCols*cell
		if t == nil {
			fill(img, image.Rect(x+1, y+1, x+cell-1, y+cell-1), sheetEmpty)
		} else {
			w, h := t.Rect.Dx(), t.Rect.Dy()
			at := image.Pt(x+(cell-w)/2, y+(cell-h)/2)
			draw.Draw(img, image.Rectangle{at, at.Add(image.Pt(w, h))}, t, t.Rect.Min, draw.Src)
		}
		writeText(img, x+4, y+4, strconv.Itoa(i+1))
	}
	return img
}
