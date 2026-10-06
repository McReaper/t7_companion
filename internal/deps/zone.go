package deps

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// Zone is a map's (or mod's) graph with what it starts from: the lines of its
// zone file and what its map source's BSP pulls in.
type Zone struct {
	*Graph
	Lines     []asset.ID // the zone file's lines, its packages' included
	MapRoots  []asset.ID // what the map source pulls in (none for a mod)
	MapSource string     // map_source/<dir>/<name>.map, when there is one
	provided  zone.Assetlists
}

// OpenZone builds the graph of a map or mod under a mod-tools root, with its
// zone's stock lists: the ignore lists of its >class chain (SetStock) and,
// for telling what a source must define, its ignore_missing_shipped lists too.
func OpenZone(w *gdt.Workspace, name string) (*Zone, error) {
	zf, err := zone.ZoneFile(w.Root, name)
	if err != nil {
		return nil, err
	}
	inh, err := zone.Inherit(w.Root, zf)
	if err != nil {
		return nil, err
	}
	ignore, err := zone.LoadAssetlists(w.Root, inh.Ignore)
	if err != nil {
		return nil, err
	}
	provided, err := zone.LoadAssetlists(w.Root, append(append([]string{}, inh.Ignore...), inh.IgnoreMissingShipped...))
	if err != nil {
		return nil, err
	}
	z := &Zone{Graph: New(w, filepath.Dir(filepath.Dir(zf))), provided: provided}
	z.SetStock(ignore)
	if z.Lines, err = ZoneRoots(w.Root, zf); err != nil {
		return nil, err
	}
	if src, err := MapSource(w.Root, name); err == nil {
		z.MapSource = src
		if z.MapRoots, err = z.Graph.MapRoots(src); err != nil {
			return nil, err
		}
	}
	return z, nil
}

// Predict is everything the zone's lines and map source pull in.
func (z *Zone) Predict() Closure {
	return z.Closure(append(append([]asset.ID{}, z.Lines...), z.MapRoots...))
}

// Dangling is a reference to an asset no source defines.
type Dangling struct {
	asset.ID
	From asset.ID // what names it (none for a zone line or the map source)
}

// Dangling lists the assets of a closure that no GDT, file or stock list
// provides — assets of a type whose source the graph reads, not engine
// built-ins ($white…) — sorted.
func (z *Zone) Dangling(c Closure) []Dangling {
	var out []Dangling
	for _, n := range c {
		if strings.HasPrefix(n.Name, "$") || z.isProvided(n.ID) {
			continue
		}
		if defined, known := z.defined(n.ID); known && !defined {
			out = append(out, Dangling(n))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func (z *Zone) isProvided(id asset.ID) bool {
	for _, e := range z.provided.Lookup(id) {
		if e.Active {
			return true
		}
	}
	return false
}

// Masked is a version of a stock asset a non-stock GDT defines, which the
// build won't use: an active ignore list provides the asset, so the linker
// packs a reference to the shipped one.
type Masked struct {
	asset.ID
	Yours gdt.Location
	Stock zone.ListEntry
}

// Masked lists the assets of a closure that a non-stock GDT defines and an
// active ignore list of the zone provides, sorted.
func (z *Zone) Masked(c Closure) []Masked {
	var out []Masked
	for _, n := range c {
		e, ok := activeEntry(z.stock.Lookup(n.ID))
		if !ok || !z.w.IsGDTType(n.Type) {
			continue
		}
		locs, err := z.w.FindAsset(n.ID)
		if err != nil {
			continue
		}
		for _, l := range locs {
			if !l.Stock {
				out = append(out, Masked{ID: n.ID, Yours: l, Stock: e})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
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

// Root is the zone line or map-source root that first pulled an asset of a
// closure in, following From.
func (c Closure) Root(id asset.ID) asset.ID {
	n, ok := c[id.Key()]
	for ok && n.From.Name != "" {
		n, ok = c[n.From.Key()]
	}
	return n.ID
}

// Chain is how an asset of a closure was pulled in: the asset, then each
// asset that pulled the previous one, up to its root.
func (c Closure) Chain(id asset.ID) []asset.ID {
	var out []asset.ID
	for n, ok := c[id.Key()]; ok; n, ok = c[n.From.Key()] {
		out = append(out, n.ID)
		if n.From.Name == "" {
			break
		}
	}
	return out
}
