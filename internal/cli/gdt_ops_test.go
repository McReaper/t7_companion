package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/gdt"
)

func TestGDTGet(t *testing.T) {
	root := fakeToolsRoot(t)
	// "dup" is an image here and a material in other.gdt; "child" derives from "clean"
	more := "{\r\n\t\"dup\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"semantic\" \"normalMap\"\r\n\t}\r\n" +
		"\t\"child\" [ \"clean\" ]\r\n\t{\r\n\t\t\"gloss\" \"2\"\r\n\t}\r\n" +
		"\t\"clean\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"colorMap\" \"dup\"\r\n\t}\r\n}\r\n"
	other := "{\r\n\t\"dup\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n}\r\n"
	for rel, body := range map[string]string{"source_data/more.gdt": more, "source_data/other.gdt": other} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	get := func(name, file, filter string, all bool) gdtAssetView {
		t.Helper()
		v, err := gdtGet(w, name, file, filter, all)
		if err != nil {
			t.Fatalf("gdtGet(%q, %q): %v", name, file, err)
		}
		return v.(gdtAssetView)
	}

	t.Run("a derived asset shows its own and inherited fields", func(t *testing.T) {
		v := get("child", "", "", false)
		if v.Parent != "clean" || v.Type != "material" || v.Own["gloss"] != "2" || v.Inherited["materialType"] != "lit" || v.Inherited["colorMap"] != "dup" {
			t.Fatalf("%+v", v)
		}
		if _, overridden := v.Inherited["gloss"]; overridden {
			t.Fatalf("a field the asset sets itself is not inherited: %+v", v.Inherited)
		}
	})

	t.Run("filter keeps matching keys only", func(t *testing.T) {
		v := get("child", "", "MATERIAL", false)
		if len(v.Own) != 0 || len(v.Inherited) != 1 || v.Inherited["materialType"] != "lit" {
			t.Fatalf("filter is a case-insensitive substring of the key: %+v", v)
		}
	})

	t.Run("file picks one definition and lists the others", func(t *testing.T) {
		v := get("dup", `source_data\other.gdt`, "", false)
		if v.File != "source_data/other.gdt" || v.Type != "material" || len(v.Others) != 2 {
			t.Fatalf("%+v", v)
		}
		if v = get("dup", "source_data/more.gdt", "", false); v.Type != "image" || v.Own["semantic"] != "normalMap" {
			t.Fatalf("%+v", v)
		}
	})

	t.Run("errors", func(t *testing.T) {
		if _, err := gdtGet(w, "dup", "source_data/test.gdt", "", false); err == nil || !strings.Contains(err.Error(), "not defined in") {
			t.Fatalf("a file that doesn't define the asset: %v", err)
		}
		if _, err := gdtGet(w, "nowhere", "", "", false); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("an unknown asset: %v", err)
		}
	})
}
