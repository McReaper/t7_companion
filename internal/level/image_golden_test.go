package level

import (
	"flag"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
)

// art draws an image one character a pixel: # ink, . plate, o marker, space
// anything else.
func art(img *image.RGBA) string {
	var s strings.Builder
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			switch img.RGBAAt(x, y) {
			case ink:
				s.WriteByte('#')
			case plate:
				s.WriteByte('.')
			case marker:
				s.WriteByte('o')
			default:
				s.WriteByte(' ')
			}
		}
		s.WriteString("|\n")
	}
	return s.String()
}

var update = flag.Bool("update", false, "rewrite testdata/labels.golden from the drawing")

// Five labels on one point take its four places, above right, below right,
// above left, below left, and the fifth its marker only; a sixth at the
// corner is pushed inside the image. testdata/labels.golden was read glyph
// by glyph when written.
func TestLabelsAreDrawnWhereTheyShould(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 44))
	DrawLabels(img, []Label{{"a1", 30, 21}, {"b2", 30, 21}, {"c3", 30, 21}, {"d4", 30, 21}, {"e5", 30, 21}, {"f", 63, 0}})
	got := art(img)
	if *update {
		if err := os.WriteFile("testdata/labels.golden", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/labels.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("labels drawn:\n%s\nwant:\n%s", got, want)
	}
}

func TestFromBGRAReadsEveryPixel(t *testing.T) {
	b := []byte{1, 2, 3, 0, 4, 5, 6, 0, 7, 8, 9, 0, 10, 11, 12, 0}
	img, err := FromBGRA(b, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []color.RGBA{{3, 2, 1, 255}, {6, 5, 4, 255}, {9, 8, 7, 255}, {12, 11, 10, 255}} {
		if c := img.RGBAAt(i%2, i/2); c != want {
			t.Errorf("pixel %d = %v, want %v", i, c, want)
		}
	}
	if _, err := FromBGRA(b[:15], 2, 2); err == nil || err.Error() != "a 2×2 capture needs 16 bytes of pixels, the bridge sent 15" {
		t.Errorf("short: %v", err)
	}
	if _, err := FromBGRA(b, 2, 0); err == nil {
		t.Error("a frame of no height")
	}
}

func TestShrinkTakesTheMeanUnderEachPixel(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 3))
	for y := range 3 {
		for x := range 5 {
			src.SetRGBA(x, y, color.RGBA{uint8(10 * x), uint8(10 * y), uint8(x + 5*y), 255})
		}
	}
	dst := Shrink(src, 2, 2)
	// columns 0-1 and 2-4, rows 0 and 1-2 (x*5/2, y*3/2)
	want := [4]color.RGBA{
		{5, 0, 0, 255},    // x 0-1, y 0: blue (0+1)/2
		{30, 0, 3, 255},   // x 2-4, y 0: blue (2+3+4)/3
		{5, 15, 8, 255},   // x 0-1, y 1-2: blue (5+6+10+11)/4
		{30, 15, 10, 255}, // x 2-4, y 1-2: blue (7+8+9+12+13+14)/6
	}
	for i, w := range want {
		if c := dst.RGBAAt(i%2, i/2); c != w {
			t.Errorf("pixel %d = %v, want %v", i, c, w)
		}
	}
}
