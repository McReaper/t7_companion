package gdt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Techset is what a material type's techsetdef exposes: the GDT fields it reads,
// its texture slots with the image semantic each expects, and the HLSL sources
// its techniques compile.
type Techset struct {
	MaterialType string    `json:"material_type"`
	File         string    `json:"file"`
	Category     string    `json:"category"` // must equal the material's materialCategory
	RenderFlags  string    `json:"render_flags,omitempty"`
	Includes     []string  `json:"includes,omitempty"`
	Textures     []TexSlot `json:"textures"`
	Params       []Param   `json:"params,omitempty"`
	Sources      []string  `json:"shader_sources,omitempty"`
}

// TexSlot is a Texture( "…" ) block.
type TexSlot struct {
	Name         string `json:"name"`
	Field        string `json:"gdt_field,omitempty"`     // the material field it reads, e.g. colorMap; none for a fixed image
	DefaultImage string `json:"default_image,omitempty"` // the image an empty field gets: a $ built-in or a stock image
	Semantic     string `json:"semantic,omitempty"`      // the image's semantic must match (diffuseMap, normalMap, …)
	Usage        string `json:"usage,omitempty"`
}

// Param is any other declaration reading material fields (Color, float2, Bool, …).
type Param struct {
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Fields []string `json:"gdt_fields"`
}

// ExposedFields is every material GDT field the techset reads.
func (t *Techset) ExposedFields() map[string]bool {
	out := map[string]bool{}
	for _, s := range t.Textures {
		if s.Field != "" { // a fixed image reads no field
			out[s.Field] = true
		}
	}
	for _, p := range t.Params {
		for _, f := range p.Fields {
			out[f] = true
		}
	}
	return out
}

// Techsets indexes the techsetdef tree (share/raw/techsetdefs_stable). A
// long-lived MCP server must see the techsetdefs written meanwhile: a material
// type it doesn't know rescans the tree (at most every rescanEvery), the tree
// is rescanned anyway once it is walkEvery old (a techsetdef deleted), and a
// resolved techset is read again when a file it read has changed (checked at
// most every restatEvery).
type Techsets struct {
	root     string
	tree     atomic.Pointer[techsetTree]
	resolved sync.Map // material type -> *resolvedTechset

	scanMu                   sync.Mutex
	lastScan                 time.Time
	rescanEvery, restatEvery time.Duration
}

// techsetTree maps a techsetdef's basename, without the extension, to its
// path. A rescan replaces it whole: readers never see one half-built.
type techsetTree map[string]string

// rescanEvery bounds how often lookups of unknown material types rescan the
// tree: a GDT full of misspelt types mustn't walk it once per asset.
const rescanEvery = 2 * time.Second

// resolvedTechset is a resolved techset and the files it was read from.
type resolvedTechset struct {
	ts      *Techset // shared: callers must not modify it
	files   []fileStamp
	checked atomic.Int64 // when files were last found unchanged, in UnixNano
}

// fileStamp is a file as it was read.
type fileStamp struct {
	path string
	mod  time.Time
	size int64
}

// OpenTechsets indexes every *.techsetdef under root.
func OpenTechsets(root string) (*Techsets, error) {
	t := &Techsets{root: root, rescanEvery: rescanEvery, restatEvery: statEvery}
	tree, err := scanTechsets(root)
	if err != nil {
		return nil, err
	}
	t.tree.Store(&tree)
	t.lastScan = time.Now()
	return t, nil
}

func scanTechsets(root string) (techsetTree, error) {
	tree := techsetTree{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".techsetdef") {
			name := strings.TrimSuffix(d.Name(), ".techsetdef")
			// prefer non-include definitions for a material type name
			if _, seen := tree[name]; !seen || filepath.Base(filepath.Dir(p)) != "include" {
				tree[name] = p
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index techsetdefs: %w", err)
	}
	return tree, nil
}

// current is the tree, rescanned first when it is walkEvery old.
func (t *Techsets) current() techsetTree {
	t.scanMu.Lock()
	stale := time.Since(t.lastScan) >= walkEvery
	t.scanMu.Unlock()
	if stale {
		t.rescan(true)
	}
	return *t.tree.Load()
}

// rescan walks the tree again, unless it was walked less than rescanEvery ago
// and the caller doesn't insist. A failed walk keeps the tree it had.
func (t *Techsets) rescan(force bool) {
	t.scanMu.Lock()
	defer t.scanMu.Unlock()
	if !force && time.Since(t.lastScan) < t.rescanEvery {
		return
	}
	if tree, err := scanTechsets(t.root); err == nil {
		t.tree.Store(&tree)
	}
	t.lastScan = time.Now()
}

// Exists reports whether a material type has a techsetdef, rescanning the tree
// for one it doesn't know.
func (t *Techsets) Exists(materialType string) bool {
	if t.has(materialType) {
		return true
	}
	t.rescan(false)
	return t.has(materialType)
}

func (t *Techsets) has(materialType string) bool {
	p, ok := t.current()[materialType]
	return ok && filepath.Base(filepath.Dir(p)) != "include"
}

// Names lists the material types (techsetdefs outside include/), sorted.
func (t *Techsets) Names() []string {
	var out []string
	for n, p := range t.current() {
		if filepath.Base(filepath.Dir(p)) != "include" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

var (
	includeRE = regexp.MustCompile(`(?m)^\s*#include\s+"([^"]+)"`)
	declRE    = regexp.MustCompile(`(?m)^\s*(Texture|Sampler|Color|Bool|Float|Int|float[1-4]?|int[1-4]?|bool)\s*\(\s*"([^"]+)"\s*\)`)
	globalsRE = regexp.MustCompile(`(?s)Globals\s*\(\s*\)\s*\{(.*?)\}`)
	kvRE      = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*"([^"]*)"`)
	fieldRE   = regexp.MustCompile(`<\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:,\s*(\$?[A-Za-z0-9_]+))?\s*>`)
	sourceRE  = regexp.MustCompile(`source\s*=\s*"([^"]+)"`)
)

// Resolve loads a material type's techsetdef and everything it #includes. The
// result is cached and shared: copy it before changing it. A cached techset
// whose files changed is read again.
func (t *Techsets) Resolve(materialType string) (*Techset, error) {
	if v, ok := t.resolved.Load(materialType); ok {
		r := v.(*resolvedTechset)
		if t.unchanged(r) {
			return r.ts, nil
		}
		t.resolved.Delete(materialType)
		t.rescan(true) // a file gone or renamed may have moved the material type
	}
	r, err := t.resolve(materialType)
	if err != nil {
		return nil, err
	}
	t.resolved.Store(materialType, r)
	return r.ts, nil
}

// unchanged reports whether the files a techset was read from are as they
// were, statting them at most every restatEvery.
func (t *Techsets) unchanged(r *resolvedTechset) bool {
	now := time.Now()
	if now.Sub(time.Unix(0, r.checked.Load())) < t.restatEvery {
		return true
	}
	for _, f := range r.files {
		fi, err := os.Stat(f.path)
		if err != nil || !fi.ModTime().Equal(f.mod) || fi.Size() != f.size {
			return false
		}
	}
	r.checked.Store(now.UnixNano())
	return true
}

func (t *Techsets) resolve(materialType string) (*resolvedTechset, error) {
	if !t.Exists(materialType) {
		return nil, fmt.Errorf("no techsetdef for material type %q", materialType)
	}
	ts := &Techset{MaterialType: materialType, File: t.current()[materialType]}
	wk := &techsetWalk{t: t, ts: ts, seenFile: map[string]bool{}, seenDecl: map[string]bool{}, seenSrc: map[string]bool{}}
	if err := wk.walk(ts.File, true); err != nil {
		return nil, err
	}
	sort.Strings(ts.Sources)
	r := &resolvedTechset{ts: ts, files: wk.files}
	r.checked.Store(time.Now().UnixNano())
	return r, nil
}

// techsetWalk follows a techsetdef and its #includes into one Techset. A
// declaration seen first wins (the including file overrides its includes).
type techsetWalk struct {
	t                           *Techsets
	ts                          *Techset
	seenFile, seenDecl, seenSrc map[string]bool
	files                       []fileStamp // what was read, as it was
}

func (wk *techsetWalk) walk(path string, top bool) error {
	if wk.seenFile[path] {
		return nil
	}
	wk.seenFile[path] = true
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	wk.files = append(wk.files, fileStamp{path, fi.ModTime(), fi.Size()})
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := stripComments(string(b))
	if top {
		readGlobals(wk.ts, src)
	}
	wk.addDecls(src)
	for _, s := range sourceRE.FindAllStringSubmatch(src, -1) {
		if !wk.seenSrc[s[1]] {
			wk.seenSrc[s[1]] = true
			wk.ts.Sources = append(wk.ts.Sources, s[1])
		}
	}
	for _, inc := range includeRE.FindAllStringSubmatch(src, -1) {
		p, ok := wk.t.includePath(path, inc[1])
		if !ok {
			continue
		}
		if !top || p != path {
			wk.ts.Includes = append(wk.ts.Includes, inc[1])
		}
		if err := wk.walk(p, false); err != nil {
			return err
		}
	}
	return nil
}

// readGlobals takes the category and render flags from the Globals() block.
func readGlobals(ts *Techset, src string) {
	g := globalsRE.FindStringSubmatch(src)
	if g == nil {
		return
	}
	for _, kv := range kvRE.FindAllStringSubmatch(g[1], -1) {
		switch kv[1] {
		case "category":
			ts.Category = kv[2]
		case "renderFlags":
			ts.RenderFlags = kv[2]
		}
	}
}

// addDecls adds the texture slots and parameters declared in src.
func (wk *techsetWalk) addDecls(src string) {
	for _, m := range declRE.FindAllStringSubmatchIndex(src, -1) {
		kind, name := src[m[2]:m[3]], src[m[4]:m[5]]
		if prop, line, ok := property(src[m[1]:]); ok {
			wk.addProperty(kind, name, prop, line)
			continue
		}
		body := blockAfter(src, m[1])
		if wk.seenDecl[kind+":"+name] {
			continue
		}
		wk.seenDecl[kind+":"+name] = true
		refs := fieldRE.FindAllStringSubmatch(body, -1)
		if kind == "Texture" {
			if slot := texSlot(name, refs, body); slot.Field != "" || slot.DefaultImage != "" {
				wk.ts.Textures = append(wk.ts.Textures, slot)
			}
			continue
		}
		wk.addParam(kind, name, refs)
	}
}

// property reads a one-line property assignment after a declaration's name,
// `Texture( "x" ).image = Image( <field, default> )`: the property and the
// rest of the line.
func property(after string) (prop, line string, ok bool) {
	rest, ok := strings.CutPrefix(strings.TrimLeft(after, " \t"), ".")
	if !ok {
		return "", "", false
	}
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[:i]
	}
	prop, line, _ = strings.Cut(rest, "=")
	return strings.TrimSpace(prop), line, true
}

// addProperty applies `Decl( "name" ).prop = …`. An .image declares a texture
// slot (unless a file walked earlier — the material type's own — did), a
// .semantic sets one's; .tweak only retitles a declaration an #include makes;
// any other property reads the fields it names.
func (wk *techsetWalk) addProperty(kind, name, prop, line string) {
	refs := fieldRE.FindAllStringSubmatch(line, -1)
	switch {
	case prop == "tweak":
	case kind == "Texture" && prop == "image":
		if !wk.seenDecl[kind+":"+name] && len(refs) > 0 {
			wk.seenDecl[kind+":"+name] = true
			wk.ts.Textures = append(wk.ts.Textures, TexSlot{Name: name, Field: refs[0][1], DefaultImage: refs[0][2]})
		}
	case kind == "Texture" && prop == "semantic":
		for i := range wk.ts.Textures {
			if s := &wk.ts.Textures[i]; s.Name == name && s.Semantic == "" {
				s.Semantic = strings.Trim(strings.TrimSpace(line), `"`)
			}
		}
	default:
		wk.addParam(kind, name+"."+prop, refs)
	}
}

// addParam adds a declaration that reads material fields.
func (wk *techsetWalk) addParam(kind, name string, refs [][]string) {
	p := Param{Kind: kind, Name: name}
	for _, r := range refs {
		p.Fields = append(p.Fields, r[1])
	}
	if len(p.Fields) > 0 {
		wk.ts.Params = append(wk.ts.Params, p)
	}
}

// fixedImageRE is a slot's image that no field sets: Image( rain_hit_n ),
// Image( "$default" ).
var fixedImageRE = regexp.MustCompile(`Image\(\s*"?(\$?[A-Za-z0-9_]+)"?\s*\)`)

// texSlot reads a Texture( "name" ) block: the first <field, default> it reads
// — or, for a slot no field sets, its fixed image — and its semantic and usage.
func texSlot(name string, refs [][]string, body string) TexSlot {
	slot := TexSlot{Name: name}
	if len(refs) > 0 {
		slot.Field, slot.DefaultImage = refs[0][1], refs[0][2]
	} else if m := fixedImageRE.FindStringSubmatch(body); m != nil {
		slot.DefaultImage = m[1]
	}
	for _, kv := range kvRE.FindAllStringSubmatch(body, -1) {
		switch kv[1] {
		case "semantic":
			slot.Semantic = kv[2]
		case "usage":
			slot.Usage = kv[2]
		}
	}
	return slot
}

// includePath resolves an #include: include/ first, then the including dir, then any match.
func (t *Techsets) includePath(from, name string) (string, bool) {
	name = strings.TrimSuffix(name, ".techsetdef")
	for _, dir := range []string{filepath.Join(t.root, "include"), filepath.Dir(from)} {
		for _, ext := range []string{".techsetdef", ".inc.techsetdef"} {
			p := filepath.Join(dir, name+ext)
			if _, err := os.Stat(p); err == nil {
				return p, true
			}
		}
	}
	p, ok := t.current()[name]
	return p, ok
}

// blockAfter returns the { … } body following position i (empty if none).
func blockAfter(src string, i int) string {
	open := strings.IndexByte(src[i:], '{')
	if open < 0 {
		return ""
	}
	// a declaration may inherit ": "parent"" before its body; stop at a newline-separated next decl
	if nl := strings.Index(src[i:i+open], "\n\n"); nl >= 0 {
		return ""
	}
	start := i + open
	depth := 0
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start+1 : j]
			}
		}
	}
	return ""
}
