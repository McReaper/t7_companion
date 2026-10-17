package gdt

import (
	"slices"
	"testing"
)

// The xmodels of every GDT, by name: a derived asset is one when its parent
// chain in its GDT is; a loop or a parent elsewhere makes none.
func TestModelsAreTheXmodelsOfTheGDTs(t *testing.T) {
	w := fixture(t)
	put(t, w, "stock.gdtdef", []byte("model_export/stock.gdt\nsource_data/stock_props.gdt\n"))
	put(t, w, "source_data/stock_props.gdt", []byte("{\r\n"+
		"\t\"p7_barrel\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t\t\"filename\" \"barrel.xmodel_bin\"\r\n\t}\r\n}\r\n"))
	put(t, w, "source_data/props.gdt", []byte("{\r\n"+
		"\t\"p_chair\" ( \"xmodel.gdf\" )\r\n\t{\r\n\t}\r\n"+
		"\t\"p_chair_red\" [ \"p_chair\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"m_chair\" ( \"material.gdf\" )\r\n\t{\r\n\t}\r\n"+
		"\t\"loop_a\" [ \"loop_b\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"loop_b\" [ \"loop_a\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"orphan\" [ \"p7_barrel\" ]\r\n\t{\r\n\t}\r\n}\r\n"))
	got := w.Models()
	want := []Model{
		{Name: "p7_barrel", File: "source_data/stock_props.gdt", Stock: true},
		{Name: "p_chair", File: "source_data/props.gdt"},
		{Name: "p_chair_red", File: "source_data/props.gdt"},
		{Name: "stock_model", File: "model_export/stock.gdt", Stock: true}, // the fixture's
	}
	if !slices.Equal(got, want) {
		t.Errorf("%+v\nwant %+v", got, want)
	}
}
