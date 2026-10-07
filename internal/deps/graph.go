package deps

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
	"github.com/McReaper/t7_companion/internal/zone"
)

// Graph answers what an asset pulls in, caching each answer.
type Graph struct {
	w      *gdt.Workspace
	mapDir string
	mu     sync.Mutex
	kids   map[asset.ID][]asset.ID // by Key
	tools  map[string]bool         // lower-cased material -> of the Tools category
	stock  zone.Assetlists         // the zone's stock lists, when set (SetStock)

	defs map[asset.ID]def // GDT definitions resolved so far (defs.go)

	klfOnce sync.Once
	klfs    map[string][]string // lower-cased lens flare uuid -> its images
}

// New returns the graph of a workspace's assets as one map or mod links them:
// mapDir is its folder (usermaps/<name>), whose raw files the linker reads
// before share/raw's (animtables/, scripts/…: the same tree); empty for none.
func New(w *gdt.Workspace, mapDir string) *Graph {
	return &Graph{w: w, mapDir: mapDir, kids: map[asset.ID][]asset.ID{}, tools: map[string]bool{}, defs: map[asset.ID]def{}}
}

// raw finds a raw file, rel to share/raw, where the linker does: in the map's
// folder first.
func (g *Graph) raw(rel string) (string, bool) {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "share/raw/") // a zone line may name it from the root
	dirs := []string{filepath.Join(g.w.Root, "share", "raw")}
	if g.mapDir != "" {
		dirs = append([]string{g.mapDir}, dirs...)
	}
	for _, d := range dirs {
		p := filepath.Join(d, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// Children returns the assets id pulls in directly, each once.
func (g *Graph) Children(id asset.ID) []asset.ID {
	k := id.Key()
	g.mu.Lock()
	c, ok := g.kids[k]
	g.mu.Unlock()
	if ok {
		return c
	}
	var s idSet
	switch strings.ToLower(id.Type) {
	case "animmappingtable":
		g.mappingChildren(id.Name, &s)
	case "fx":
		g.fxChildren(id.Name, &s)
	case "klf":
		g.klfChildren(id.Name, &s)
	default:
		g.gdtChildren(id, &s)
	}
	g.mu.Lock()
	g.kids[k] = s.list
	g.mu.Unlock()
	return s.list
}

// gdtChildren adds what a GDT asset's fields name, and an xmodel's materials.
func (g *Graph) gdtChildren(id asset.ID, out *idSet) {
	d := g.lookup(id)
	name, typ, fields := d.name, d.typ, d.fields
	switch {
	case !d.ok:
	case typ == "weaponcamotable":
		g.camoTableChildren(fields, out)
	case typ == "attachmentcosmeticvariant":
		g.fieldChildren(typ, usedVariants(fields), out)
	case typ == "material" && g.materialChildren(fields, out):
	default:
		g.fieldChildren(typ, fields, out)
		switch typ {
		case "xmodel":
			g.modelChildren(name, fields, out)
		case "xanim":
			g.animChildren(fields, out)
		}
		if g.w.LinkerType(typ) == "weapon" { // every weapon carries the default cosmetic variant
			out.add(asset.ID{Type: "attachmentcosmeticvariant", Name: "defaultattachmentcosmeticvariant"})
		}
	}
}

// resolve reads the first definition found: its name, GDT type and effective
// fields (a derived asset's merged with its parents').
func (g *Graph) resolve(locs []gdt.Location, err error) (string, string, []gdt.Field, bool) {
	if err != nil || len(locs) == 0 {
		return "", "", nil, false
	}
	f, err := g.w.Load(locs[0].File)
	if err != nil {
		return "", "", nil, false
	}
	a := f.AtLine(locs[0].Line)
	if a == nil {
		return "", "", nil, false
	}
	typ, fields, err := g.w.Resolved(f, a)
	return a.Name, typ, fields, err == nil
}

// fieldChildren adds the assets fields name: typed references (AssetCombo)
// and file paths.
func (g *Graph) fieldChildren(typ string, fields []gdt.Field, out *idSet) {
	fields = setNotes(fields)
	for _, r := range g.w.Refs(typ, fields) {
		if !editorOnly[strings.ToLower(r.Field)] {
			out.add(g.refID(r))
		}
	}
	sc, _ := g.w.Schema(typ)
	for _, fl := range fields {
		g.fileChildren(sc, fl, out)
		if sc != nil {
			if e := sc.Lookup(fl.Key); e != nil && e.Varies {
				g.variesChild(e, gdt.Unquote(fl.Value), out)
			}
		}
	}
}

// noteParam is a note's parameter field (an xanim's customnote0actionparam1):
// the note's action field is the name up to "param".
var noteParam = regexp.MustCompile(`(?i)^(.*action)param\d+$`)

// setNotes drops the parameters of the notes whose action is None (or unset):
// APE keeps the last values, which nothing reads.
func setNotes(fields []gdt.Field) []gdt.Field {
	actions := fieldMap(fields)
	out := fields[:0:0]
	for _, f := range fields {
		if m := noteParam.FindStringSubmatch(f.Key); m != nil {
			if a := actions[strings.ToLower(m[1])]; a == "" || strings.EqualFold(a, "None") {
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

// editorOnly are the reference fields only the editors read, by their
// deffiles' tooltips: the models an xanim plays on and attaches in
// AssetViewer, a vehicle bundle's tag-dropdown model, the model Radiant shows
// for a spawner or a destructible.
var editorOnly = map[string]bool{"previewmodel": true, "previewattachmodel": true, "previewinclude": true, "radiantmodel": true}

// variesChild resolves a field the deffile declares with several asset types,
// one per script branch (an xanim note's actionparam1, by its action): the
// value names the one the GDTs define it as, when exactly one. A path to an
// effect is handled with the other file paths.
func (g *Graph) variesChild(e *gdt.Entry, v string, out *idSet) {
	if len(e.Alts) == 0 || v == "" {
		return
	}
	want := map[string]bool{}
	for _, alt := range e.Alts {
		want[g.w.LinkerType(alt)] = true
	}
	locs, err := g.w.Find(v)
	if err != nil {
		return
	}
	types := map[string]bool{}
	for _, l := range locs {
		if lt := g.w.LinkerType(g.w.TypeOf(l)); want[lt] {
			types[lt] = true
		}
	}
	if len(types) == 1 {
		for t := range types {
			out.add(asset.ID{Type: t, Name: v})
		}
	}
}

// refID is the asset a typed reference names. A deffile only drives APE's
// property page — the linker's converter has its own idea of each field — and
// one is sometimes wrong (projectileweapon.awi types lowReadyOutAnim as an
// xmodel): when no GDT defines the target as the declared type but they define
// it as exactly one other, that one is what the linker packs.
func (g *Graph) refID(r gdt.Ref) asset.ID {
	id := asset.ID{Type: g.w.LinkerType(r.Type), Name: r.Target}
	locs, err := g.w.Find(r.Target)
	if err != nil || len(locs) == 0 {
		return id
	}
	types := map[string]bool{}
	for _, l := range locs {
		if t := g.w.TypeOf(l); t != "" {
			types[g.w.LinkerType(t)] = true
		}
	}
	if types[id.Type] || len(types) != 1 {
		return id
	}
	for t := range types {
		id.Type = t
	}
	return id
}

// tableTypes are the AI animation files an aitype's fields name, by extension:
// the linker packs each under its file name.
var tableTypes = map[string]string{
	".ai_am":  "animmappingtable",
	".ai_asm": "animstatemachine",
	".ai_ast": "animselectortable",
	".ai_bt":  "behaviortree",
}

// fileChildren adds the asset a field's file path names: an effect is an fx
// named by its path under share/raw/fx without the extension (a path under
// share/raw may leave the .efx out), an AI table is packed under its file
// name, a path under pc/main is an image (a weapon's reticles), and a zone
// package (csvInclude) adds its lines.
func (g *Graph) fileChildren(sc *gdt.Schema, fl gdt.Field, out *idSet) {
	v := strings.ReplaceAll(gdt.Unquote(fl.Value), `\`, "/")
	if v == "" {
		return
	}
	rel := ""
	if sc != nil {
		if e := sc.Lookup(fl.Key); e != nil && (e.Kind == "Path" || e.Varies) {
			rel = strings.Trim(filepath.ToSlash(e.RelPath), "/")
		}
	}
	ext := strings.ToLower(path.Ext(v))
	switch {
	case ext == ".efx":
		name, _ := g.fxName(rel, v)
		out.add(asset.ID{Type: "fx", Name: name})
	case ext == "" && strings.HasPrefix(strings.ToLower(rel), "share/raw"):
		if name, ok := g.fxName(rel, v); ok {
			out.add(asset.ID{Type: "fx", Name: name})
		}
	case tableTypes[ext] != "":
		out.add(asset.ID{Type: tableTypes[ext], Name: v})
	case strings.EqualFold(rel, "pc/main"):
		out.add(asset.ID{Type: "image", Name: strings.TrimSuffix(path.Base(v), path.Ext(v))})
	case strings.EqualFold(rel, "share/zone_source"):
		g.packageChildren(v, out)
	}
}

// fxName is the fx an effect path names, and whether its .efx exists: its path
// under share/raw/fx without the extension. The path is relative to the
// field's directory (rel) when the field says one, or to share/raw/fx or
// share/raw: the first whose file exists wins.
func (g *Graph) fxName(rel, v string) (string, bool) {
	v = strings.TrimSuffix(v, path.Ext(v))
	var names []string
	for _, dir := range []string{rel, "share/raw/fx", "share/raw"} {
		if dir == "" {
			continue
		}
		if name, ok := strings.CutPrefix(path.Join(dir, v), "share/raw/fx/"); ok {
			if _, found := g.raw("fx/" + name + ".efx"); found {
				return name, true
			}
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return v, false
	}
	return names[0], false
}

// packageChildren adds the lines of share/zone_source/<name>.zpkg.
func (g *Graph) packageChildren(name string, out *idSet) {
	name = strings.TrimSuffix(name, ".zpkg")
	lines, err := zone.ZoneLines(g.w.Root, filepath.Join(g.w.Root, "share", "zone_source", name+".zpkg"))
	if err != nil {
		return
	}
	for _, l := range lines {
		out.add(zone.Canonical(l.ID))
	}
}

// modelChildren adds an xmodel's materials: those its LOD and collision files
// use, except those its skinOverride replaces (`from to` lines), and the
// replacements.
func (g *Graph) modelChildren(name string, fields []gdt.Field, out *idSet) {
	swaps := map[string]string{}
	for _, fl := range fields {
		if fl.Key != "skinOverride" {
			continue
		}
		for _, line := range strings.Split(gdt.Unquote(fl.Value), `\r\n`) { // the escape text, not a line break
			if from, to, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
				swaps[strings.ToLower(from)] = strings.TrimSpace(to)
			}
		}
	}
	mats, _ := g.w.ModelMaterials(name)
	for _, m := range mats {
		if to, ok := swaps[strings.ToLower(m)]; ok {
			m = to
		}
		out.add(asset.ID{Type: "material", Name: m})
	}
}

// idSet is a list of assets without repeats (compared by Key).
type idSet struct {
	seen map[asset.ID]bool
	list []asset.ID
}

func (s *idSet) add(id asset.ID) {
	if id.Name == "" {
		return
	}
	if s.seen == nil {
		s.seen = map[asset.ID]bool{}
	}
	if k := id.Key(); !s.seen[k] {
		s.seen[k] = true
		s.list = append(s.list, id)
	}
}
