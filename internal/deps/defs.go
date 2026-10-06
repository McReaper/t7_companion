package deps

import (
	"github.com/McReaper/t7_companion/internal/asset"
	"github.com/McReaper/t7_companion/internal/gdt"
)

// A GDT asset's definition is read from its whole GDT, and the GDTs a zone
// pulls from are many and large (stock ones run to megabytes): resolving
// assets one by one would parse the same files again and again. The graph
// resolves them a batch at a time instead (prefetch), parsing each GDT once
// per batch, and keeps every definition it has resolved.

// def is a GDT asset's first definition: its name, GDT type and effective
// fields (a derived asset's merged with its parents'); ok is false when no
// GDT defines it.
type def struct {
	name, typ string
	fields    []gdt.Field
	ok        bool
}

// lookup returns an asset's definition, resolving it if no batch did.
func (g *Graph) lookup(id asset.ID) def {
	g.prefetch([]asset.ID{id})
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.defs[id.Key()]
}

// prefetch resolves the definitions of the GDT assets among ids not resolved
// yet, parsing each GDT they live in once.
func (g *Graph) prefetch(ids []asset.ID) {
	byFile := map[string]map[int][]asset.ID{} // GDT -> line -> assets defined there
	for _, id := range ids {
		k := id.Key()
		g.mu.Lock()
		_, done := g.defs[k]
		g.mu.Unlock()
		if done || !g.w.IsGDTType(id.Type) {
			continue
		}
		locs, err := g.w.FindAsset(id)
		if err != nil || len(locs) == 0 {
			g.setDef(k, def{})
			continue
		}
		l := locs[0]
		if byFile[l.File] == nil {
			byFile[l.File] = map[int][]asset.ID{}
		}
		byFile[l.File][l.Line] = append(byFile[l.File][l.Line], k)
	}
	for file, lines := range byFile {
		g.resolveIn(file, lines)
	}
}

// resolveIn resolves the assets defined at lines of one GDT.
func (g *Graph) resolveIn(file string, lines map[int][]asset.ID) {
	f, err := g.w.Load(file)
	for _, a := range fileAssets(f, err) {
		keys, ok := lines[a.Line]
		if !ok {
			continue
		}
		typ, fields, err := g.w.Resolved(f, a)
		for _, k := range keys {
			g.setDef(k, def{name: a.Name, typ: typ, fields: fields, ok: err == nil})
		}
		delete(lines, a.Line)
	}
	for _, keys := range lines { // not found at their line: the GDT changed since it was indexed
		for _, k := range keys {
			g.setDef(k, def{})
		}
	}
}

func fileAssets(f *gdt.File, err error) []*gdt.Asset {
	if err != nil {
		return nil
	}
	return f.Assets
}

func (g *Graph) setDef(k asset.ID, d def) {
	g.mu.Lock()
	g.defs[k] = d
	g.mu.Unlock()
}
