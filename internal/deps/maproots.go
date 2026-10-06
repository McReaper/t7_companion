package deps

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	for _, id := range everyMap {
		s.add(id)
	}
	err := g.mapRoots(mapFile, &s, map[string]bool{})
	return s.list, err
}

// everyMap is what every compiled map pulls in, whatever its source holds.
var everyMap = []asset.ID{
	{Type: "image", Name: "vdReveal"},
	{Type: "material", Name: "shadowcaster"},
	{Type: "xmodel", Name: "skybox_default_black"},
}

// sunKey: an entity key naming the sun settings (ssi) of a lighting state —
// ssi, ssi1…ssi4 and their _runtime_override.
var sunKey = regexp.MustCompile(`^ssi\d*(_runtime_override)?$`)

func (g *Graph) mapRoots(file string, out *idSet, seen map[string]bool) error {
	key := strings.ToLower(filepath.Clean(file))
	if seen[key] {
		return nil
	}
	top := len(seen) == 0
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
		if !top && e.keys["classname"] == "worldspawn" {
			g.drawn(e.materials, out) // a prefab's world brushes are compiled, not its settings
			continue
		}
		g.entityRoots(e, out, seen)
	}
	return nil
}

// entityKeys are the entity keys (lower-cased) naming an asset, by its type.
var entityKeys = map[string]string{
	"fxdef":                 "fx", // an fx entity's effect
	"destroyefx":            "fx", // a destructible's
	"rattleefx":             "fx",
	"destroyedmodel":        "xmodel", // a dyn_model's broken version
	"physpreset":            "physpreset",
	"scriptbundlename":      "scriptbundle", // an fxanim's bundle
	"siege_anim":            "sanim",
	"modeloverridematerial": "material",
	"vehicletype":           "vehicle",
	"skyboxmodel":           "xmodel", // worldspawn
	"lutmaterial":           "material",
	"weathercolormap":       "image", // a weather grime volume's
	"weathercolormap2":      "image",
	"weatherglossmap":       "image",
	"weatherglossmap2":      "image",
	"weathernormalmap":      "image",
	"weathernormalmap2":     "image",
}

// entityRoots adds what one entity of a map source pulls in.
func (g *Graph) entityRoots(e mapEntity, out *idSet, seen map[string]bool) {
	g.modelRoots(e, out, seen)
	g.classRoots(e, out)
	for key, v := range e.keys {
		switch {
		case v == "":
		case entityKeys[key] != "":
			out.add(efxRef(entityKeys[key], v))
		case sunKey.MatchString(key):
			// the compiled map bakes a lighting state's sun settings in: what
			// they name (the sky model) is packed, not the ssi bundle itself
			if _, typ, fields, ok := g.resolve(g.w.FindTyped(v, "ssi")); ok {
				g.fieldChildren(typ, fields, out)
			}
		}
	}
	g.drawn(e.materials, out)
}

// modelRoots adds an entity's model, or a prefab's contents. A script_struct's
// model and a spawner's (actor_…) are only Radiant's preview: the struct
// exists for scripts, the aitype says what spawns. *N names the entity's own
// brushes.
func (g *Graph) modelRoots(e mapEntity, out *idSet, seen map[string]bool) {
	m, class := e.keys["model"], e.keys["classname"]
	switch {
	case m == "", strings.HasPrefix(m, "*"), class == "script_struct", strings.HasPrefix(class, "actor_"):
	case strings.EqualFold(path.Ext(m), ".map"):
		_ = g.mapRoots(filepath.Join(g.w.Root, "map_source", filepath.FromSlash(m)), out, seen) // a missing prefab is cod2map's to report
	default:
		out.add(asset.ID{Type: "xmodel", Name: m})
	}
}

// classRoots adds what an entity's class names: an actor_<aitype> spawner, a
// zbarrier_<name>, a glass's type, a physics dyn_model's default preset.
func (g *Graph) classRoots(e mapEntity, out *idSet) {
	class := e.keys["classname"]
	if name, ok := strings.CutPrefix(class, "actor_"); ok {
		out.add(asset.ID{Type: "aitype", Name: name})
	}
	if name, ok := strings.CutPrefix(class, "zbarrier_"); ok {
		out.add(asset.ID{Type: "zbarrier", Name: name})
	}
	if class == "glass" {
		g.glassChildren(e.keys["type"], out)
	}
	if class == "dyn_model" && e.keys["use_physics"] == "1" && e.keys["physpreset"] == "" {
		g.defaultPhyspreset(e.keys["model"], out)
	}
}

// drawn adds the materials brushes and patches draw (not the Tools category's).
func (g *Graph) drawn(materials []string, out *idSet) {
	for _, m := range materials {
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

// defaultPhyspreset: a physics dyn_model with no physPreset key takes its
// model's physicsPreset; when the model has none either, it asks for a
// physpreset named after the model, which the linker packs empty (0 bytes)
// when no GDT defines it.
func (g *Graph) defaultPhyspreset(model string, out *idSet) {
	if model == "" {
		return
	}
	if _, _, fields, ok := g.resolve(g.w.FindAsset(asset.ID{Type: "xmodel", Name: model})); ok && fieldMap(fields)["physicspreset"] != "" {
		return // the model's own preset, an xmodel edge
	}
	out.add(asset.ID{Type: "physpreset", Name: model})
}
