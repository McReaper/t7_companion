package gdt

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Workspace is a BO3 mod-tools root: the deffiles, the techsetdefs, and every
// GDT under it.
type Workspace struct {
	Root     string
	Deffiles string
	Techsets *Techsets

	once   sync.Once
	idx    *index
	stock  map[string]bool // lower-cased relative paths listed in stock.gdtdef
	scErr  error
	schema sync.Map // type -> *Schema
}

// skipDirs are never descended into inside the GDT directories: VCS data and backups.
var skipDirs = map[string]bool{".git": true, "backup": true, "_backups": true}

// Open prepares a workspace rooted at the mod-tools install.
func Open(root string) (*Workspace, error) {
	w := &Workspace{Root: root, Deffiles: filepath.Join(root, "deffiles")}
	ts, err := OpenTechsets(filepath.Join(root, "share", "raw", "techsetdefs_stable"))
	if err != nil {
		return nil, err
	}
	w.Techsets = ts
	return w, nil
}

func (w *Workspace) scan() {
	w.once.Do(func() {
		w.stock = map[string]bool{}
		if f, err := os.Open(filepath.Join(w.Root, "stock.gdtdef")); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if l := strings.TrimSpace(sc.Text()); l != "" {
					w.stock[strings.ToLower(filepath.ToSlash(l))] = true
				}
			}
			f.Close()
		}
		w.idx, w.scErr = loadIndex(w.Root)
	})
}

// Warm builds or refreshes the asset index in the background, so the first
// lookup doesn't pay for it (the MCP server calls this at startup).
func (w *Workspace) Warm() { go w.scan() }

// Rel returns a path relative to the root, with forward slashes.
func (w *Workspace) Rel(p string) string {
	r, err := filepath.Rel(w.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// IsStock reports whether a GDT is one Treyarch shipped (listed in stock.gdtdef).
func (w *Workspace) IsStock(path string) bool {
	w.scan()
	return w.stock[strings.ToLower(w.Rel(path))]
}

// Schema loads (and caches) an asset type's deffile.
func (w *Workspace) Schema(assetType string) (*Schema, error) {
	if s, ok := w.schema.Load(assetType); ok {
		return s.(*Schema), nil
	}
	s, err := LoadSchema(w.Deffiles, assetType)
	if err != nil {
		return nil, err
	}
	w.schema.Store(assetType, s)
	return s, nil
}

// Location is one place an asset name is defined.
type Location struct {
	File   string `json:"file"` // relative to the root
	Line   int    `json:"line"`
	Type   string `json:"type,omitempty"`
	Parent string `json:"parent,omitempty"`
	Stock  bool   `json:"stock"`
}

// Find returns every definition of an asset name across the workspace's GDTs.
// More than one is the linker's `Duplicate '<type>' asset` error waiting to happen.
func (w *Workspace) Find(name string) ([]Location, error) {
	w.scan()
	if w.scErr != nil {
		return nil, w.scErr
	}
	var out []Location
	for _, h := range w.idx.lookup(name) {
		out = append(out, Location{File: h.File, Line: h.Line, Type: h.Type, Parent: h.Parent, Stock: w.stock[strings.ToLower(h.File)]})
	}
	return out, nil
}

// Touched re-indexes one GDT after it was written.
func (w *Workspace) Touched(path string) {
	w.scan()
	if w.idx != nil {
		w.idx.refreshFile(w.Root, w.Rel(path))
	}
}

// Load parses a GDT given relative to the root (or absolute).
func (w *Workspace) Load(file string) (*File, error) {
	return ParseFile(w.Abs(file))
}

// Abs resolves a root-relative path.
func (w *Workspace) Abs(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(w.Root, filepath.FromSlash(file))
}

// Resolved returns an asset's effective type and fields, walking the derivation
// chain ("child" [ "parent" ]) so inherited values are visible; own fields win.
func (w *Workspace) Resolved(a *Asset) (string, []Field, error) {
	var chain []*Asset
	seen := map[string]bool{}
	cur := a
	for cur != nil && cur.Parent != "" && !seen[cur.Name] {
		seen[cur.Name] = true
		chain = append(chain, cur)
		locs, err := w.Find(cur.Parent)
		if err != nil || len(locs) == 0 {
			break
		}
		f, err := w.Load(locs[0].File)
		if err != nil {
			break
		}
		cur = f.Find(cur.Parent)
	}
	if cur != nil && cur.Parent == "" {
		chain = append(chain, cur)
	}
	typ := ""
	merged := map[string]string{}
	var order []string
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Type != "" {
			typ = chain[i].Type
		}
		for _, f := range chain[i].Fields {
			if _, ok := merged[f.Key]; !ok {
				order = append(order, f.Key)
			}
			merged[f.Key] = f.Value
		}
	}
	out := make([]Field, 0, len(order))
	for _, k := range order {
		out = append(out, Field{Key: k, Value: merged[k]})
	}
	return typ, out, nil
}
