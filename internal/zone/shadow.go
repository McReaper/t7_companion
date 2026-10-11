package zone

import (
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// A Shadow is your own version of a stock asset that the build doesn't use:
// an active line of an assetlist the map's >class marks `ignore` makes the
// linker pack only a reference to the shipped asset.
type Shadow struct {
	asset.ID
	File  string    // your version: a zone file relative to the root (ZonedShadows), or a GDT as the gdt package names it (GDTShadows)
	Line  int       // its line there
	Stock ListEntry // the active line that ships the stock asset
}

// IgnoreLists loads the stock assetlists zoneFile's >class chain marks
// `ignore`: for their active entries the linker packs only a reference to the
// shipped asset. Its ignore_missing_shipped lists are packed in full, so they
// shadow nothing.
func IgnoreLists(root, zoneFile string) (Assetlists, Inherited, error) {
	inh, err := Inherit(root, zoneFile)
	if err != nil {
		return nil, inh, err
	}
	lists, err := LoadAssetlists(root, inh.Ignore)
	return lists, inh, err
}

// Active returns the entry that ships r's stock version, if one does.
func (a Assetlists) Active(r asset.ID) (ListEntry, bool) {
	for _, e := range a.Lookup(r) {
		if e.Active {
			return e, true
		}
	}
	return ListEntry{}, false
}

// ZonedShadows lists the lines of a map's zone files naming an asset an ignore
// list still ships: the classic stock script copied into the map and zoned.
// It needs no link.
func ZonedShadows(root, zoneFile string, lists Assetlists) []Shadow {
	lines, err := ZoneLines(root, zoneFile)
	if err != nil {
		return nil
	}
	var out []Shadow
	for _, l := range lines {
		if e, ok := lists.Active(l.ID); ok {
			out = append(out, Shadow{ID: l.ID, File: relTo(root, l.File), Line: l.N, Stock: e})
		}
	}
	return out
}

// GDTShadows lists the assets the last link packed only as a reference to the
// shipped one that a non-stock GDT of w also defines, once per asset and GDT
// line.
func GDTShadows(r *Report, w *gdt.Workspace, lists Assetlists) []Shadow {
	var out []Shadow
	seen := map[asset.ID]bool{}
	for _, p := range r.Assets {
		id := Canonical(p.ID) // the report names a material mc/x and a second packing x|dup
		e, ok := lists.Active(id)
		if !ok || seen[id.Key()] {
			continue
		}
		seen[id.Key()] = true
		locs, err := w.FindAsset(id) // by the type the linker packs it as: a bulletweapon is a weapon
		if err != nil {
			continue
		}
		for _, l := range locs {
			if !l.Stock {
				out = append(out, Shadow{ID: id, File: l.File, Line: l.Line, Stock: e})
			}
		}
	}
	return out
}

// relTo is p relative to root, with forward slashes, when p is under root.
func relTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
