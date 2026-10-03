package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// For every xmodel the linker packed, the materials read from its LOD files
// must be the materials the linker packed under it (a skinOverride swaps one
// for another, so only maps without overrides are compared strictly). Runs on
// a real install, opt-in (T7KB_ORACLE=1 and TA_TOOLS_PATH): a modder's machine
// has TA_TOOLS_PATH set anyway, and a cold run reads every LOD file.
func TestModelMaterialsMatchLinker(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" || os.Getenv("T7KB_ORACLE") == "" {
		t.Skip("set T7KB_ORACLE=1 and TA_TOOLS_PATH to check LOD materials against the linker's reports")
	}
	w, err := workspace(root)
	if err != nil {
		t.Fatal(err)
	}
	dirs, _ := filepath.Glob(filepath.Join(root, "usermaps", "*", "zone_source", "all", "assetinfo"))
	same, differ, skipped := 0, 0, 0
	for _, dir := range dirs {
		name := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(dir))))
		r, err := zone.Load(dir, name)
		if err != nil {
			continue
		}
		packed := map[string]map[string]bool{} // xmodel -> materials packed directly under it
		for _, p := range r.Assets {
			if p.Type == "material" && len(p.Chain) > 0 && p.Chain[0].Type == "xmodel" {
				m := packed[p.Chain[0].Name]
				if m == nil {
					m = map[string]bool{}
					packed[p.Chain[0].Name] = m
				}
				m[strings.ToLower(stripCategory(p.Name))] = true
			}
		}
		for model, want := range packed {
			got, err := w.ModelMaterials(model)
			if err != nil || got == nil {
				skipped++
				continue
			}
			gotSet := map[string]bool{}
			for _, g := range got {
				gotSet[strings.ToLower(g)] = true
			}
			for from, to := range skinOverrides(t, w, model) { // the linker packs the replacement
				if gotSet[from] {
					gotSet[to] = true
				}
			}
			missing := 0
			for m := range want {
				if !gotSet[m] {
					missing++
				}
			}
			if missing == 0 {
				same++
			} else {
				differ++
				if differ <= 8 {
					t.Logf("%s / %s: packed %v, LOD files %v", name, model, keys(want), got)
				}
			}
		}
	}
	t.Logf("xmodels whose packed materials all come from their LOD files: %d; not: %d; no GDT definition: %d", same, differ, skipped)
	if same == 0 {
		t.Error("no xmodel checked")
	}
	if differ > 0 {
		t.Errorf("%d xmodels packed a material their LOD files (after skinOverride) don't name", differ)
	}
}

// skinOverrides reads an xmodel's skinOverride pairs ("from to" per line),
// lower-cased.
func skinOverrides(t *testing.T, w *gdt.Workspace, model string) map[string]string {
	t.Helper()
	locs, err := w.FindTyped(model, "xmodel")
	if err != nil || len(locs) == 0 {
		return nil
	}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, a := range f.Assets {
		if a.Name != model {
			continue
		}
		_, fields, _ := w.Resolved(f, a)
		for _, fl := range fields {
			if fl.Key != "skinOverride" {
				continue
			}
			for _, line := range strings.Split(gdt.Unquote(fl.Value), `\r\n`) {
				if p := strings.Fields(line); len(p) == 2 {
					out[strings.ToLower(p[0])] = strings.ToLower(p[1])
				}
			}
		}
	}
	return out
}

// stripCategory drops the techset category prefix the linker gives a packed
// material ("mc/mtl_x" -> "mtl_x").
func stripCategory(s string) string {
	if i := strings.IndexByte(s, '/'); i > 0 && i <= 3 {
		return s[i+1:]
	}
	return s
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
