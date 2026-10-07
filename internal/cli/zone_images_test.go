package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// imagesRoot lays out a mod-tools root whose last link of zm_x packed images
// from a GDT of yours (one inheriting mipBase 1/2, one locked by doNotResize,
// one already at 1/8, one too small for a step to save anything, one defined
// in two GDTs), from a stock GDT, and from none.
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
		img("big_c", "\t\t\"mipBase\" \"1/1\"\n")+
		img("base_img", "\t\t\"mipBase\" \"1/2\"\n")+
		"\t\"half_n\" [ \"base_img\" ]\n\t{\n\t\t\"baseImage\" \"x.tif\"\n\t}\n"+
		img("tiny", "")+
		img("eighth", "\t\t\"mipBase\" \"1/8\"\n")+
		img("locked", "\t\t\"doNotResize\" \"1\"\n")+
		img("dup_c", "")+
		"}\n")
	write("source_data/other.gdt", "{\n"+img("dup_c", "")+"}\n")
	write("source_data/stock_imgs.gdt", "{\n"+img("stock_c", "")+"}\n")
	const zone = "|zone_source/zm_x.zone|csv"
	write("usermaps/zm_x/zone_source/all/assetinfo/zm_x.csv", "index,type,name,resident,streamed,parentStack\n"+
		"1,image,big_c,300,4194304,|mc/mtl_a|material|weapon_a|weapon"+zone+"\n"+
		"2,image,half_n,300,1048576,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"3,image,tiny,300,65536,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"4,image,eighth,300,262144,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"5,image,locked,300,1048576,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"6,image,dup_c,300,262144,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"7,image,stock_c,300,2097152,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"8,image,$white,15,0,|mc/mtl_b|material|xm_b|xmodel"+zone+"\n"+
		"9,material,mc/mtl_a,1000,0,|weapon_a|weapon"+zone+"\n"+
		"10,weapon,weapon_a,500,0,"+zone+"\n")
	return root
}

func TestZoneImages(t *testing.T) {
	root := imagesRoot(t)
	res, err := zoneImages(root, "zm_x", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Yours.Images != 6 || res.Stock != (imageGroup{1, 2097452}) || res.NoGDT != (imageGroup{1, 15}) {
		t.Errorf("groups: yours %+v stock %+v no_gdt %+v", res.Yours, res.Stock, res.NoGDT)
	}
	// a step leaves a quarter of the streamed bytes, rounded up to 64 KiB:
	// big_c 4 MiB -> 1 MiB, half_n 1 MiB -> 256 KiB, dup_c 256 KiB -> 64 KiB;
	// tiny stays at 64 KiB, eighth can't step down, locked keeps its size
	if want := int64(3145728 + 786432 + 196608); res.StepSaves != want {
		t.Errorf("step_down_saves %d, want %d", res.StepSaves, want)
	}
	var got []string
	for _, im := range res.Largest {
		got = append(got, im.Image+" "+im.MipBase+" "+im.Line)
	}
	if want := "big_c 1/1 weapon weapon_a|half_n 1/2 xmodel xm_b|dup_c 1/1 xmodel xm_b"; strings.Join(got, "|") != want {
		t.Errorf("largest: %q, want %q", strings.Join(got, "|"), want)
	}
	if dup := res.Largest[2]; dup.AlsoIn != 1 {
		t.Errorf("dup_c is defined in two GDTs: %+v", dup)
	}
	if len(res.ByGDT) == 0 || res.ByGDT[0].GDT != "source_data/mine.gdt" || res.ByGDT[0].StepSaves < 3145728+786432 {
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
}
