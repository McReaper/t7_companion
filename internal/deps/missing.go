package deps

import (
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// rawTypes are the types a raw file defines, by the directory (under the map's
// folder or share/raw) its name is relative to; "" for a name that is the
// file's own path (a zone line's scripts/zm/x.gsc).
var rawTypes = map[string]string{
	"animmappingtable":  "animtables",
	"animselectortable": "animtables",
	"animstatemachine":  "animstatemachines",
	"behaviortree":      "behavior",
	"scriptparsetree":   "",
	"rawfile":           "",
	"stringtable":       "",
	"structuredtable":   "",
	"ttf":               "",
}

// defined reports whether a source defines an asset — a GDT, an .efx, a raw
// file, a lens flare — and whether the graph can tell at all (a type whose
// source it doesn't read, sound or localize, can't be checked).
func (g *Graph) defined(id asset.ID) (defined, known bool) {
	t := strings.ToLower(id.Type)
	if engineAsset(id) {
		return true, true
	}
	if dir, ok := rawTypes[t]; ok {
		_, found := g.raw(filepath.ToSlash(filepath.Join(dir, id.Name)))
		return found, true
	}
	switch {
	case t == "fx":
		_, found := g.raw("fx/" + id.Name + ".efx")
		return found, true
	case t == "klf":
		g.klfOnce.Do(g.loadKlfs)
		_, found := g.klfs[strings.ToLower(id.Name)]
		return found, true
	case g.w.IsGDTType(t):
		locs, err := g.w.FindAsset(id)
		return err == nil && len(locs) > 0, err == nil
	}
	return false, false
}

// engineAsset: what every compiled map packs (everyMap) needs no source.
func engineAsset(id asset.ID) bool {
	for _, e := range everyMap {
		if e.Key() == id.Key() {
			return true
		}
	}
	return false
}
