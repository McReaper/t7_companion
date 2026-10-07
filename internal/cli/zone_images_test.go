package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// imagesRoot lays out a mod-tools root whose last link of zm_x packed images
// from GDTs of yours (one inheriting mipBase 1/2, one locked by doNotResize,
// one already at 1/8, two too small for a step to save anything, one with an
// empty mipBase, one defined in two GDTs), from a stock GDT, and from none.
func imagesRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir()) // keep the index caches out of the real profile
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("deffiles/image.awi", "\tAsset.AddEntry_Combo(\"mipBase\", \"1/1|1/2|1/4|1/8\");\n\tAsset.AddEntry_CheckBox(\"doNotResize\", false);\n")
	write("bin/converter_gdt_dirs_0.txt", "source_data\n")
	write("stock.gdtdef", "source_data/stock_imgs.gdt\n")
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", "Globals()\n{\n\tcategory = \"Geometry\"\n}\n")
	img := func(name, fields string) string {
		return "\t\"" + name + "\" ( \"image.gdf\" )\n\t{\n" + fields + "\t}\n"
	}
	write("source_data/mine.gdt", "{\n"+
		img("big_c", "\t\t\"mipBase\" \"1/1\"\n\t\t\"doNotResize\" \"0\"\n")+ // APE writes the unchecked box out
		img("base_img", "\t\t\"mipBase\" \"1/2\"\n")+
		"\t\"half_n\" [ \"base_img\" ]\n\t{\n\t\t\"baseImage\" \"x.tif\"\n\t}\n"+
		img("tiny", "")+
		img("eighth", "\t\t\"mipBase\" \"1/8\"\n")+
		img("locked", "\t\t\"doNotResize\" \"1\"\n")+
		img("blank_mip", "\t\t\"mipBase\" \"\"\n")+
		img("small", "")+
		"}\n")
	write("source_data/a_small.gdt", "{\n"+img("a_img", "")+"}\n")
	write("source_data/other.gdt", "{\n"+img("dup_c", "")+"}\n")
	write("source_data/other2.gdt", "{\n"+img("dup_c", "")+"}\n")
	write("source_data/stock_imgs.gdt", "{\n"+img("stock_c", "")+"}\n")
	const inZone = "|zone_source/zm_x.zone|csv"
	const xm = "|mc/mtl_b|material|xm_b|xmodel" + inZone
	write("usermaps/zm_x/zone_source/all/assetinfo/zm_x.csv", "index,type,name,resident,streamed,parentStack\n"+
		"1,material,mc/mtl_a,1000,0,|weapon_a|weapon"+inZone+"\n"+ // not an image, and before them
		"2,image,big_c,300,4194304,|mc/mtl_a|material|weapon_a|weapon"+inZone+"\n"+
		"3,image,half_n,300,1048576,"+xm+"\n"+
		"4,image,tiny,300,65536,"+xm+"\n"+
		"5,image,eighth,300,262144,"+xm+"\n"+
		"6,image,locked,300,1048576,"+xm+"\n"+
		"7,image,dup_c,300,262144,"+xm+"\n"+
		"8,image,blank_mip,300,2097152,"+xm+"\n"+
		"9,image,small,300,1000,"+xm+"\n"+
		"10,image,a_img,300,262144,"+xm+"\n"+
		"11,image,stock_c,300,2097152,"+xm+"\n"+
		"12,image,$white,15,0,"+xm+"\n"+
		"13,weapon,weapon_a,500,0,"+inZone+"\n")
	return root
}

func TestZoneImages(t *testing.T) {
	root := imagesRoot(t)
	res, err := zoneImages(root, "zm_x", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Yours != (imageGroup{9, 9244276}) || res.Stock != (imageGroup{1, 2097452}) || res.NoGDT != (imageGroup{1, 15}) {
		t.Errorf("groups: yours %+v stock %+v no_gdt %+v", res.Yours, res.Stock, res.NoGDT)
	}
	// a step leaves a quarter of the streamed bytes, rounded up to 64 KiB:
	// big_c 4 MiB -> 1 MiB, blank_mip 2 MiB -> 512 KiB, half_n 1 MiB -> 256 KiB,
	// a_img and dup_c 256 KiB -> 64 KiB; tiny stays at 64 KiB, small would grow
	// to it, eighth can't step down, locked keeps its size
	if want := int64(3145728 + 1572864 + 786432 + 196608 + 196608); res.StepSaves != want {
		t.Errorf("step_down_saves %d, want %d", res.StepSaves, want)
	}
	var got []string
	for _, im := range res.Largest {
		got = append(got, im.Image+" "+im.MipBase+" "+im.Line)
	}
	want := "big_c 1/1 weapon weapon_a|blank_mip 1/1 xmodel xm_b|half_n 1/2 xmodel xm_b|a_img 1/1 xmodel xm_b|dup_c 1/1 xmodel xm_b"
	if strings.Join(got, "|") != want {
		t.Errorf("largest: %q, want %q", strings.Join(got, "|"), want)
	}
	if len(res.Largest) == 5 && (res.Largest[4].AlsoIn != 1 || res.Largest[3].AlsoIn != 0) {
		t.Errorf("dup_c is defined in two GDTs, a_img in one: %+v", res.Largest[3:])
	}
	// mine.gdt saves the most; of the two that save as much, by name
	mine := imageGDTOut{GDT: "source_data/mine.gdt", imageGroup: imageGroup{7, 8719388}, StepSaves: 3145728 + 1572864 + 786432}
	if len(res.ByGDT) != 3 || res.ByGDT[0] != mine || res.ByGDT[1].GDT != "source_data/a_small.gdt" {
		t.Errorf("by_gdt: %+v", res.ByGDT)
	}

	line, err := zoneImages(root, "zm_x", "weapon,weapon_a")
	if err != nil {
		t.Fatal(err)
	}
	if line.Line != "weapon weapon_a" || line.Yours.Images != 1 || line.StepSaves != 3145728 || line.Stock.Images != 0 {
		t.Errorf("one line: %+v", line)
	}
	if _, err := zoneImages(root, "zm_x", "weapon,nothing"); err == nil {
		t.Error("a line the link didn't pack must be an error")
	}
	if _, err := zoneImages(root, "zm_none", ""); err == nil {
		t.Error("a map with no link report must be an error")
	}
}

// The answer keeps the 10 GDTs and the 10 images that save the most, and says
// how many GDTs it left out.
func TestImageCaps(t *testing.T) {
	var ims []yourImage
	for i := range 12 {
		ims = append(ims, yourImage{Packed: zone.Packed{ID: asset.ID{Type: "image", Name: fmt.Sprintf("img%02d", i)}, Streamed: 1 << 20},
			loc: gdt.Location{File: fmt.Sprintf("g%02d.gdt", i)}, mipBase: "1/1"})
	}
	by, more, total := imagesByGDT(ims)
	if len(by) != 10 || more != 2 || total != 12*786432 {
		t.Errorf("12 GDTs: %d listed, %d more, %d saved", len(by), more, total)
	}
	if by, more, _ := imagesByGDT(ims[:3]); len(by) != 3 || more != 0 {
		t.Errorf("3 GDTs: %d listed, %d more", len(by), more)
	}
	if l := largestImages(ims); len(l) != 10 {
		t.Errorf("12 images: %d listed", len(l))
	}
}
