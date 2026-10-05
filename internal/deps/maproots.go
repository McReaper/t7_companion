package deps

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// MapSource is a map's Radiant source, map_source/<dir>/<name>.map.
func MapSource(root, name string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(root, "map_source", "*", name+".map"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no map source for %q under map_source/*/", name)
	}
	return matches[0], nil
}

// MapRoots returns what a map source pulls in through the BSP it compiles to:
// the models placed in it and in its prefabs (a misc_prefab's model, a .map
// under map_source/, read recursively; not a script_struct's, a preview), the assets entity classes name (an
// actor_<aitype> spawner, a zbarrier_<name>) or their keys (entityKeys: an fx
// entity's effect, the worldspawn's sky model and LUT…), and the materials its brushes
// and patches draw — not the Tools category's (clip, volumes, sky…), which
// are never drawn.
func (g *Graph) MapRoots(mapFile string) ([]asset.ID, error) {
	var s idSet
	err := g.mapRoots(mapFile, &s, map[string]bool{})
	return s.list, err
}

func (g *Graph) mapRoots(file string, out *idSet, seen map[string]bool) error {
	key := strings.ToLower(filepath.Clean(file))
	if seen[key] {
		return nil
	}
	seen[key] = true
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	ents, err := readMap(f)
	if err != nil {
		return err
	}
	for _, e := range ents {
		g.entityRoots(e, out, seen)
	}
	return nil
}

// entityKeys are the entity keys (lower-cased) naming an asset, by its type.
var entityKeys = map[string]string{
	"fxdef":                 "fx", // an fx entity's effect
	"destroyefx":            "fx", // a destructible's
	"rattleefx":             "fx",
	"physpreset":            "physpreset",
	"scriptbundlename":      "scriptbundle", // an fxanim's bundle
	"siege_anim":            "sanim",
	"modeloverridematerial": "material",
	"vehicletype":           "vehicle",
	"skyboxmodel":           "xmodel", // worldspawn
	"lutmaterial":           "material",
}

// entityRoots adds what one entity of a map source pulls in.
func (g *Graph) entityRoots(e mapEntity, out *idSet, seen map[string]bool) {
	class := e.keys["classname"]
	// *N: the entity's own brushes; a script_struct's model is only Radiant's preview
	if m := e.keys["model"]; m != "" && !strings.HasPrefix(m, "*") && class != "script_struct" {
		if strings.EqualFold(path.Ext(m), ".map") {
			_ = g.mapRoots(filepath.Join(g.w.Root, "map_source", filepath.FromSlash(m)), out, seen) // a missing prefab is cod2map's to report
		} else {
			out.add(asset.ID{Type: "xmodel", Name: m})
		}
	}
	if name, ok := strings.CutPrefix(class, "actor_"); ok {
		out.add(asset.ID{Type: "aitype", Name: name})
	}
	if name, ok := strings.CutPrefix(class, "zbarrier_"); ok {
		out.add(asset.ID{Type: "zbarrier", Name: name})
	}
	for key, typ := range entityKeys {
		if v := e.keys[key]; v != "" {
			out.add(efxRef(typ, v))
		}
	}
	for _, m := range e.materials {
		if !g.isTool(m) {
			out.add(asset.ID{Type: "material", Name: m})
		}
	}
}

// isTool reports whether a material is of the Tools category: drawn only in
// Radiant (clip, volumes, triggers…).
func (g *Graph) isTool(material string) bool {
	k := strings.ToLower(material)
	g.mu.Lock()
	tool, ok := g.tools[k]
	g.mu.Unlock()
	if ok {
		return tool
	}
	_, _, fields, found := g.resolve(g.w.FindAsset(asset.ID{Type: "material", Name: material}))
	tool = found && strings.EqualFold(fieldMap(fields)["materialcategory"], "Tools")
	g.mu.Lock()
	g.tools[k] = tool
	g.mu.Unlock()
	return tool
}
