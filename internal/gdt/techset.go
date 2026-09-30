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
	Field        string `json:"gdt_field"` // the material field it reads, e.g. colorMap
	DefaultImage string `json:"default_image,omitempty"`
	Semantic     string `json:"semantic,omitempty"` // the image's semantic must match (diffuseMap, normalMap, …)
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
		out[s.Field] = true
	}
	for _, p := range t.Params {
		for _, f := range p.Fields {
			out[f] = true
		}
	}
	return out
}

// Techsets indexes the techsetdef tree (share/raw/techsetdefs_stable).
type Techsets struct {
	root     string
	byName   map[string]string // basename without extension -> path
	resolved sync.Map          // material type -> *Techset (shared: callers must not modify it)
}

// OpenTechsets indexes every *.techsetdef under root.
func OpenTechsets(root string) (*Techsets, error) {
	t := &Techsets{root: root, byName: map[string]string{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".techsetdef") {
			name := strings.TrimSuffix(d.Name(), ".techsetdef")
			// prefer non-include definitions for a material type name
			if _, seen := t.byName[name]; !seen || filepath.Base(filepath.Dir(p)) != "include" {
				t.byName[name] = p
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("index techsetdefs: %w", err)
	}
	return t, nil
}

// Exists reports whether a material type has a techsetdef.
func (t *Techsets) Exists(materialType string) bool {
	p, ok := t.byName[materialType]
	return ok && filepath.Base(filepath.Dir(p)) != "include"
}

// Names lists the material types (techsetdefs outside include/), sorted.
func (t *Techsets) Names() []string {
	var out []string
	for n, p := range t.byName {
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
	fieldRE   = regexp.MustCompile(`<\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:,\s*\$?([A-Za-z0-9_]+))?\s*>`)
	sourceRE  = regexp.MustCompile(`source\s*=\s*"([^"]+)"`)
)

// Resolve loads a material type's techsetdef and everything it #includes. The
// result is cached and shared: copy it before changing it.
func (t *Techsets) Resolve(materialType string) (*Techset, error) {
	if v, ok := t.resolved.Load(materialType); ok {
		return v.(*Techset), nil
	}
	ts, err := t.resolve(materialType)
	if err != nil {
		return nil, err
	}
	t.resolved.Store(materialType, ts)
	return ts, nil
}

func (t *Techsets) resolve(materialType string) (*Techset, error) {
	if !t.Exists(materialType) {
		return nil, fmt.Errorf("no techsetdef for material type %q", materialType)
	}
	ts := &Techset{MaterialType: materialType, File: t.byName[materialType]}
	seenFile := map[string]bool{}
	seenDecl := map[string]bool{}
	seenSrc := map[string]bool{}
	var walk func(path string, top bool) error
	walk = func(path string, top bool) error {
		if seenFile[path] {
			return nil
		}
		seenFile[path] = true
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := stripComments(string(b))
		if top {
			if g := globalsRE.FindStringSubmatch(src); g != nil {
				for _, kv := range kvRE.FindAllStringSubmatch(g[1], -1) {
					switch kv[1] {
					case "category":
						ts.Category = kv[2]
					case "renderFlags":
						ts.RenderFlags = kv[2]
					}
				}
			}
		}
		for _, m := range declRE.FindAllStringSubmatchIndex(src, -1) {
			kind, name := src[m[2]:m[3]], src[m[4]:m[5]]
			body := blockAfter(src, m[1])
			if seenDecl[kind+":"+name] {
				continue
			}
			seenDecl[kind+":"+name] = true
			refs := fieldRE.FindAllStringSubmatch(body, -1)
			if kind == "Texture" {
				slot := TexSlot{Name: name}
				for _, r := range refs {
					if slot.Field == "" {
						slot.Field, slot.DefaultImage = r[1], r[2]
					}
				}
				for _, kv := range kvRE.FindAllStringSubmatch(body, -1) {
					switch kv[1] {
					case "semantic":
						slot.Semantic = kv[2]
					case "usage":
						slot.Usage = kv[2]
					}
				}
				if slot.Field != "" {
					ts.Textures = append(ts.Textures, slot)
				}
				continue
			}
			p := Param{Kind: kind, Name: name}
			for _, r := range refs {
				p.Fields = append(p.Fields, r[1])
			}
			if len(p.Fields) > 0 {
				ts.Params = append(ts.Params, p)
			}
		}
		for _, s := range sourceRE.FindAllStringSubmatch(src, -1) {
			if !seenSrc[s[1]] {
				seenSrc[s[1]] = true
				ts.Sources = append(ts.Sources, s[1])
			}
		}
		for _, inc := range includeRE.FindAllStringSubmatch(src, -1) {
			p, ok := t.includePath(path, inc[1])
			if !ok {
				continue
			}
			if !top || p != path {
				ts.Includes = append(ts.Includes, inc[1])
			}
			if err := walk(p, false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(ts.File, true); err != nil {
		return nil, err
	}
	sort.Strings(ts.Sources)
	return ts, nil
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
	p, ok := t.byName[name]
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
