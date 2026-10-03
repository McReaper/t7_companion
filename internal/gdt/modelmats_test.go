package gdt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An xmodel's materials come from its LOD files: gdt_refs on a material must
// find the models whose LOD files use it, and see a LOD file re-exported since.
func TestModelUses(t *testing.T) {
	w := fixture(t) // stock_model: filename a\b, lowLod donor\low, mediumLod donor\med (missing)
	write := func(rel, body string) {
		p := filepath.Join(w.Root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	export := func(mats ...string) string { // the text form, which Materials reads too
		s := "MODEL\nVERSION 6\n"
		for i, m := range mats {
			s += "MATERIAL " + string(rune('0'+i)) + " \"" + m + "\" \"Phong\" \"Phong\"\n"
		}
		return s
	}
	write("model_export/a/b.xmodel_bin", export("mtl_x"))
	write("model_export/donor/low.xmodel_bin", export("mtl_y", "mtl_x"))

	fields := func(material string) string {
		hits, err := w.ModelUses(material)
		if err != nil {
			t.Fatal(err)
		}
		var f []string
		for _, h := range hits {
			if h.Asset != "stock_model" || h.Type != "xmodel" || h.File != "model_export/stock.gdt" {
				t.Errorf("unexpected hit %+v", h)
			}
			f = append(f, h.Field)
		}
		return strings.Join(f, " | ")
	}
	if got := fields("MTL_X"); got != "filename file | lowLod file" {
		t.Errorf("mtl_x (case-insensitive): %s", got)
	}
	if got := fields("mtl_y"); got != "lowLod file" {
		t.Errorf("mtl_y: %s", got)
	}

	// re-export the LOD file with other materials: the cache must notice
	write("model_export/donor/low.xmodel_bin", export("mtl_z"))
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(w.Root, "model_export", "donor", "low.xmodel_bin"), later, later); err != nil {
		t.Fatal(err)
	}
	if got := fields("mtl_y"); got != "" {
		t.Errorf("a stale LOD file was served from the cache: %s", got)
	}
	if got := fields("mtl_z"); got != "lowLod file" {
		t.Errorf("mtl_z after the re-export: %s", got)
	}

	// a fresh workspace on the same root reads the persisted cache
	w2, err := Open(w.Root)
	if err != nil {
		t.Fatal(err)
	}
	if hits, err := w2.ModelUses("mtl_x"); err != nil || len(hits) != 1 {
		t.Errorf("from the persisted cache: %+v, %v", hits, err)
	}
}

func TestListHas(t *testing.T) {
	v := `mtl_skybox_default mtl_skybox_default_day\r\n` // as a GDT stores it: the escape text, not a line break
	for name, want := range map[string]bool{"mtl_skybox_default": true, "mtl_skybox_default_day": true, "mtl_skybox": false} {
		if got := listHas(v, name); got != want {
			t.Errorf("listHas(%q) = %v, want %v", name, got, want)
		}
	}
	if listHas("mtl_a mtl_b", "mtl_a") {
		t.Error("a value without the list separator is not a list")
	}
}
