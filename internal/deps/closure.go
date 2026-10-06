package deps

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/zone"
)

// unpacked are types the graph walks through that the linker doesn't pack as
// assets of their own: the sub-assets it writes inside the asset referencing
// them (a customization table's body types, their body and helmet styles), and
// localize lines, whose strings join the zone's.
var unpacked = map[string]bool{"playerbodytype": true, "playerbodystyle": true, "playerhelmetstyle": true, "localize": true}

// Packed reports whether the linker packs an asset of type typ as an asset of
// its own.
func Packed(typ string) bool { return !unpacked[strings.ToLower(typ)] }

// Node is an asset of a closure and the asset that first pulled it in (none for
// a root).
type Node struct {
	asset.ID
	From asset.ID
}

// Closure is everything some roots pull in, by Key.
type Closure map[asset.ID]Node

// Closure follows roots down to everything they pull in, breadth first, so an
// asset's From is its nearest puller. With a zone's stock lists (SetStock), a
// script one of its active ignore lists provides is left out: the linker
// packs nothing for it, the shipped one runs. Every other stock asset is
// packed as a reference to the shipped one, and its children are followed.
func (g *Graph) Closure(roots []asset.ID) Closure {
	out := Closure{}
	var queue []asset.ID
	visit := func(id, from asset.ID) {
		if _, ok := out[id.Key()]; ok || g.shipped(id) {
			return
		}
		out[id.Key()] = Node{ID: id, From: from}
		queue = append(queue, id)
	}
	for _, r := range roots {
		visit(r, asset.ID{})
	}
	for len(queue) > 0 { // a level at a time, its GDT definitions resolved in one batch
		level := queue
		queue = nil
		g.prefetch(level)
		for _, id := range level {
			for _, c := range g.Children(id) {
				visit(c, id)
			}
		}
	}
	return out
}

// SetStock gives the graph a zone's stock lists: the assets its active ignore
// lists provide (zone.Inherit, zone.LoadAssetlists).
func (g *Graph) SetStock(lists zone.Assetlists) { g.stock = lists }

// shipped: a script an active ignore list of the zone provides.
func (g *Graph) shipped(id asset.ID) bool {
	if g.stock == nil || !strings.EqualFold(id.Type, "scriptparsetree") {
		return false
	}
	_, active := activeEntry(g.stock.Lookup(id))
	return active
}

// ZoneRoots returns the assets a zone file lists, its packages' lines included,
// named as the sources name them (zone.Canonical).
func ZoneRoots(root, zoneFile string) ([]asset.ID, error) {
	lines, err := zone.ZoneLines(root, zoneFile)
	if err != nil {
		return nil, err
	}
	out := make([]asset.ID, 0, len(lines))
	for _, l := range lines {
		out = append(out, zone.Canonical(l.ID))
	}
	return out, nil
}
