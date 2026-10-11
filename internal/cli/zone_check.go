package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/zone"
)

const zoneMaxShadowed = 25

// shadowOut is a version of a stock asset that the build won't use.
type shadowOut struct {
	Asset string `json:"asset"`
	Yours string `json:"yours"`
	Stock string `json:"active_stock_line"`
}

// shadowGDT groups, per GDT, the assets the build takes from stock instead.
type shadowGDT struct {
	GDT      string   `json:"gdt"`
	Assets   int      `json:"assets"`
	Examples []string `json:"examples"`
	Lists    []string `json:"stock_lists"`
}

type zoneCheckResult struct {
	Map string `json:"map"`
	*zoneStatus
	Inherited zone.Inherited `json:"inherited"`
	Zoned     []shadowOut    `json:"zoned_but_stock_ships,omitempty"`
	GDTs      []shadowGDT    `json:"gdt_versions_not_used,omitempty"`
	More      int            `json:"more,omitempty"`
	Advice    string         `json:"advice,omitempty"`
}

// zoneCheck lists the assets you made your own version of — zoned in the map's
// .zone/.zpkg, or defined in a non-stock GDT the last link drew on — whose stock
// version ships instead, because an assetlist the map's >class marks `ignore`
// still lists them. Works before the first link (from the zone files) and
// better after (from the linker's report).
func zoneCheck(toolsPath, name string) (*zoneCheckResult, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	zf, err := zone.ZoneFile(root, name)
	if err != nil {
		return nil, err
	}
	inh, err := zone.Inherit(root, zf)
	if err != nil {
		return nil, err
	}
	lists, err := zone.LoadAssetlists(root, inh.Ignore)
	if err != nil {
		return nil, err
	}
	res := &zoneCheckResult{Map: name, Inherited: inh, Zoned: zonedShadows(root, zf, lists)}
	if len(res.Zoned) > zoneMaxShadowed {
		res.More += len(res.Zoned) - zoneMaxShadowed
		res.Zoned = res.Zoned[:zoneMaxShadowed]
	}
	if z, err := openZone(toolsPath, name); err == nil {
		res.zoneStatus = &z.status
		res.GDTs = groupByGDT(gdtShadows(z, lists))
		if len(res.GDTs) > zoneMaxShadowed {
			res.More += len(res.GDTs) - zoneMaxShadowed
			res.GDTs = res.GDTs[:zoneMaxShadowed]
		}
	}
	if len(res.Zoned)+len(res.GDTs) > 0 {
		res.Advice = "the stock version of these ships, not yours: harmless for a copy you never changed; to ship your " +
			"own, comment its active stock line out with // (in the BO3 root's zone_source/all/assetlist, not the map's) and relink"
	}
	return res, nil
}

// groupByGDT folds GDT shadows into one entry per GDT, largest first: a
// community pack often redefines dozens of stock assets.
func groupByGDT(shadows []shadowOut) []shadowGDT {
	by := map[string]*shadowGDT{}
	for _, s := range shadows {
		file, _, _ := strings.Cut(strings.TrimPrefix(s.Yours, "defined in "), ":")
		g := by[file]
		if g == nil {
			g = &shadowGDT{GDT: file}
			by[file] = g
		}
		g.Assets++
		if len(g.Examples) < 3 {
			g.Examples = append(g.Examples, s.Asset)
		}
		list := strings.TrimSuffix(strings.TrimPrefix(s.Stock[:strings.LastIndex(s.Stock, ":")], "zone_source/all/assetlist/"), ".csv")
		if !slices.Contains(g.Lists, list) {
			g.Lists = append(g.Lists, list)
		}
	}
	out := make([]shadowGDT, 0, len(by))
	for _, g := range by {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Assets != out[j].Assets {
			return out[i].Assets > out[j].Assets
		}
		return out[i].GDT < out[j].GDT
	})
	return out
}

// zonedShadows: lines of the map's zone files naming an asset an ignore list
// still provides — the classic stock script copied into the map and zoned.
func zonedShadows(root, zoneFile string, lists zone.Assetlists) []shadowOut {
	lines, err := zone.ZoneLines(root, zoneFile)
	if err != nil {
		return nil
	}
	var out []shadowOut
	for _, l := range lines {
		if e, ok := activeEntry(lists.Lookup(l.ID)); ok {
			out = append(out, shadowOut{Asset: l.String(), Yours: fmt.Sprintf("zoned at %s:%d", relTo(root, l.File), l.N), Stock: fmt.Sprintf("%s:%d", e.File(), e.Line)})
		}
	}
	return out
}

// gdtShadows: assets the last link packed only as a reference to the shipped
// one, that a non-stock GDT also defines.
func gdtShadows(z *zoneCtx, lists zone.Assetlists) []shadowOut {
	w, err := workspace(z.toolsPath)
	if err != nil {
		return nil
	}
	var out []shadowOut
	seen := map[asset.ID]bool{}
	for _, p := range z.report.Assets {
		id := zone.Canonical(p.ID) // the report names a material mc/x and a second packing x|dup
		e, ok := activeEntry(lists.Lookup(id))
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
				out = append(out, shadowOut{Asset: id.String(), Yours: fmt.Sprintf("defined in %s:%d", l.File, l.Line), Stock: fmt.Sprintf("%s:%d", e.File(), e.Line)})
			}
		}
	}
	return out
}

func activeEntry(entries []zone.ListEntry) (zone.ListEntry, bool) {
	for _, e := range entries {
		if e.Active {
			return e, true
		}
	}
	return zone.ListEntry{}, false
}

func relTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return p
}
