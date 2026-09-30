package gdt

import "testing"

// FuzzParse feeds the parser arbitrary input — community GDTs come in every
// shape. It must never panic, and anything it accepts must survive a write:
// rendering (with an edit and an added asset) has to parse back to the same
// assets. Run longer with: go test ./internal/gdt -fuzz FuzzParse -fuzztime 60s
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"{\r\n\t\"a\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t}\r\n\t\"b\" [ \"a\" ]\r\n\t{\r\n\t}\r\n}\r\n",
		`{ "a" ( "x.gdf" ) { "k" "v" } }`,
		"{\n\t\"a\" ( \"x.gdf\" ) { } \"b\" ( \"x.gdf\" ) { }\n}\n",
		"// only a comment\n",
		"\xEF\xBB\xBF{\r\n}\r\n",
		"{\n\t\"p\" ( \"xmodel.gdf\" )\n\t{\n\t\t\"filename\" \"a\\\\b\\\"c.xmodel_bin\"\n\t}\n}\n",
		"",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		file, err := Parse(src)
		if err != nil {
			return
		}
		if len(file.Assets) > 0 {
			file.Assets[0].Set("fuzzKey", "1")
		}
		file.Add(&Asset{Name: "fuzz_added", Type: "material", Fields: []Field{{Key: "k", Value: "v"}}})
		out := file.Bytes()
		back, err := Parse(out)
		if err != nil {
			t.Fatalf("accepted input renders to a file that doesn't parse: %v\ninput: %q\noutput: %q", err, src, out)
		}
		if len(back.Assets) != len(file.Assets) {
			t.Fatalf("%d assets in memory, %d after a write\ninput: %q\noutput: %q", len(file.Assets), len(back.Assets), src, out)
		}
		for i, a := range file.Assets {
			if back.Assets[i].Name != a.Name || back.Assets[i].Parent != a.Parent || len(back.Assets[i].Fields) != len(a.Fields) {
				t.Fatalf("asset %d changed across a write: %+v -> %+v\ninput: %q\noutput: %q", i, a, back.Assets[i], src, out)
			}
		}
	})
}
