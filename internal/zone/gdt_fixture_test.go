package zone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// layout writes files (path relative to a new root -> body) and returns the root.
func layout(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// gdtRoot is a mod-tools root the gdt package can open: the given files over
// the deffiles, GDT folder list and techset it needs. A GDT listed in
// stock.gdtdef is stock.
func gdtRoot(t *testing.T, files map[string]string) (string, *gdt.Workspace) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir()) // keep the index caches out of the real profile
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	all := map[string]string{
		"deffiles/image.awi":                                   "\tAsset.AddEntry_Combo(\"mipBase\", \"1/1|1/2|1/4|1/8\");\n\tAsset.AddEntry_CheckBox(\"doNotResize\", false);\n",
		"deffiles/material.awi":                                `	Asset.AddEntry_Combo( "materialType", "lit" );`,
		"deffiles/bulletweapon.awi":                            `	Asset.AddEntry_AssetCombo( "worldModel", "xmodel" );`,
		"bin/converter_gdt_dirs_0.txt":                         "source_data\n",
		"stock.gdtdef":                                         "source_data/stock.gdt\n",
		"share/raw/techsetdefs_stable/geometry/lit.techsetdef": "Globals()\n{\n\tcategory = \"Geometry\"\n}\n",
	}
	for k, v := range files {
		all[k] = v
	}
	root := layout(t, all)
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, w
}

// gdtAsset is one asset of a GDT, with its fields, as APE writes it.
func gdtAsset(name, typ string, fields ...string) string {
	s := "\t\"" + name + "\" ( \"" + typ + ".gdf\" )\n\t{\n"
	for i := 0; i+1 < len(fields); i += 2 {
		s += "\t\t\"" + fields[i] + "\" \"" + fields[i+1] + "\"\n"
	}
	return s + "\t}\n"
}

// linked writes usermaps/<name>'s report (the csv rows after its header) and
// loads it.
func linked(t *testing.T, root, name, rows string) *Report {
	t.Helper()
	dir := filepath.Join(root, "usermaps", name, "zone_source", "all", "assetinfo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".csv"), []byte("index,type,name,resident,streamed,parentStack\n"+rows), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Load(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
