package cli

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapCheckOut is a map_check answer: how many findings of each kind, and the
// first of them, the worst kinds first.
type mapCheckOut struct {
	Map      string         `json:"map"`
	Counts   map[string]int `json:"counts"`
	Findings []findingOut   `json:"findings"`
	More     int            `json:"more,omitempty"` // findings past the limit
}

type findingOut struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Face     string `json:"face"`
	Material string `json:"material"`
	Detail   string `json:"detail"`
}

// findingOrder ranks the kinds, the worst first: what shows wrong in game,
// then what costs for nothing.
var findingOrder = map[string]int{"unknown": 0, "zfight": 1, "seam": 2, "stretched": 3, "hidden": 4}

// seeThrough are the usages of drawn materials a player sees past: a face
// behind one shows.
var seeThrough = map[string]bool{"window": true, "decal": true, "water": true, "foliage": true, "clip": true}

const (
	findingsLimit    = 50
	maxFindingsLimit = 200
)

// mapCheck checks the textures of the map open in Radiant: all its world
// brushes, or those ids names.
func mapCheck(ctx context.Context, ids []string, kinds string, limit, pid int) (*mapCheckOut, error) {
	if limit < 0 || limit > maxFindingsLimit {
		return nil, fmt.Errorf("limit %d: at most %d", limit, maxFindingsLimit)
	}
	want := map[string]bool{}
	for _, k := range strings.Split(kinds, ",") {
		if k = strings.TrimSpace(k); k == "" {
			continue
		}
		if _, ok := findingOrder[k]; !ok {
			return nil, fmt.Errorf("kind %q: unknown, zfight, seam, stretched or hidden", k)
		}
		want[k] = true
	}
	c, err := radiant.Connect(ctx, radiantMachine, pid)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	snap, err := level.Read(ctx, c)
	if err != nil {
		return nil, err
	}
	root := installOf(snap.State.Map)
	w, err := workspace(root)
	if err != nil {
		return nil, fmt.Errorf("the map's materials can't be looked up: %w", err)
	}
	found, err := level.CheckTextures(snap, ids, materialInfo(w, tilesIn(root)))
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(found, func(a, b level.Finding) int { return cmp.Compare(findingOrder[a.Kind], findingOrder[b.Kind]) })
	if limit == 0 {
		limit = findingsLimit
	}
	out := &mapCheckOut{Map: baseName(snap.State.Map), Counts: map[string]int{}, Findings: []findingOut{}}
	for _, f := range found {
		if len(want) > 0 && !want[f.Kind] {
			continue
		}
		out.Counts[f.Kind]++
		if len(out.Findings) == limit {
			out.More++
			continue
		}
		out.Findings = append(out.Findings, findingOut{Kind: f.Kind, ID: f.ID, Face: f.Face, Material: f.Material, Detail: f.Detail})
	}
	return out, nil
}

// materialInfo is what the checks know of a material, from the install's
// catalog: known when a GDT defines it, drawn unless its materialType is a
// tool's (caulk's tools_replace, clip's tools_blend: the script_ blockout
// textures are lit, though their usage is tools), opaque when drawn and not
// seen through, or caulk.
func materialInfo(w *gdt.Workspace, tiles level.Tiles) func(string) level.MaterialInfo {
	cat := map[string]gdt.EditorMaterial{}
	for _, m := range w.EditorMaterials() {
		cat[m.Name] = m
	}
	seen := map[string]level.MaterialInfo{}
	return func(m string) level.MaterialInfo {
		if mi, ok := seen[m]; ok {
			return mi
		}
		e, ok := cat[m]
		if !ok { // a model's material on a brush: drawn and opaque
			locs, _ := w.FindTyped(m, "material")
			ok = len(locs) > 0
		}
		drawn := !strings.HasPrefix(e.Type, "tools")
		mi := level.MaterialInfo{Known: ok, Drawn: drawn, Opaque: m == "caulk" || drawn && !seeThrough[e.Usage]}
		if tiles != nil {
			if t, err := tiles(m); err == nil {
				mi.Tile = t
			}
		}
		seen[m] = mi
		return mi
	}
}
