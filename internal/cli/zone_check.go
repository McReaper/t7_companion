package cli

import (
	"fmt"
	"slices"
	"sort"

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
	root := toolsRoot(toolsPath)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	zf, err := zone.ZoneFile(root, name)
	if err != nil {
		return nil, err
	}
	lists, inh, err := zone.IgnoreLists(root, zf)
	if err != nil {
		return nil, err
	}
	res := &zoneCheckResult{Map: name, Inherited: inh}
	for _, s := range zone.ZonedShadows(root, zf, lists) {
		res.Zoned = append(res.Zoned, toShadowOut(s))
	}
	if len(res.Zoned) > zoneMaxShadowed {
		res.More += len(res.Zoned) - zoneMaxShadowed
		res.Zoned = res.Zoned[:zoneMaxShadowed]
	}
	if z, err := openZone(toolsPath, name); err == nil {
		res.zoneStatus = &z.status
		if w, err := workspace(toolsPath); err == nil {
			res.GDTs = groupByGDT(zone.GDTShadows(z.report, w, lists))
		}
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

func toShadowOut(s zone.Shadow) shadowOut {
	where := "zoned at"
	if s.InGDT {
		where = "defined in"
	}
	return shadowOut{Asset: s.String(), Yours: fmt.Sprintf("%s %s:%d", where, s.File, s.Line), Stock: fmt.Sprintf("%s:%d", s.Stock.File(), s.Stock.Line)}
}

// groupByGDT folds GDT shadows into one entry per GDT, largest first: a
// community pack often redefines dozens of stock assets.
func groupByGDT(shadows []zone.Shadow) []shadowGDT {
	by := map[string]*shadowGDT{}
	for _, s := range shadows {
		g := by[s.File]
		if g == nil {
			g = &shadowGDT{GDT: s.File}
			by[s.File] = g
		}
		g.Assets++
		if len(g.Examples) < 3 {
			g.Examples = append(g.Examples, s.String())
		}
		if !slices.Contains(g.Lists, s.Stock.List) {
			g.Lists = append(g.Lists, s.Stock.List)
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
