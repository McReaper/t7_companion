package gdt

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Workspace is a BO3 mod-tools root: the deffiles, the techsetdefs, and every
// GDT under it.
type Workspace struct {
	Root     string
	Deffiles string
	Techsets *Techsets

	once sync.Once
	idx  *index

	refreshMu   sync.Mutex
	lastStat    time.Time // last cheap re-stat of indexed files
	lastWalk    time.Time // last full walk (finds new GDTs)
	walkRunning bool
	stock       map[string]bool // lower-cased relative paths listed in stock.gdtdef
	scErr       error
	schema      sync.Map // type -> *Schema

	parsedMu sync.Mutex
	parsed   map[string]parsedFile // abs path -> last parse, for read-only Load
}

type parsedFile struct {
	mod  int64
	size int64
	f    *File
}

// parsedMax bounds the Load cache: resolving a material touches its parent's and
// images' GDTs, often the same few, but a stock GDT can be megabytes.
const parsedMax = 64

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
		w.lastStat, w.lastWalk = time.Now(), time.Now()
	})
}

// Freshness: a long-lived MCP server must see GDTs saved in APE meanwhile.
// Re-stat the indexed files (cheap) at most every statEvery, and walk the tree
// for new GDTs in the background at most every walkEvery.
const (
	statEvery = 5 * time.Second
	walkEvery = 2 * time.Minute
)

func (w *Workspace) refresh() {
	w.scan()
	if w.idx == nil {
		return
	}
	w.refreshMu.Lock()
	now := time.Now()
	doStat := now.Sub(w.lastStat) >= statEvery
	doWalk := now.Sub(w.lastWalk) >= walkEvery && !w.walkRunning
	if doStat {
		w.lastStat = now
	}
	if doWalk {
		w.lastWalk, w.walkRunning = now, true
	}
	w.refreshMu.Unlock()
	if doStat {
		w.idx.restat(w.Root)
	}
	if doWalk {
		go func() {
			if fresh, err := loadIndex(w.Root); err == nil {
				w.idx.replace(fresh)
			}
			w.refreshMu.Lock()
			w.walkRunning = false
			w.refreshMu.Unlock()
		}()
	}
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

// Find returns every definition of an asset name across the workspace's GDTs,
// of any type. Names are per type — an image and a material called "clip" are
// both fine (Treyarch's own GDTs have 800+ such pairs); only two definitions of
// the same type are the linker's `Duplicate '<type>' asset` error (see Duplicates).
func (w *Workspace) Find(name string) ([]Location, error) {
	w.refresh()
	if w.scErr != nil {
		return nil, w.scErr
	}
	var out []Location
	for _, h := range w.idx.lookup(name) {
		out = append(out, Location{File: h.File, Line: h.Line, Type: h.Type, Parent: h.Parent, Stock: w.stock[strings.ToLower(h.File)]})
	}
	return out, nil
}

// TypeOf is a definition's asset type, following a derived asset's parent chain.
func (w *Workspace) TypeOf(l Location) string {
	for depth := 0; l.Type == "" && l.Parent != "" && depth < 16; depth++ {
		locs, err := w.Find(l.Parent)
		if err != nil || len(locs) == 0 {
			return ""
		}
		l = locs[0]
	}
	return l.Type
}

// FindTyped returns the definitions of name whose (resolved) type is typ.
func (w *Workspace) FindTyped(name, typ string) ([]Location, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	var out []Location
	for _, l := range locs {
		if lt := w.TypeOf(l); lt == "" || strings.EqualFold(lt, typ) {
			l.Type = lt
			out = append(out, l)
		}
	}
	return out, nil
}

// Duplicates groups a name's definitions by resolved type and returns only the
// types defined more than once — the real `Duplicate '<type>' asset` errors.
func (w *Workspace) Duplicates(name string) (map[string][]Location, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	by := map[string][]Location{}
	for _, l := range locs {
		l.Type = w.TypeOf(l)
		by[l.Type] = append(by[l.Type], l)
	}
	for t, ls := range by {
		if len(ls) < 2 {
			delete(by, t)
		}
	}
	return by, nil
}

// Touched re-indexes one GDT after it was written.
func (w *Workspace) Touched(path string) {
	w.scan()
	if w.idx != nil {
		w.idx.refreshFile(w.Root, w.Rel(path))
	}
}

// Load parses a GDT given relative to the root (or absolute). The result is
// shared and cached until the file changes on disk: callers must not modify it
// (edits parse their own copy).
func (w *Workspace) Load(file string) (*File, error) {
	path := w.Abs(file)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	w.parsedMu.Lock()
	pf, ok := w.parsed[path]
	w.parsedMu.Unlock()
	if ok && pf.mod == info.ModTime().UnixNano() && pf.size == info.Size() {
		return pf.f, nil
	}
	f, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	w.parsedMu.Lock()
	if w.parsed == nil || len(w.parsed) >= parsedMax {
		w.parsed = map[string]parsedFile{}
	}
	w.parsed[path] = parsedFile{info.ModTime().UnixNano(), info.Size(), f}
	w.parsedMu.Unlock()
	return f, nil
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
