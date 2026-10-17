package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// A box and a texture op in an install: a material its maps use often gets
// the tile they use, one they hardly use its GDT's tiling, one nothing sizes
// Radiant's 64.
func TestMapEditSizesTilesFromTheInstall(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir()) // keep the GDT index's cache out of the real profile
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
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
	face := func(mat, w, h string) string {
		return " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) " + mat + " " + w + " " + h + " 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\n"
	}
	write("deffiles/material.awi", "")
	write("share/raw/techsetdefs_stable/geometry/lit.techsetdef", "")
	faces := strings.Repeat(face("brick", "128", "64"), minTileFaces) + face("rare", "8", "8")
	write("map_source/zm/other.map", "{\n{\n"+faces+"}\n}\n")
	write("source_data/m.gdt", "{\r\n\t\"rare\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"tilingWidth\" \"512\"\r\n\t\t\"tilingHeight\" \"256\"\r\n\t}\r\n}\r\n")
	b, err := os.ReadFile("../level/testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	f := radianttest.New(41, filepath.Join(root, "map_source", "zm", "test_claude.map"))
	f.Text = string(b)[strings.Index(string(b), "// entity 0"):]
	onMachine(t, f)

	ops, err := decodeOps([]byte(`[
		{"box": {"min": [0, 0, 0], "max": [64, 64, 64], "material": "brick"}},
		{"box": {"min": [0, 0, 64], "max": [64, 64, 72], "material": "rare"}},
		{"texture": {"ids": ["@1"], "faces": "top", "material": "none_such"}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := mapEdit(context.Background(), ops, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]tileOut{
		"brick":     {[2]float64{128, 64}, "the install's maps, on 10 of its 10 faces"},
		"rare":      {[2]float64{32, 16}, "its GDT's 512×256 texels at 16 a unit"},
		"none_such": {[2]float64{64, 64}, `assumed, no GDT defines material "none_such"`},
	}
	for m, w := range want {
		if out.Tiles[m] != w {
			t.Errorf("%s: %+v, want %+v", m, out.Tiles[m], w)
		}
	}
	if len(out.Tiles) != 3 || len(out.Textured) != 1 || out.Textured[0] != (texturedOut{Op: 2, Faces: 1}) || len(out.Made) != 2 {
		t.Errorf("answer %+v", out)
	}
}

// The install's maps are read once per install; an install whose GDTs can't be
// opened sizes nothing, and says why.
func TestTileUsesAreReadOnceAndAnUnopenableInstallSaysWhy(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "deffiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	tileUses(root)["sentinel"] = level.TileUse{W: 1}
	if tileUses(root)["sentinel"].W != 1 {
		t.Error("the install's maps were read again")
	}
	_, err := mapTiles(filepath.Join(root, "map_source", "x.map"))("brick")
	if err == nil || !strings.Contains(err.Error(), "techsetdefs") {
		t.Errorf("an install with no techsets: %v", err)
	}
}

func TestInstallOfIsTheFolderWithDeffilesAboveTheMap(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "deffiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TA_TOOLS_PATH", `D:\elsewhere\`)
	if got := installOf(filepath.Join(root, "map_source", "zm", "x.map")); got != root {
		t.Errorf("installOf = %q, want %q", got, root)
	}
	if got := installOf(filepath.Join(t.TempDir(), "x.map")); got != `D:\elsewhere` {
		t.Errorf("a map in no install: %q, $TA_TOOLS_PATH's", got)
	}
	t.Setenv("TA_TOOLS_PATH", "")
	if installOf("") != "" || mapTiles("") != nil {
		t.Error("no map and no TA_TOOLS_PATH: no install, no tiles")
	}
}
