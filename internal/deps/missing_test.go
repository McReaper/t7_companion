package deps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// What the graph may call missing: an fx with no .efx is (the linker reads
// effects from fx/), a lens flare with no .klf is not (the linker links one
// anyway, from a source the graph doesn't read), an engine constant needs no
// source.
func TestDefined(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for rel, body := range map[string]string{
		"deffiles/xmodel.awi": `	Asset.AddEntry_Path( "filename", "" );`,
		"share/raw/techsetdefs_stable/geometry/lit.techsetdef": "Globals()\n{\n}\n",
		"share/raw/fx/here.efx":                                "iwfx 3\n",
		"share/raw/lensflares/known.klf":                       "{\n    uuid \"aaaa-1\"\n}\n",
		"bin/converter_gdt_dirs_0.txt":                         "source_data\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	g := New(w, "")
	for id, want := range map[asset.ID][2]bool{ // defined, known
		{Type: "fx", Name: "here"}:                 {true, true},
		{Type: "fx", Name: "gone"}:                 {false, true},
		{Type: "klf", Name: "aaaa-1"}:              {false, false},
		{Type: "klf", Name: "bbbb-2"}:              {false, false},
		{Type: "image", Name: "vdReveal"}:          {true, true},
		{Type: "sound", Name: "zm_x"}:              {false, false},
		{Type: "xmodel", Name: "nowhere"}:          {false, true},
		{Type: "scriptparsetree", Name: "a/b.gsc"}: {false, true},
	} {
		if d, k := g.defined(id); d != want[0] || k != want[1] {
			t.Errorf("defined(%s) = %v, %v; want %v, %v", id, d, k, want[0], want[1])
		}
	}
}
