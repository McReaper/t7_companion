package gdt

import (
	"slices"
	"strings"
)

// Model is an xmodel asset: what a misc_model places by its model key.
type Model struct {
	Name  string
	File  string // its GDT, relative to the root
	Stock bool   // a GDT Treyarch shipped
}

// Models lists the xmodels of the install's GDTs by name, read from the
// index: a derived asset is one when its parent chain in its GDT is.
func (w *Workspace) Models() []Model {
	w.refresh()
	if w.idx == nil {
		return nil
	}
	w.idx.mu.RLock()
	defer w.idx.mu.RUnlock()
	var out []Model
	for rel, fe := range w.idx.files {
		byName := make(map[string]*hit, len(fe.Assets))
		for i := range fe.Assets {
			byName[fe.Assets[i].Name] = &fe.Assets[i]
		}
		for i := range fe.Assets {
			if typeOf(&fe.Assets[i], byName) == "xmodel" {
				out = append(out, Model{Name: fe.Assets[i].Name, File: rel, Stock: w.stock[strings.ToLower(rel)]})
			}
		}
	}
	slices.SortFunc(out, func(a, b Model) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// typeOf is an asset's type, its first ancestor's in its GDT that has one.
func typeOf(h *hit, byName map[string]*hit) string {
	seen := map[string]bool{}
	for cur := h; cur != nil && !seen[cur.Name]; cur = byName[cur.Parent] {
		seen[cur.Name] = true
		if cur.Type != "" {
			return cur.Type
		}
	}
	return ""
}
