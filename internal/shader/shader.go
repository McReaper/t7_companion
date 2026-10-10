// Package shader finds Black Ops III's compiled shaders in the mod tools'
// cache, groups a source's permutations by the program they compiled to, and
// decompiles one to HLSL with t7_dxbc.
//
// The cache is share/assetconvert/shaders/pc/v7, a file per compiled shader
// named <source>_<stage>_main_<hash> (techsetdef_unlit.hlsl_ps_main_CJK6…):
// the HLSL source with its directory's slashes made underscores, the stage
// (ps, vs, gs, cs) and a hash of what was compiled. The hash covers the
// source's content and its defines (a vertex and a pixel shader can share one,
// and editing a source changes it), and the stock sources aren't shipped, so
// it can't be recomputed: the permutation a techset uses is found by grouping
// a source's files by program instead, which permutations differing only by
// defines the code ignores share. The linker adds a file here for every
// custom source it compiles.
package shader

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/McReaper/t7_dxbc/dxbc"
	"github.com/McReaper/t7_dxbc/hlsl"
)

// Dir is the shader cache under a mod-tools root.
func Dir(root string) string {
	return filepath.Join(root, "share", "assetconvert", "shaders", "pc", "v7")
}

// Name is a cache file's name, split.
type Name struct {
	Source, Stage, Hash string
}

// stages are the stages a cache file's name gives.
var stages = map[string]bool{"ps": true, "vs": true, "gs": true, "cs": true, "hs": true, "ds": true}

// ParseName splits a cache file's name; ok is false for any other name.
func ParseName(s string) (n Name, ok bool) {
	head, hash := cut(s, "_")
	if hash == "" {
		return Name{}, false
	}
	head, ok = strings.CutSuffix(head, "_main")
	if !ok {
		return Name{}, false
	}
	source, stage := cut(head, "_")
	if source == "" || !stages[stage] {
		return Name{}, false
	}
	return Name{source, stage, hash}, true
}

// cut splits s around the last sep; after is "" without one.
func cut(s, sep string) (before, after string) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+len(sep):]
}

// SourceName is how the cache names a source a techset gives with its
// directory (specialty/thermal_scope.hlsl is specialty_thermal_scope.hlsl).
func SourceName(source string) string {
	return strings.ToLower(strings.NewReplacer("/", "_", `\`, "_").Replace(source))
}

// Files lists the cache's shader files, by source.
func Files(dir string) (map[string][]Name, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("the shader cache: %w", err)
	}
	out := map[string][]Name{}
	for _, e := range entries {
		if n, ok := ParseName(e.Name()); ok && !e.IsDir() {
			out[strings.ToLower(n.Source)] = append(out[strings.ToLower(n.Source)], n)
		}
	}
	return out, nil
}

// File is a cache file's name.
func (n Name) File() string { return n.Source + "_" + n.Stage + "_main_" + n.Hash }

// Variant is a source's permutations for one stage that compiled to one
// program. What the program reads tells variants apart.
type Variant struct {
	Stage        string   `json:"stage"`
	Files        []string `json:"files"` // sorted
	Instructions int      `json:"instructions"`
	Globals      []string `json:"globals,omitempty"`   // the $Globals variables it reads
	Resources    []string `json:"resources,omitempty"` // the buffers, textures and samplers it binds: name@register
	Inputs       []string `json:"inputs,omitempty"`    // its input signature's semantics
}

// Variants groups a source's cache files by program, for one stage or, with
// stage "", every stage: largest groups first.
func Variants(dir string, files []Name, stage string) ([]Variant, error) {
	byKey := map[[32]byte]*Variant{}
	var order [][32]byte
	for _, n := range files {
		if stage != "" && n.Stage != stage {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n.File()))
		if err != nil {
			return nil, err
		}
		key, v, err := summarize(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n.File(), err)
		}
		if byKey[key] == nil {
			v.Stage = n.Stage
			byKey[key] = v
			order = append(order, key)
		}
		byKey[key].Files = append(byKey[key].Files, n.File())
	}
	out := make([]Variant, 0, len(order))
	for _, k := range order {
		sort.Strings(byKey[k].Files)
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Stage != out[j].Stage {
			return out[i].Stage < out[j].Stage
		}
		return len(out[i].Files) > len(out[j].Files)
	})
	return out, nil
}

// programChunks are what a shader's program is: its code, its signatures and
// its reflection, which the decompilation follows. The container's other
// chunks (statistics) don't change it.
var programChunks = [][]string{{"SHEX", "SHDR"}, {"ISGN", "ISG1"}, {"OSGN", "OSG1", "OSG5"}, {"PCSG", "PSG1"}, {"RDEF"}}

// summarize reads a compiled shader: the key of its program, and what it
// reads.
func summarize(b []byte) ([32]byte, *Variant, error) {
	c, err := dxbc.Parse(b)
	if err != nil {
		return [32]byte{}, nil, err
	}
	h := sha256.New()
	for _, names := range programChunks {
		chunk := c.Chunk(names...)
		fmt.Fprintf(h, "%d:", len(chunk))
		h.Write(chunk)
	}
	var key [32]byte
	copy(key[:], h.Sum(nil))
	v := &Variant{}
	if p, err := dxbc.DecodeProgram(c.Chunk("SHEX", "SHDR")); err == nil {
		v.Instructions = len(p.Instructions)
	}
	if rdef := c.Chunk("RDEF"); rdef != nil {
		if ref, err := dxbc.ReadReflection(rdef); err == nil {
			v.Globals, v.Resources = reads(ref)
		}
	}
	if in, _, err := c.Signatures(); err == nil {
		for _, el := range in {
			v.Inputs = append(v.Inputs, fmt.Sprintf("%s%d", el.Name, el.Index))
		}
	}
	return key, v, nil
}

// reads lists the $Globals variables a shader reads and what it binds.
func reads(ref *dxbc.Reflection) (globals, resources []string) {
	for _, b := range ref.Buffers {
		if b.Name == "$Globals" {
			for _, v := range b.Vars {
				if v.Flags&dxbc.VariableUsed != 0 {
					globals = append(globals, v.Name)
				}
			}
		}
	}
	for _, b := range ref.Bindings {
		if b.Name != "$Globals" {
			resources = append(resources, b.Name+"@"+b.Register())
		}
	}
	return globals, resources
}

// ErrIncomplete is wrapped by Decompile's error when the shader decompiled in
// part: the HLSL holds what was, and the error names what wasn't.
var ErrIncomplete = errors.New("decompiled in part")

// Decompile decompiles a cache file.
func Decompile(dir, file string) (string, error) {
	if _, ok := ParseName(file); !ok || filepath.Base(file) != file {
		return "", fmt.Errorf("%q is not a shader cache file's name (<source>_<stage>_main_<hash>)", file)
	}
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", err
	}
	src, err := hlsl.Decompile(b)
	if err != nil && src != "" {
		return src, fmt.Errorf("%w: %w", ErrIncomplete, err)
	}
	return src, err
}
