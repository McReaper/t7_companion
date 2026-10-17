package gdt

import (
	"strings"
	"testing"
)

func TestMaterialTilingIsItsTilingOrItsLargestImage(t *testing.T) {
	w := fixture(t)
	put(t, w, "texture_assets/t/tall.tif", tiffOf(512, 2048))
	put(t, w, "texture_assets/t/wide.tif", tiffOf(1024, 256))
	put(t, w, "texture_assets/t/line.tif", tiffOf(1, 64))
	put(t, w, "texture_assets/t/tiny.tif", tiffOf(8, 8))
	img := func(name, file, mip string) string {
		s := "\t\"" + name + "\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"baseImage\" \"texture_assets\\\\t\\\\" + file + "\"\r\n"
		if mip != "" {
			s += "\t\t\"mipBase\" \"" + mip + "\"\r\n"
		}
		return s + "\t}\r\n"
	}
	mat := func(name string, fields ...string) string {
		s := "\t\"" + name + "\" ( \"material.gdf\" )\r\n\t{\r\n"
		for i := 0; i < len(fields); i += 2 {
			s += "\t\t\"" + fields[i] + "\" \"" + fields[i+1] + "\"\r\n"
		}
		return s + "\t}\r\n"
	}
	put(t, w, "source_data/tiling.gdt", []byte("{\r\n"+
		img("i_tall", "tall.tif", "1/2")+img("i_wide", "wide.tif", "")+img("i_gone", "gone.tif", "")+
		mat("m_set", "tilingWidth", "1024", "tilingHeight", "128", "colorMap", "i_tall")+
		mat("m_auto", "tilingWidth", "<auto>", "colorMap", "i_tall", "specColorMap", "i_wide")+
		mat("m_half", "tilingWidth", "256", "tilingHeight", "<auto>", "colorMap", "i_wide")+
		mat("m_builtin", "colorMap", "$white_diffuse", "specColorMap", "i_gone")+
		img("i_line", "line.tif", "")+img("i_tiny", "tiny.tif", "1/16")+
		mat("m_tall", "colorMap", "i_tall")+
		mat("m_wauto", "tilingWidth", "<auto>", "tilingHeight", "64", "colorMap", "i_wide")+
		mat("m_wonly", "tilingWidth", "256", "colorMap", "i_gone")+
		mat("m_honly", "tilingHeight", "64", "colorMap", "i_gone")+
		mat("m_line", "colorMap", "i_line")+
		mat("m_tiny", "colorMap", "i_tiny")+
		mat("m_mixed", "colorMap", "$white_diffuse", "specColorMap", "i_wide")+
		mat("m_gonefirst", "colorMap", "i_gone", "specColorMap", "i_wide")+
		"\t\"m_child\" [ \"m_set\" ]\r\n\t{\r\n\t}\r\n}\r\n"))
	for name, want := range map[string]Tiling{
		"m_set":       {1024, 128, "tiling"},
		"m_auto":      {1024, 1024, "images"}, // 256×1024 at mipBase 1/2, and 1024×256
		"m_half":      {256, 256, "tiling and images"},
		"m_wauto":     {1024, 64, "tiling and images"},
		"m_child":     {1024, 128, "tiling"},
		"m_tall":      {256, 1024, "images"},
		"m_line":      {1, 64, "images"},
		"m_tiny":      {1, 1, "images"}, // 8 texels at mipBase 1/16 still load one
		"m_mixed":     {1024, 256, "images"},
		"m_gonefirst": {1024, 256, "images"},
	} {
		if got, err := w.MaterialTiling(name); err != nil || got != want {
			t.Errorf("%s: %+v, %v, want %+v", name, got, err, want)
		}
	}
	for name, msg := range map[string]string{
		"m_builtin": "no image of it on disk",
		"m_wonly":   "no image of it on disk",
		"m_honly":   "no image of it on disk",
		"m_nowhere": `no GDT defines material "m_nowhere"`,
		"i_tall":    `no GDT defines material "i_tall"`,
	} {
		if _, err := w.MaterialTiling(name); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: %v, want %q", name, err, msg)
		}
	}
	if _, _, err := w.imageTexels("i_none"); err == nil {
		t.Error("an image no GDT defines has no size")
	}
}
