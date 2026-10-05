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

// derived are packed types no source names: the linker makes them from their
// parent (an xmodel's meshes, a material's techset, an effect's lights), from
// the compiled map (its parts, volumes, texture combos, navigation…) or for
// the zone itself (string, assetlist).
var derived = map[string]bool{
	"xmodelmesh": true, "techset": true, "lightdescription": true, "string": true, "assetlist": true,
	"gfx_map": true, "col_map": true, "com_map": true, "game_map": true, "map_ents": true,
	"skyBox": true, "sstMinMaxCPU": true, "shadowTrees": true, "reflectionProbes": true, "cookieArray": true,
	"sunVolumes": true, "texturecombo": true, "navmesh": true, "navvolume": true, "glasses": true,
	"vertexData0": true, "vertexData1": true, "indices": true, "keyvaluepairs": true, "bgcache": true,
}

// The graph against what the linker packed for a linked map (T7KB_ORACLE_MAP,
// zm_test by default), two ways:
//
//   - edges: for each packed asset whose report parent is an asset or the
//     compiled map, whether the graph has that edge — per family (parent type
//     → child type), so a family the graph doesn't read yet stands out;
//   - closure: what the zone's lines and the map source pull in through the
//     graph, against what the report shows each pulling in (recall) and
//     against everything packed (precision: an asset the graph predicts that
//     the link didn't pack).
//
// Runs with T7KB_ORACLE=1 and TA_TOOLS_PATH.
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
	w, err := gdt.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	g := New(w, filepath.Dir(filepath.Dir(filepath.Dir(dir)))) // usermaps/<map>
	zf, err := zone.ZoneFile(root, name)
	if err != nil {
		t.Fatal(err)
	}
	zoneRoots, err := ZoneRoots(root, zf)
	if err != nil {
		t.Fatal(err)
	}
	var mapRoots []asset.ID
	if src, err := MapSource(root, name); err == nil {
		if mapRoots, err = g.MapRoots(src); err != nil {
			t.Fatal(err)
		}
	}
	scoreEdges(t, g, r, mapRoots)
	scoreClosure(t, g.Closure(append(zoneRoots, mapRoots...)), r)
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
		if len(p.Chain) == 0 || derived[p.Type] {
			continue
		}
		parent, child := zone.Canonical(p.Chain[0]), zone.Canonical(p.ID)
		switch {
		case parent.Type == "csv", parent.Key() == child.Key():
		case isBSP(parent):
			by.add(parent.Type+" → "+child.Type, fromMap[child.Key()], child.Name)
		case !derived[parent.Type]:
			by.add(parent.Type+" → "+child.Type, has(g.Children(parent), child), parent.Name+" → "+child.Name)
		}
	}
	report(t, "edges (parent type → child type)", by)
}

func scoreClosure(t *testing.T, c Closure, r *zone.Report) {
	zoneRecall, mapRecall := tally{}, tally{}
	packed := map[asset.ID]bool{}
	for _, p := range r.Assets {
		id := zone.Canonical(p.ID)
		packed[id.Key()] = true
		if derived[p.Type] || throughDerived(p.Chain) {
			continue
		}
		recall := zoneRecall
		if throughBSP(p.Chain) {
			recall = mapRecall
		}
		_, ok := c[id.Key()]
		recall.add(id.Type, ok, id.Name)
	}
	report(t, "closure recall, pulled in by the zone's lines", zoneRecall)
	report(t, "closure recall, pulled in by the map (BSP)", mapRecall)

	precision := tally{}
	for k, n := range c {
		if Packed(k.Type) {
			precision.add(k.Type, packed[k], n.From.String()+" → "+n.Name)
		}
	}
	report(t, "closure precision (predicted, and packed)", precision)
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

// isBSP: the compiled map's parts (gfx_map, col_map…), named after the .d3dbsp.
func isBSP(id asset.ID) bool { return strings.HasSuffix(id.Name, ".d3dbsp") }

func throughBSP(chain []asset.ID) bool {
	for _, c := range chain {
		if isBSP(c) {
			return true
		}
	}
	return false
}

// throughDerived: pulled in by something the linker made (a texture combo, sun
// volumes…), not by a source.
func throughDerived(chain []asset.ID) bool {
	for _, c := range chain {
		if derived[c.Type] && !isBSP(c) {
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
