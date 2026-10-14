package gdt

import (
	"sort"
	"testing"
)

func TestEditorMaterialsAreTheOnesRadiantsBrowserShows(t *testing.T) {
	w := fixture(t)
	put(t, w, "stock.gdtdef", []byte("model_export/stock.gdt\nsource_data/stock_mats.gdt\n"))
	put(t, w, "source_data/stock_mats.gdt", []byte("{\r\n"+
		"\t\"st_brick\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"exterior wall\"\r\n\t\t\"surfaceType\" \"brick\"\r\n\t}\r\n}\r\n"))
	put(t, w, "source_data/mats.gdt", []byte("{\r\n"+
		"\t\"m_floor\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"colorMap\" \"i_planks\"\r\n\t\t\"materialType\" \"lit\"\r\n"+
		"\t\t\"surfaceType\" \"wood\"\r\n\t\t\"usage\" \"floor\"\r\n\t}\r\n"+
		"\t\"m_child\" [ \"m_floor\" ]\r\n\t{\r\n\t\t\"surfaceType\" \"brick\"\r\n\t}\r\n"+
		"\t\"m_grandchild\" [ \"m_child\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"m_model\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"<not in editor>\"\r\n\t}\r\n"+
		"\t\"m_nousage\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"surfaceType\" \"metal\"\r\n\t}\r\n"+
		"\t\"m_dnu\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"doNotUse\" \"1\"\r\n\t\t\"usage\" \"tools\"\r\n\t}\r\n"+
		"\t\"m_used\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"doNotUse\" \"0\"\r\n\t\t\"usage\" \"tools\"\r\n\t}\r\n"+
		"\t\"i_planks\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"usage\" \"floor\"\r\n\t}\r\n"+
		"\t\"i_child\" [ \"i_planks\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"loop_a\" [ \"loop_b\" ]\r\n\t{\r\n\t\t\"usage\" \"floor\"\r\n\t}\r\n"+
		"\t\"loop_b\" [ \"loop_a\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"orphan\" [ \"elsewhere\" ]\r\n\t{\r\n\t\t\"usage\" \"floor\"\r\n\t}\r\n}\r\n"))
	got := map[string]EditorMaterial{}
	var names []string
	for _, m := range w.EditorMaterials() {
		got[m.Name] = m
		names = append(names, m.Name)
	}
	sort.Strings(names)
	want := map[string]EditorMaterial{
		"st_brick":     {Name: "st_brick", Usage: "exterior wall", Surface: "brick", File: "source_data/stock_mats.gdt", Stock: true},
		"m_floor":      {Name: "m_floor", Usage: "floor", Surface: "wood", ColorMap: "i_planks", File: "source_data/mats.gdt"},
		"m_child":      {Name: "m_child", Usage: "floor", Surface: "brick", ColorMap: "i_planks", File: "source_data/mats.gdt"},
		"m_grandchild": {Name: "m_grandchild", Usage: "floor", Surface: "brick", ColorMap: "i_planks", File: "source_data/mats.gdt"},
		"m_dnu":        {Name: "m_dnu", Usage: "tools", File: "source_data/mats.gdt", DoNotUse: true},
		"m_used":       {Name: "m_used", Usage: "tools", File: "source_data/mats.gdt"},
	}
	if len(got) != len(want) {
		t.Errorf("materials %v: the ones with a usage, their own or a parent's, other than <not in editor>", names)
	}
	for n, m := range want {
		if got[n] != m {
			t.Errorf("%s: %+v, want %+v", n, got[n], m)
		}
	}
}

func TestMatLineReadsACatalogFieldsLine(t *testing.T) {
	for line, want := range map[string]matFields{
		"\t\t\"usage\" \"interior wall\"": {Usage: "interior wall"},
		"  \"surfaceType\"   \"brick\"  ": {Surface: "brick"},
		"\"colorMap\" \"a\\\\b\"":         {Color: `a\b`},
		"\t\t\"doNotUse\" \"1\"":          {DoNotUse: "1"},
		"\t\t\"usages\" \"floor\"":        {},
		"\t\t\"usage\" floor":             {},
		"\t\t\"usage\" \"":                {},
		"\t\t\"usage":                     {},
		"":                                {},
		"\"":                              {},
	} {
		var m matFields
		matLine(&m, []byte(line))
		if m != want {
			t.Errorf("%q: %+v, want %+v", line, m, want)
		}
	}
}
