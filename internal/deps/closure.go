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
// asset's From is its nearest puller.
func (g *Graph) Closure(roots []asset.ID) Closure {
	out := Closure{}
	var queue []asset.ID
	for _, r := range roots {
		if _, ok := out[r.Key()]; !ok {
			out[r.Key()] = Node{ID: r}
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, c := range g.Children(id) {
			if _, ok := out[c.Key()]; !ok {
				out[c.Key()] = Node{ID: c, From: id}
				queue = append(queue, c)
			}
		}
	}
	return out
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
