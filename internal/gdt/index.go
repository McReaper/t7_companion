package gdt

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	writeMu sync.Map // lower-cased abs path -> *sync.Mutex: one writer per GDT

	models modelMats // xmodel LOD files and their materials, for ModelUses

	gdtTypesOnce sync.Once
	gdtTypes     map[string]bool // linker types a GDT type packs as (IsGDTType)
}

// lockFile serialises writes to one GDT within this process.
func (w *Workspace) lockFile(path string) func() {
	m, _ := w.writeMu.LoadOrStore(strings.ToLower(filepath.Clean(path)), &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// InGDTDirs reports whether path is under one of the directories gdtdb indexes
// (bin/converter_gdt_dirs_0.txt) — a GDT anywhere else is never built.
func (w *Workspace) InGDTDirs(path string) bool {
	rel := strings.ToLower(w.Rel(path))
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return false
	}
	for _, d := range GDTDirs(w.Root) {
		d = strings.ToLower(strings.Trim(filepath.ToSlash(d), "/"))
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
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
	sort.Slice(out, func(i, j int) bool { // the index is a map: make "the first definition" stable
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// TypeOf is a definition's asset type, following a derived asset's parent chain
// within its own GDT (where a parent has to be). When the GDT defines the
// parent's name more than once (a material and an xmodel), the first
// definition is the parent, as for gdtdb and Resolved.
func (w *Workspace) TypeOf(l Location) string {
	for depth := 0; l.Type == "" && l.Parent != "" && depth < 16; depth++ {
		locs, err := w.Find(l.Parent)
		if err != nil {
			return ""
		}
		next := l
		for _, p := range locs { // sorted by file, then line
			if p.File == l.File {
				next = p
				break
			}
		}
		if next == l {
			return ""
		}
		l = next
	}
	return l.Type
}

// FindTyped returns the definitions of name whose (resolved) type is typ. A
// derived asset whose parent chain is broken has no type and matches none.
func (w *Workspace) FindTyped(name, typ string) ([]Location, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	var out []Location
	for _, l := range locs {
		if lt := w.TypeOf(l); lt != "" && strings.EqualFold(lt, typ) {
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
		if t == "" || len(ls) < 2 { // unresolved derived assets: no type to collide on
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
// chain ("child" [ "parent" ]) inside f; own fields win. A derived asset's parent
// must be in the same GDT — gdtdb rejects it otherwise (`GDT ParseError: … Parent
// Entity '<name>' does not exist in GDT`), and all 16,972 derived assets of a
// stock install follow that. A parent missing from f leaves the type unknown.
func (w *Workspace) Resolved(f *File, a *Asset) (string, []Field, error) {
	chain := []*Asset{a}
	seen := map[string]bool{a.Name: true}
	for cur := a; cur.Parent != ""; {
		p := f.Find(cur.Parent)
		if p == nil {
			break
		}
		if seen[p.Name] {
			return "", nil, fmt.Errorf("derivation cycle at %q", p.Name)
		}
		seen[p.Name] = true
		chain = append(chain, p)
		cur = p
	}
	typ := ""
	merged := map[string]string{}
	var order []string
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].Type != "" {
			typ = chain[i].Type
		}
		for _, fl := range chain[i].Fields {
			if _, ok := merged[fl.Key]; !ok {
				order = append(order, fl.Key)
			}
			merged[fl.Key] = fl.Value
		}
	}
	if root := chain[len(chain)-1]; root.Parent != "" {
		typ = "" // chain broken: the parent isn't in this GDT
	}
	out := make([]Field, 0, len(order))
	for _, k := range order {
		out = append(out, Field{Key: k, Value: merged[k]})
	}
	return typ, out, nil
}
