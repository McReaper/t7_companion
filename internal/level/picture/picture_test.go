package picture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestFromBGRASwapsTheChannels(t *testing.T) {
	img, err := FromBGRA([]byte{1, 2, 3, 0, 4, 5, 6, 9}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c := img.RGBAAt(0, 0); c != (color.RGBA{3, 2, 1, 255}) {
		t.Errorf("first pixel = %v", c)
	}
	if c := img.RGBAAt(1, 0); c != (color.RGBA{6, 5, 4, 255}) {
		t.Errorf("second pixel = %v", c)
	}
	if _, err := FromBGRA(make([]byte, 7), 2, 1); err == nil {
		t.Error("too few bytes for the frame")
	}
	if _, err := FromBGRA(nil, 0, 1); err == nil {
		t.Error("a frame of no width")
	}
}

func TestFitKeepsTheAspect(t *testing.T) {
	for _, tc := range [][4]int{{1648, 869, 768, 405}, {693, 1165, 768, 457}, {300, 200, 768, 300}, {1000, 1, 100, 100}, {1, 1000, 100, 1}} {
		w, h := Fit(tc[0], tc[1], tc[2])
		want := [2]int{tc[2], tc[3]}
		if tc[0] < tc[1] {
			want = [2]int{tc[3], tc[2]}
		}
		if tc[0] <= tc[2] && tc[1] <= tc[2] {
			want = [2]int{tc[0], tc[1]}
		}
		if (tc[0] == 1000 && h != 1) || (tc[1] == 1000 && w != 1) {
			t.Errorf("Fit(%d, %d, %d) = %d×%d: a side never goes below 1", tc[0], tc[1], tc[2], w, h)
		} else if tc[0] != 1000 && tc[1] != 1000 && [2]int{w, h} != want {
			t.Errorf("Fit(%d, %d, %d) = %d×%d, want %v", tc[0], tc[1], tc[2], w, h, want)
		}
	}
}

func TestShrinkAveragesThePixelsUnderEach(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x := range 4 {
		for y := range 2 {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 40), uint8(y * 100), 7, 255})
		}
	}
	dst := Shrink(src, 2, 1)
	if dst.Rect.Dx() != 2 || dst.Rect.Dy() != 1 {
		t.Fatalf("size %v", dst.Rect)
	}
	if c := dst.RGBAAt(0, 0); c != (color.RGBA{20, 50, 7, 255}) {
		t.Errorf("left = %v, want the mean of x 0-1", c)
	}
	if c := dst.RGBAAt(1, 0); c != (color.RGBA{100, 50, 7, 255}) {
		t.Errorf("right = %v, want the mean of x 2-3", c)
	}
	if Shrink(src, 4, 2) != src || Shrink(src, 8, 8) != src {
		t.Error("an image is scaled up")
	}
	odd := Shrink(src, 3, 2) // 4 → 3 columns: one source pixel each at least
	if odd.Rect.Dx() != 3 || odd.RGBAAt(2, 1).A != 255 {
		t.Errorf("odd shrink %v", odd.Rect)
	}
}

func TestLabelsAreDrawnInsideTheImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	DrawLabels(img, []Label{{ID: "5ec8de49", X: 30, Y: 30}, {ID: "ffz0", X: 119, Y: 0}})
	if c := img.RGBAAt(30, 30); c != marker {
		t.Errorf("the point's marker = %v", c)
	}
	var white, black int
	for y := range 60 {
		for x := range 120 {
			switch img.RGBAAt(x, y) {
			case ink:
				white++
			case plate:
				black++
			}
		}
	}
	if white == 0 || black == 0 {
		t.Errorf("%d ink and %d plate pixels", white, black)
	}
	// a label at the corner is pushed inside: its plate starts no further than the edge allows
	if img.RGBAAt(119, 5) == (color.RGBA{}) && img.RGBAAt(118, 5) == (color.RGBA{}) && img.RGBAAt(110, 5) == (color.RGBA{}) {
		t.Error("the corner label is not drawn inside the image")
	}
}

func TestPNGDecodesBack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.SetRGBA(2, 1, color.RGBA{9, 8, 7, 255})
	b, err := PNG(img)
	if err != nil {
		t.Fatal(err)
	}
	back, err := png.Decode(bytes.NewReader(b))
	if err != nil || back.Bounds() != img.Rect {
		t.Fatalf("decode: %v %v", back, err)
	}
	if r, g, bl, _ := back.At(2, 1).RGBA(); r>>8 != 9 || g>>8 != 8 || bl>>8 != 7 {
		t.Errorf("pixel = %d %d %d", r>>8, g>>8, bl>>8)
	}
}

// Shrink gives each pixel the mean of the source pixels under it; along a
// side that grows instead, each pixel takes the one source row or column
// under it.
func TestShrinkMeansThePixelsUnderEach(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for x, v := range []uint8{0, 100, 200, 250} {
		src.Set(x, 0, color.RGBA{v, v, v, 255})
		src.Set(x, 1, color.RGBA{v / 10, v / 10, v / 10, 255})
	}
	tall := Shrink(src, 2, 4) // half as wide, twice as high
	for _, c := range []struct{ x, y, want int }{{0, 0, 50}, {1, 0, 225}, {0, 1, 50}, {0, 2, 5}, {1, 3, 22}} {
		if got := int(tall.RGBAAt(c.x, c.y).R); got != c.want {
			t.Errorf("2×4 at %d,%d: %d, want %d", c.x, c.y, got, c.want)
		}
	}
	wide := Shrink(src, 8, 1) // twice as wide, half as high
	for _, c := range []struct{ x, want int }{{0, 0}, {1, 0}, {2, 55}, {7, 137}} {
		if got := int(wide.RGBAAt(c.x, 0).R); got != c.want {
			t.Errorf("8×1 at %d: %d, want %d", c.x, got, c.want)
		}
	}
}
