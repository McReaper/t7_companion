package level

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// Lines that are no face, each followed by one that is: a line of 18
	// fields is the shortest face it reads; one past the buffer's 64 KiB start.
	write("map_source/c.map", strings.Join([]string{
		" ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) short17 64", f("after", "16", "16"),
		" ( 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 )", f("after", "16", "16"),
		" ( 0 0 0 ) x x x x x x x x x x unclosed 64 32 0 0 0 0", f("after", "16", "16"),
		" ( 0 0 0 0 0 0 0 0 0 0 0 0 0 ) unopened 64 32 0 0 0 0", f("after", "16", "16"),
		f("flat", "16", "0"), f("after", "16", "16"),
		f("nan", "16", "x"), f("after", "16", "16"),
		"// " + strings.Repeat("x", 600_000), f("after", "16", "16"),
		" ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) edge18 64 32",
	}, "\n"))
	// Ties go to the smaller tile across, then down, whatever order the
	// counts come in: twenty materials with the same four tiles once each.
	var ties strings.Builder
	for i := range 20 {
		for _, wh := range [][2]string{{"128", "8"}, {"64", "64"}, {"64", "32"}, {"256", "8"}} {
			ties.WriteString(f(fmt.Sprintf("tie%d", i), wh[0], wh[1]))
		}
	}
	write("map_source/ties.map", ties.String())

	got := ScanTiles(root)
	for m, want := range map[string]TileUse{
		"brick":  {W: 64, H: 32, N: 3, Faces: 4},
		"wood":   {W: 64, H: 64, N: 1, Faces: 2},
		"after":  {W: 16, H: 16, N: 7, Faces: 7},
		"edge18": {W: 64, H: 32, N: 1, Faces: 1},
	} {
		if got[m] != want {
			t.Errorf("%s %+v, want %+v", m, got[m], want)
		}
	}
	for i := range 20 {
		if u := got[fmt.Sprintf("tie%d", i)]; u != (TileUse{W: 64, H: 32, N: 1, Faces: 4}) {
			t.Errorf("tie%d %+v: a tie goes to the smaller tile", i, u)
		}
	}
	if len(got) != 24 {
		t.Errorf("%d materials %v: a zero or unreadable size, a line short of a face, .map files only", len(got), got)
	}
	if len(ScanTiles(filepath.Join(root, "missing"))) != 0 {
		t.Error("no map_source, no tiles")
	}
}
