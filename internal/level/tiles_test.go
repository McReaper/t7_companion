package level

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanTilesFindsTheTileEachMaterialHasMost(t *testing.T) {
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
	f := func(mat, w, h string) string {
		return " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) " + mat + " " + w + " " + h + " 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0\r\n"
	}
	write("map_source/zm/a.map", "{\r\n{\r\n"+f("brick", "64", "32")+f("brick", "-64", "32")+f("brick", "128", "64")+
		f("wood", "128", "128")+f("wood", "64", "64")+"}\r\n{\r\nmesh\r\n{\r\n ( 0 0 0 ) ( 1 0 0 ) short\r\n}\r\n}\r\n}\r\n")
	write("map_source/_prefabs/b.MAP", "{\n{\n"+f("brick", "64", "-32")+f("caulk", "0", "16")+f("caulk", "x", "16")+"}\n}\n")
	write("map_source/notes.txt", f("brick", "8", "8")+f("brick", "8", "8")+f("brick", "8", "8")+f("brick", "8", "8"))
	got := ScanTiles(root)
	if u := got["brick"]; u != (TileUse{W: 64, H: 32, N: 3, Faces: 4}) {
		t.Errorf("brick %+v: 3 of its 4 faces at 64×32, signs dropped, .map files only", u)
	}
	if u := got["wood"]; u != (TileUse{W: 64, H: 64, N: 1, Faces: 2}) {
		t.Errorf("wood %+v: a tie goes to the smaller tile", u)
	}
	if _, ok := got["caulk"]; ok || len(got) != 2 {
		t.Errorf("%v: a zero or unreadable size is no face", got)
	}
	if len(ScanTiles(filepath.Join(root, "missing"))) != 0 {
		t.Error("no map_source, no tiles")
	}
}
