package zone

import (
	"os"
	"sort"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// ChangedSince lists the files that changed after r's link, so that r may no
// longer describe the sources: those the link read (Changed, from .deps), then
// the non-stock GDTs of w defining a packed asset that were saved after it —
// the linker records a GDT asset by hash, not by file. A nil w checks .deps
// alone.
func ChangedSince(r *Report, w *gdt.Workspace) []string {
	changed, _ := Changed(r.Dir, r.Zone)
	if w == nil {
		return changed
	}
	seen := map[string]bool{}
	var gdts []string
	for _, p := range r.Assets {
		locs, err := w.Find(Canonical(p.ID).Name) // the report names a material mc/x
		if err != nil {
			continue
		}
		for _, l := range locs {
			if seen[l.File] {
				continue
			}
			seen[l.File] = true
			if newerThanLink(w, l, r) {
				gdts = append(gdts, w.Abs(l.File))
			}
		}
	}
	sort.Strings(gdts)
	return append(changed, gdts...)
}

func newerThanLink(w *gdt.Workspace, l gdt.Location, r *Report) bool {
	if l.Stock {
		return false
	}
	st, err := os.Stat(w.Abs(l.File))
	return err == nil && st.ModTime().After(r.Linked)
}
