package gdt

import (
	"cmp"
	"strings"
)

// EditorMaterial is a material Radiant's texture browser shows: its usage,
// the filter it is under there, is one other than <not in editor> (the
// default).
type EditorMaterial struct {
	Name     string
	Usage    string // floor, interior wall, exterior trim, decal, tools…
	Surface  string // its surfaceType: brick, wood, concrete…
	ColorMap string // its color image
	File     string // its GDT, relative to the root
	Stock    bool   // a GDT Treyarch shipped
	DoNotUse bool   // marked DO NOT USE: Radiant crosses it out (script_wall among them)
	Type     string // its materialType: lit…, tools_replace for caulk, tools_blend for clip
}

// EditorMaterials lists the materials Radiant's texture browser shows, read
// from the index: a derived material's fields come from its parent chain in
// its GDT, its own lines first.
func (w *Workspace) EditorMaterials() []EditorMaterial {
	w.refresh()
	if w.idx == nil {
		return nil
	}
	w.idx.mu.RLock()
	defer w.idx.mu.RUnlock()
	var out []EditorMaterial
	for rel, fe := range w.idx.files {
		byName := make(map[string]*hit, len(fe.Assets))
		for i := range fe.Assets {
			byName[fe.Assets[i].Name] = &fe.Assets[i]
		}
		for i := range fe.Assets {
			m, ok := editorMaterial(&fe.Assets[i], byName)
			if !ok {
				continue
			}
			m.File, m.Stock = rel, w.stock[strings.ToLower(rel)]
			out = append(out, m)
		}
	}
	return out
}

// editorMaterial reads one asset of a GDT as a material of the editor's,
// walking its parents in the same GDT (gdtdb's rule) for what it doesn't set.
func editorMaterial(h *hit, byName map[string]*hit) (EditorMaterial, bool) {
	var f matFields
	typ := ""
	seen := map[string]bool{}
	for cur := h; cur != nil && !seen[cur.Name]; cur = byName[cur.Parent] {
		seen[cur.Name] = true
		if cur.Mat != nil {
			f.Usage = cmp.Or(f.Usage, cur.Mat.Usage)
			f.Surface = cmp.Or(f.Surface, cur.Mat.Surface)
			f.Color = cmp.Or(f.Color, cur.Mat.Color)
			f.DoNotUse = cmp.Or(f.DoNotUse, cur.Mat.DoNotUse)
			f.Type = cmp.Or(f.Type, cur.Mat.Type)
		}
		if cur.Type != "" {
			typ = cur.Type
			break
		}
	}
	if typ != "material" || f.Usage == "" || f.Usage == "<not in editor>" {
		return EditorMaterial{}, false
	}
	return EditorMaterial{Name: h.Name, Usage: f.Usage, Surface: f.Surface, ColorMap: f.Color, DoNotUse: f.DoNotUse == "1", Type: f.Type}, true
}
