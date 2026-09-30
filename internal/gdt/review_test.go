package gdt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for the branch review: odd layouts, same-name assets of
// different types in one GDT, write containment, cycles, and stable results.

func TestOddLayoutsFallBackToAFullRender(t *testing.T) {
	for name, src := range map[string]string{
		"one line":        `{ "a" ( "material.gdf" ) { "materialType" "lit" } }`,
		"closing braces":  "{\n\t\"a\" ( \"material.gdf\" )\n\t{\n\t\t\"materialType\" \"lit\"\n\t} }\n",
		"shared line":     "{\n\t\"a\" ( \"material.gdf\" ) { } \"b\" ( \"material.gdf\" ) { }\n}\n",
		"brace on header": "{ \"a\" ( \"material.gdf\" )\n\t{\n\t}\n}\n",
	} {
		f, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f.Find("a").Set("colorTint", "1 0 0 1")
		f.Add(&Asset{Name: "fresh", Type: "material"})
		out := f.Bytes() // must not panic on overlapping spans
		back, err := Parse(out)
		if err != nil || len(back.Assets) != len(f.Assets) {
			t.Fatalf("%s: rendered file doesn't round-trip (%v):\n%s", name, err, out)
		}
		if v, _ := back.Find("a").Get("colorTint"); v != "1 0 0 1" {
			t.Fatalf("%s: edit lost:\n%s", name, out)
		}
	}
}

func TestEmptyAndBOMFiles(t *testing.T) {
	f, err := Parse([]byte("// nothing yet\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.Add(&Asset{Name: "n", Type: "material"})
	if back, err := Parse(f.Bytes()); err != nil || len(back.Assets) != 1 {
		t.Fatalf("an asset added to a comment-only GDT must get the { } wrapper: %v\n%s", err, f.Bytes())
	}
	bom := append([]byte("\xEF\xBB\xBF"), "{\r\n\t\"a\" ( \"material.gdf\" )\r\n\t{\r\n\t}\r\n}\r\n"...)
	if f, err := Parse(bom); err != nil || len(f.Assets) != 1 || string(f.Bytes()) != string(bom) {
		t.Fatalf("a UTF-8 BOM must parse and round-trip: %v", err)
	}
}

func TestSameNameDifferentTypesInOneGDT(t *testing.T) {
	w := fixture(t)
	put(t, w, "source_data/pair.gdt", []byte("{\r\n"+
		"\t\"clip\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"semantic\" \"diffuseMap\"\r\n\t}\r\n"+
		"\t\"clip\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"colorMap\" \"clip\"\r\n\t}\r\n}\r\n"))
	if _, err := w.Edit(EditRequest{File: "source_data/pair.gdt", Asset: "clip", DryRun: true, Set: map[string]string{"noCastShadow": "1"}}); err == nil ||
		!strings.Contains(err.Error(), "pass type") {
		t.Fatalf("editing an ambiguous name without a type must be refused: %v", err)
	}
	r, err := w.Edit(EditRequest{File: "source_data/pair.gdt", Asset: "clip", Type: "material", Set: map[string]string{"noCastShadow": "1"}})
	if err != nil || !r.Written || r.Type != "material" || r.Created {
		t.Fatalf("type picks the material: %+v %v", r, err)
	}
	f, _ := ParseFile(filepath.Join(w.Root, "source_data", "pair.gdt"))
	img, mtl := f.FindAll("clip")[0], f.FindAll("clip")[1]
	if _, ok := img.Get("noCastShadow"); ok {
		t.Fatal("the material's field landed on the image")
	}
	if v, _ := mtl.Get("noCastShadow"); v != "1" {
		t.Fatal("the material wasn't edited")
	}
	hits, _, _ := w.ReferencedBy("clip")
	if len(hits) != 1 || hits[0].Field != "colorMap" {
		t.Fatalf("material clip's colorMap names image clip — a real reference: %+v", hits)
	}
}

func TestWritesStayInsideGDTDirs(t *testing.T) {
	w := fixture(t)
	for _, file := range []string{"../escape.gdt", "deffiles/x.gdt", filepath.Join(os.TempDir(), "x.gdt")} {
		if _, err := w.Edit(EditRequest{File: file, Asset: "x", Type: "material"}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("%s: a GDT outside gdtdb's directories must be refused: %v", file, err)
		}
	}
	if err := ValidName("asset name", "bad\"name"); err == nil {
		t.Error("a quote in a name must be rejected")
	}
	if _, err := w.Edit(EditRequest{File: "source_data/n.gdt", Asset: "ok", Type: "material", Set: map[string]string{"bad\nkey": "1"}}); err == nil {
		t.Error("a key with a line break must be rejected")
	}
}

func TestCheckReportsCyclesAndKeepsGoing(t *testing.T) {
	w := fixture(t)
	put(t, w, "source_data/cycle.gdt", []byte("{\r\n"+
		"\t\"a\" [ \"b\" ]\r\n\t{\r\n\t}\r\n\t\"b\" [ \"a\" ]\r\n\t{\r\n\t}\r\n"+
		"\t\"fine\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"stencil\" \"Sometimes\"\r\n\t}\r\n}\r\n"))
	r, err := w.Check("source_data/cycle.gdt", "")
	if err != nil {
		t.Fatalf("a cycle must not abort the whole check: %v", err)
	}
	if r.Checked != 3 || !hasIssue(reportFor(r, "a"), "", "error", "cycle") || !hasIssue(reportFor(r, "fine"), "stencil", "error", "not one of") {
		t.Fatalf("cycle reported on its assets, the rest still checked: %+v", r.Assets)
	}
}

func TestFindIsOrderedAndBuiltinsAreNotMissing(t *testing.T) {
	w := fixture(t)
	put(t, w, "source_data/a_first.gdt", []byte("{\r\n\t\"my_img\" ( \"image.gdf\" )\r\n\t{\r\n\t}\r\n}\r\n"))
	locs, _ := w.Find("my_img")
	if len(locs) != 2 || locs[0].File != "source_data/a_first.gdt" {
		t.Fatalf("Find must be sorted by file, line: %+v", locs)
	}
	r, err := w.Edit(EditRequest{File: "source_data/b.gdt", Asset: "m", Type: "material", DryRun: true,
		Set: map[string]string{"materialType": "lit", "colorMap": "$white_diffuse"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range r.Issues {
		if is.Field == "colorMap" {
			t.Fatalf("$ engine built-ins are not missing assets: %+v", is)
		}
	}
}

func TestImageDonorIgnoresMinorityKeys(t *testing.T) {
	w := fixture(t)
	put(t, w, "stock.gdtdef", []byte("model_export/stock.gdt\nsource_data/imgs.gdt\n"))
	var b strings.Builder
	b.WriteString("{\r\n")
	for i, extra := range []string{"", "", "\t\t\"quirk\" \"1\"\r\n"} {
		b.WriteString("\t\"i" + string(rune('0'+i)) + "\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"semantic\" \"normalMap\"\r\n\t\t\"mipBase\" \"1/1\"\r\n" + extra + "\t}\r\n")
	}
	b.WriteString("}\r\n")
	put(t, w, "source_data/imgs.gdt", []byte(b.String()))
	fields, n, err := w.imageDonor("normalMap")
	if err != nil || n != 3 {
		t.Fatalf("donor: %d %v", n, err)
	}
	for _, f := range fields {
		if f.Key == "quirk" {
			t.Fatal("a key on 1 of 3 images must not reach a new image")
		}
	}
}
