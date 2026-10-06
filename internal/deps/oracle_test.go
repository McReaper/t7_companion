package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// derivedTypes (lower-cased) are packed types no source names: the linker
// makes them from their parent (an xmodel's meshes, a material's techset, an
// effect's lights), from the compiled map (its parts, volumes, texture combos,
// navigation…) or for the zone itself (string, assetlist).
var derivedTypes = map[string]bool{
	"xmodelmesh": true, "techset": true, "lightdescription": true, "string": true, "assetlist": true,
	"gfx_map": true, "col_map": true, "com_map": true, "game_map": true, "map_ents": true,
	"skybox": true, "sstminmaxcpu": true, "shadowtrees": true, "reflectionprobes": true, "cookiearray": true,
	"sunvolumes": true, "texturecombo": true, "navmesh": true, "navvolume": true, "glasses": true,
	"vertexdata0": true, "vertexdata1": true, "indices": true, "keyvaluepairs": true, "bgcache": true,
}

func derived(typ string) bool { return derivedTypes[strings.ToLower(typ)] }

// The graph against what the linker loaded for a linked map (T7KB_ORACLE_MAP,
// zm_test by default):
//
//   - edges: for each packed asset whose report parent is an asset or the
//     compiled map, whether the graph has that edge — per family (parent type
//     → child type), so a family the graph doesn't read yet stands out;
//   - closure: what the zone's lines and the map source pull in through the
//     graph, against what the report shows each pulling in (recall) and
//     against everything the link loaded (precision);
//   - causes: each miss and each extra traced up to the first edge out of an
//     asset both sides have, so one wrong edge counts once with what it drags.
//
// Runs with T7KB_ORACLE=1 and TA_TOOLS_PATH; T7KB_ORACLE_EXAMPLES sets how
// many examples a line shows.
func TestGraphMatchesLinker(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" || os.Getenv("T7KB_ORACLE") == "" {
		t.Skip("T7KB_ORACLE and TA_TOOLS_PATH not set")
	}
	name := os.Getenv("T7KB_ORACLE_MAP")
	if name == "" {
		name = "zm_test"
	}
	dir, err := zone.ReportDir(root, name)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zone.Load(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	g, roots := oracleGraph(t, root, name, filepath.Dir(filepath.Dir(filepath.Dir(dir)))) // usermaps/<map>
	scoreEdges(t, g, r, roots.fromMap)
	c := g.Closure(append(roots.zone, roots.fromMap...))
	loaded := loadedBy(r)
	scoreClosure(t, c, loaded)
	reportCauses(t, "causes of misses", missCauses(c, loaded))
	reportCauses(t, "causes of extras", extraCauses(c, loaded))
}

type oracleRoots struct{ zone, fromMap []asset.ID }

// oracleGraph builds the graph of a map with its zone's stock lists, and the
// roots of its zone file and map source.
func oracleGraph(t *testing.T, root, name, mapDir string) (*Graph, oracleRoots) {
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	g := New(w, mapDir)
	zf, err := zone.ZoneFile(root, name)
	if err != nil {
		t.Fatal(err)
	}
	var roots oracleRoots
	if roots.zone, err = ZoneRoots(root, zf); err != nil {
		t.Fatal(err)
	}
	inh, err := zone.Inherit(root, zf)
	if err != nil {
		t.Fatal(err)
	}
	lists, err := zone.LoadAssetlists(root, inh.Ignore)
	if err != nil {
		t.Fatal(err)
	}
	g.SetStock(lists)
	if src, err := MapSource(root, name); err == nil {
		if roots.fromMap, err = g.MapRoots(src); err != nil {
			t.Fatal(err)
		}
	}
	return g, roots
}

type score struct {
	total, found int
	missed       []string
}

// examples is how many misses each line shows (T7KB_ORACLE_EXAMPLES, 3 by default).
var examples = func() int {
	if n, err := strconv.Atoi(os.Getenv("T7KB_ORACLE_EXAMPLES")); err == nil {
		return n
	}
	return 3
}()

func (s *score) add(ok bool, example string) {
	s.total++
	if ok {
		s.found++
	} else if len(s.missed) < examples {
		s.missed = append(s.missed, example)
	}
}

type tally map[string]*score

func (t tally) add(key string, ok bool, example string) {
	if t[key] == nil {
		t[key] = &score{}
	}
	t[key].add(ok, example)
}

func scoreEdges(t *testing.T, g *Graph, r *zone.Report, mapRoots []asset.ID) {
	fromMap := idKeys(mapRoots)
	by := tally{}
	for _, p := range r.Assets {
		if len(p.Chain) == 0 || derived(p.Type) {
			continue
		}
		parent, child := zone.Canonical(p.Chain[0]), zone.Canonical(p.ID)
		switch {
		case parent.Type == "csv", parent.Key() == child.Key():
		case isBSP(parent):
			by.add(parent.Type+" → "+child.Type, fromMap[child.Key()], child.Name)
		case !derived(parent.Type):
			by.add(parent.Type+" → "+child.Type, has(g.Children(parent), child), parent.Name+" → "+child.Name)
		}
	}
	report(t, "edges (parent type → child type)", by)
}

// counted: an asset the link loaded that the closure should predict — not one
// the linker or the light bake made.
func counted(k asset.ID, chain []asset.ID) bool {
	return !derived(k.Type) && k.Type != "csv" && !throughDerived(chain)
}

func scoreClosure(t *testing.T, c Closure, loaded map[asset.ID][]asset.ID) {
	zoneRecall, mapRecall := tally{}, tally{}
	for k, chain := range loaded {
		if !counted(k, chain) {
			continue
		}
		recall := zoneRecall
		if throughBSP(chain) {
			recall = mapRecall
		}
		_, ok := c[k]
		recall.add(k.Type, ok, k.Name)
	}
	report(t, "closure recall, pulled in by the zone's lines", zoneRecall)
	report(t, "closure recall, pulled in by the map (BSP)", mapRecall)

	precision := tally{}
	for k, n := range c {
		if !Packed(k.Type) { // not an asset of its own: the report may show it or not
			continue
		}
		_, ok := loaded[k]
		precision.add(k.Type, ok, n.From.String()+" → "+n.Name)
	}
	report(t, "closure precision (predicted, and loaded by the link)", precision)
}

// loadedBy is every asset the link loaded, by Key, with the chain that pulled
// it in: the report's lines, and the parents in their chains — the report has
// no line for an asset packed inside its parent (a customization table's body
// types) or for some it reached only through its children (a material whose
// techset and images it lists).
func loadedBy(r *zone.Report) map[asset.ID][]asset.ID {
	out := map[asset.ID][]asset.ID{}
	for _, p := range r.Assets {
		chain := make([]asset.ID, len(p.Chain))
		for i, c := range p.Chain {
			chain[i] = zone.Canonical(c)
		}
		out[zone.Canonical(p.ID).Key()] = chain
		for i, c := range chain {
			if _, ok := out[c.Key()]; !ok {
				out[c.Key()] = chain[i+1:]
			}
		}
	}
	return out
}

// missCauses groups the loaded assets the closure lacks by the first edge, up
// their report chain, out of an asset the closure has.
func missCauses(c Closure, loaded map[asset.ID][]asset.ID) map[string][]string {
	missing := map[asset.ID]bool{}
	for k, chain := range loaded {
		if _, ok := c[k]; !ok && counted(k, chain) {
			missing[k] = true
		}
	}
	causes := map[string][]string{}
	for k := range missing {
		child, chain := k, loaded[k]
		for _, a := range chain {
			if !missing[a.Key()] {
				causes[edge(a, child)] = append(causes[edge(a, child)], k.String())
				break
			}
			child = a
		}
	}
	return causes
}

// extraCauses groups the predicted assets the link didn't load by the first
// edge, up the closure, out of an asset it did load (or the root).
func extraCauses(c Closure, loaded map[asset.ID][]asset.ID) map[string][]string {
	causes := map[string][]string{}
	for k, n := range c {
		if _, ok := loaded[k]; ok || !Packed(k.Type) {
			continue
		}
		cur := n
		for cur.From.Name != "" {
			if _, ok := loaded[cur.From.Key()]; ok {
				break
			}
			cur = c[cur.From.Key()]
		}
		causes[edge(cur.From, cur.ID)] = append(causes[edge(cur.From, cur.ID)], k.String())
	}
	return causes
}

func edge(from, to asset.ID) string {
	if from.Name == "" {
		return "(root) → " + to.String()
	}
	return from.Type + " → " + to.Type + "  [" + from.Name + " → " + to.Name + "]"
}

func reportCauses(t *testing.T, title string, causes map[string][]string) {
	keys := make([]string, 0, len(causes))
	total := 0
	for k, v := range causes {
		keys = append(keys, k)
		total += len(v)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(causes[keys[i]]) != len(causes[keys[j]]) {
			return len(causes[keys[i]]) > len(causes[keys[j]])
		}
		return keys[i] < keys[j]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d assets, %d edges (each with what it drags)\n", title, total, len(keys))
	for _, k := range keys {
		fmt.Fprintf(&b, "  %4d  %s\n", len(causes[k]), k)
	}
	t.Log(b.String())
}

func idKeys(ids []asset.ID) map[asset.ID]bool {
	out := make(map[asset.ID]bool, len(ids))
	for _, id := range ids {
		out[id.Key()] = true
	}
	return out
}

func has(ids []asset.ID, want asset.ID) bool {
	return idKeys(ids)[want.Key()]
}

// isBSP: the compiled map's parts that place what its source names (gfx_map
// draws, col_map collides and spawns), named after the .d3dbsp. Its other
// parts (sun volumes, reflection probes…) carry what the light bake made.
func isBSP(id asset.ID) bool {
	return (id.Type == "gfx_map" || id.Type == "col_map") && strings.HasSuffix(id.Name, ".d3dbsp")
}

func throughBSP(chain []asset.ID) bool {
	for _, c := range chain {
		if isBSP(c) {
			return true
		}
	}
	return false
}

// throughDerived: pulled in by something the linker or the light bake made (a
// texture combo, sun volume probes…), not by a source.
func throughDerived(chain []asset.ID) bool {
	for _, c := range chain {
		if derived(c.Type) && !isBSP(c) {
			return true
		}
	}
	return false
}

func report(t *testing.T, title string, by tally) {
	keys := make([]string, 0, len(by))
	total, found := 0, 0
	for k, s := range by {
		keys = append(keys, k)
		total += s.total
		found += s.found
	}
	sort.Slice(keys, func(i, j int) bool {
		mi, mj := by[keys[i]].total-by[keys[i]].found, by[keys[j]].total-by[keys[j]].found
		if mi != mj {
			return mi > mj
		}
		return keys[i] < keys[j]
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d/%d (%.1f%%)\n", title, found, total, pct(found, total))
	for _, k := range keys {
		s := by[k]
		fmt.Fprintf(&b, "  %-40s %6d/%-6d %5.1f%%  %s\n", k, s.found, s.total, pct(s.found, s.total), strings.Join(s.missed, " | "))
	}
	t.Log(b.String())
}

func pct(a, b int) float64 {
	if b == 0 {
		return 100
	}
	return 100 * float64(a) / float64(b)
}
