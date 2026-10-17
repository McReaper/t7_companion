package cli

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/tiff"
)

// show answers a sheet of the materials' color images read from the install:
// no Radiant is asked for, and a material whose image can't be had keeps its
// number, grey, with a note saying why.
func TestMapMaterialsShowsASheetOfTheirColorImages(t *testing.T) {
	root := materialsInstall(t)
	solid := func(w, h int, c color.RGBA) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return img
	}
	put := func(rel string, b []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var red, blue bytes.Buffer
	if err := png.Encode(&red, solid(400, 400, color.RGBA{200, 10, 10, 255})); err != nil { // larger than its square, as textures are
		t.Fatal(err)
	}
	if err := tiff.Encode(&blue, solid(32, 64, color.RGBA{10, 10, 200, 255}), nil); err != nil {
		t.Fatal(err)
	}
	put("texture_assets/t/red.png", red.Bytes())
	put("texture_assets/t/blue.tif", blue.Bytes())
	put("texture_assets/t/bad.png", []byte("not an image"))
	mat := func(name, colorMap string) string {
		s := "\t\"" + name + "\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"decal\"\r\n"
		if colorMap != "" {
			s += "\t\t\"colorMap\" \"" + colorMap + "\"\r\n"
		}
		return s + "\t}\r\n"
	}
	img := func(name, file string) string {
		return "\t\"" + name + "\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"baseImage\" \"texture_assets\\\\t\\\\" + file + "\"\r\n\t}\r\n"
	}
	put("source_data/sheet.gdt", []byte("{\r\n"+mat("s_bad", "i_bad")+mat("s_gone", "i_gone")+mat("s_nomap", "")+
		mat("s_red", "i_red")+mat("s_tif", "i_tif")+img("i_bad", "bad.png")+img("i_gone", "gone.png")+
		img("i_red", "red.png")+img("i_tif", "blue.tif")+"}\r\n"))
	t.Setenv("TA_TOOLS_PATH", root)
	onMachine(t) // no Radiant runs: show must not need one

	out, b, err := mapMaterials(context.Background(), mapMaterialsArgs{Usage: "decal", Show: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.Sheet, " ") != "s_bad s_gone s_nomap s_red s_tif" || out.W != 768 || out.H != 384 || out.Board != nil {
		t.Fatalf("answer %+v", out)
	}
	notes := strings.Join(out.Notes, "\n")
	for _, want := range []string{"1, s_bad: bad.png: image: unknown format", "2, s_gone: open ", "3, s_nomap: no colorMap"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q lack %q", notes, want)
		}
	}
	if len(out.Notes) != 3 {
		t.Errorf("notes %q: one per image that can't be had", out.Notes)
	}
	sheet, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{96, 96, color.RGBA{96, 96, 96, 255}},          // 1, grey: no image
		{3*192 + 96, 96, color.RGBA{200, 10, 10, 255}}, // 4, the PNG
		{96, 192 + 96, color.RGBA{10, 10, 200, 255}},   // 5, the TIFF, half as wide, centred
		{20, 192 + 96, color.RGBA{24, 24, 24, 255}},    // beside it, the sheet's ground
		{767, 96, color.RGBA{24, 24, 24, 255}},         // the PNG's square stops a pixel short of its cell's edge
		{766, 96, color.RGBA{200, 10, 10, 255}},
		{2, 2, color.RGBA{0, 0, 0, 255}}, // the number's plate
	} {
		if got := color.RGBAModel.Convert(sheet.At(c.x, c.y)).(color.RGBA); got != c.want {
			t.Errorf("at %d,%d: %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// a smaller image, and one material: one square of the whole size
	one, _, err := mapMaterials(context.Background(), mapMaterialsArgs{Name: "s_red", Show: true, Size: 300, Usage: "decal"})
	if err != nil || one.W != 300 || one.H != 300 || len(one.Notes) != 0 {
		t.Errorf("%+v, %v", one, err)
	}
}
