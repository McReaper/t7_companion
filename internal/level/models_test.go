package level

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// Every "model" key of the maps under map_source counts, prefabs and
// upper-case extensions too; lines that only look like one don't.
func TestScanModelsCountsTheEntitiesPlacingEachModel(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ent := func(model string) string { return "{\n\"classname\" \"misc_model\"\n\"model\" \"" + model + "\"\n}\n" }
	write("map_source/zm/a.map", ent("p7_barrel")+ent("p7_barrel")+ent("p7_chair")+"{\n  \"model\" \"p7_barrel\"  \r\n}\n")
	write("map_source/_prefabs/b.MAP", ent("p7_chair"))
	write("map_source/notes.txt", ent("p7_barrel"))
	write("map_source/c.map", strings.Join([]string{
		`"model" ""`, `"model" "with"quote"`, `"models" "p7_barrel"`, `"model" "p7_barrel`, `// "model" "p7_barrel"`,
		"// " + strings.Repeat("x", 600_000), ent("p7_after"),
	}, "\n"))
	got := ScanModels(root)
	want := map[string]int{"p7_barrel": 3, "p7_chair": 2, "p7_after": 1}
	if len(got) != len(want) {
		t.Errorf("%v, want %v", got, want)
	}
	for m, n := range want {
		if got[m] != n {
			t.Errorf("%s: %d, want %d", m, got[m], n)
		}
	}
	if len(ScanModels(filepath.Join(root, "none"))) != 0 {
		t.Error("an install with no map_source places nothing")
	}
}

// hide takes volumes and triggers for volumes, and any brush with a sky
// face for sky, in any entity; a volume_decal stays.
func TestHiddenByNamesVolumesAndSkyBrushes(t *testing.T) {
	face := func(m string) string {
		return " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) " + m + " 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n"
	}
	brush := func(g string, mats ...string) string {
		s := "{\nguid \"{" + g + "}\"\n"
		for _, m := range mats {
			s += face(m)
		}
		return s + "}\n"
	}
	m, err := mapfile.Parse([]byte("{\n\"classname\" \"worldspawn\"\n" + brush("S1", "sky", "caulk") + brush("W1", "caulk") + brush("S2", "SKY") + "}\n" +
		"{\nguid \"{V1}\"\n\"classname\" \"volume_sun\"\n" + brush("VB", "sky") + "}\n" +
		"{\nguid \"{T1}\"\n\"classname\" \"trigger_multiple\"\n}\n" +
		"{\nguid \"{D1}\"\n\"classname\" \"volume_decal\"\n}\n" +
		"{\nguid \"{G1}\"\n\"classname\" \"func_group\"\n" + brush("S3", "skybox_hidden", "sky") + brush("N1", "skyline_paint") + "}\n"))
	if err != nil {
		t.Fatal(err)
	}
	snap := &Snapshot{Map: m}
	for what, want := range map[string][]string{
		"volumes":     {"{V1}", "{T1}"},
		"sky":         {"{S1}", "{S2}", "{VB}", "{S3}"},
		"volumes,sky": {"{S1}", "{S2}", "{V1}", "{T1}", "{S3}"},
		"":            nil,
	} {
		if got := hiddenBy(snap, what); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", what, got, want)
		}
	}
}
