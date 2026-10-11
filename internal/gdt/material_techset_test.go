package gdt

import (
	"strings"
	"testing"
)

// A material's techset is found through the materialType it inherits from its
// parent; a material with none, or no material, is an error.
func TestMaterialTechset(t *testing.T) {
	w := fixture(t)
	ts, err := w.MaterialTechset("child_mtl")
	if err != nil || ts.MaterialType != "lit" {
		t.Fatalf("child_mtl: %+v %v", ts, err)
	}
	if _, err := w.MaterialTechset("no_such_mtl"); err == nil {
		t.Error("a material no GDT defines must be an error")
	}
	if _, err := w.MaterialTechset("my_img"); err == nil {
		t.Error("an image is not a material")
	}
	put(t, w, "source_data/typeless.gdt", []byte("{\r\n\t\"typeless_mtl\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"colorMap\" \"my_img\"\r\n\t}\r\n}\r\n"))
	w.Touched(w.Abs("source_data/typeless.gdt"))
	if _, err := w.MaterialTechset("typeless_mtl"); err == nil || !strings.Contains(err.Error(), "no materialType") {
		t.Errorf("a material with no materialType: %v", err)
	}
	w.Techsets = nil
	if _, err := w.MaterialTechset("child_mtl"); err == nil {
		t.Error("no techsetdefs must be an error")
	}
}
