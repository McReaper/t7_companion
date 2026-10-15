package level

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThumbShrinksATextureToItsSquare(t *testing.T) {
	dir := t.TempDir()
	src := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 30, 160, 90, 255
	}
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "wide.png")
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	th, err := Thumb(p, 80)
	if err != nil || th.Rect.Dx() != 80 || th.Rect.Dy() != 20 || th.RGBAAt(40, 10) != (color.RGBA{30, 160, 90, 255}) {
		t.Fatalf("%v, %v", th.Rect, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.tif"), []byte("II*\x00junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Thumb(filepath.Join(dir, "x.tif"), 80); err == nil || !strings.HasPrefix(err.Error(), "x.tif: ") {
		t.Errorf("a broken TIFF: %v", err)
	}
	if _, err := Thumb(filepath.Join(dir, "none.png"), 80); err == nil {
		t.Error("a missing file: no error")
	}
}

func TestSheetIsAsWideAsItsThumbnailsUpToFour(t *testing.T) {
	tile := func(c color.RGBA) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, 30, 30))
		fill(img, img.Rect, c)
		return img
	}
	red, green := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}
	for n, want := range map[int][2]int{1: {32, 32}, 2: {64, 32}, 4: {128, 32}, 5: {128, 64}, 9: {128, 96}} {
		thumbs := make([]*image.RGBA, n)
		for i := range thumbs {
			thumbs[i] = tile(red)
		}
		thumbs[n-1] = tile(green)
		s := Sheet(thumbs, 32)
		if s.Rect.Dx() != want[0] || s.Rect.Dy() != want[1] {
			t.Errorf("%d thumbnails: %v, want %v", n, s.Rect.Size(), want)
		}
		last := image.Pt((n-1)%4*32+16, (n-1)/4*32+16)
		if s.RGBAAt(last.X, last.Y) != green || s.RGBAAt(31, 31) != sheetGap {
			t.Errorf("%d thumbnails: the last at %v is %v, the gap %v", n, last, s.RGBAAt(last.X, last.Y), s.RGBAAt(31, 31))
		}
	}
	s := Sheet([]*image.RGBA{nil, tile(red)}, 40)
	if s.RGBAAt(20, 20) != sheetEmpty || s.RGBAAt(60, 20) != red || s.RGBAAt(2, 2) != plate || s.RGBAAt(42, 2) != plate {
		t.Errorf("an empty square is grey, each numbered in its corner: %v %v %v %v", s.RGBAAt(20, 20), s.RGBAAt(60, 20), s.RGBAAt(2, 2), s.RGBAAt(42, 2))
	}
}
