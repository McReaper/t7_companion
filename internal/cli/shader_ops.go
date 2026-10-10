package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/shader"
)

// shaderOut is a shader_decompile answer: one compiled shader's HLSL, a page
// at a time, or, when the name gives several programs, what tells them apart.
type shaderOut struct {
	Shader     string          `json:"shader,omitempty"` // the cache file decompiled
	Source     string          `json:"source,omitempty"`
	Alike      int             `json:"alike,omitempty"`      // other permutations that compiled to the same program
	Incomplete string          `json:"incomplete,omitempty"` // what isn't decompiled: the HLSL holds the rest
	HLSL       string          `json:"hlsl,omitempty"`
	Length     int             `json:"length,omitempty"`          // the whole HLSL's characters
	NextOffset int             `json:"next_offset,omitempty"`     // where the next page starts
	Sources    []string        `json:"sources,omitempty"`         // a material type's in-game sources, when several
	Params     []string        `json:"material_params,omitempty"` // the constants a material type's techset gives its shaders
	Variants   []shaderVariant `json:"variants,omitempty"`
	Note       string          `json:"note,omitempty"`
}

// shaderVariant is one program a source compiled to, and what it reads.
type shaderVariant struct {
	Stage        string   `json:"stage"`
	Shader       string   `json:"shader"` // one of its files: pass it as name
	Permutations int      `json:"permutations"`
	Instructions int      `json:"instructions"`
	Globals      []string `json:"globals,omitempty"`
	Resources    []string `json:"resources,omitempty"`
	Inputs       []string `json:"inputs,omitempty"`
}

// shaderStages are the stages a request may narrow to.
var shaderStages = map[string]bool{"": true, "ps": true, "vs": true, "gs": true, "cs": true}

// shaderDecompile decompiles a shader of the mod tools' cache, named by its
// file, its source or a material type. all returns the whole HLSL instead of
// a page.
func shaderDecompile(toolsPath, name, stage string, offset, maxChars int, all bool) (*shaderOut, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	stage = strings.ToLower(stage)
	if !shaderStages[stage] {
		return nil, fmt.Errorf("stage %q: ps, vs, gs or cs", stage)
	}
	dir := shader.Dir(root)
	files, err := shader.Files(dir)
	if err != nil {
		return nil, err
	}
	pg := page{offset, maxChars, all}
	if n, ok := shader.ParseName(name); ok && hasFile(files[strings.ToLower(n.Source)], n) {
		return decompileShader(dir, files[strings.ToLower(n.Source)], n, pg)
	}
	if source := sourceFor(name, files); source != "" {
		return shaderOfSource(dir, source, files[source], stage, pg)
	}
	sources, params, err := materialSources(toolsPath, name, files)
	if err != nil {
		return nil, err
	}
	var out *shaderOut
	switch len(sources) {
	case 0:
		return nil, fmt.Errorf("no compiled shader, shader source or material type named %q in %s%s", name, dir, similarSources(name, files))
	case 1:
		if out, err = shaderOfSource(dir, sources[0], files[sources[0]], stage, pg); err != nil {
			return nil, err
		}
	default:
		out = &shaderOut{Sources: sources, Note: fmt.Sprintf("material type %s draws with %d shader sources in game: pass one as name", name, len(sources))}
	}
	out.Params = params
	return out, nil
}

func hasFile(names []shader.Name, n shader.Name) bool {
	for _, m := range names {
		if strings.EqualFold(m.File(), n.File()) {
			return true
		}
	}
	return false
}

// sourceFor is the cache's name for a shader source, "" when it has none.
func sourceFor(name string, files map[string][]shader.Name) string {
	s := shader.SourceName(name)
	for _, c := range []string{s, s + ".hlsl"} {
		if len(files[c]) > 0 {
			return c
		}
	}
	return ""
}

// materialSources are the cache's names for the shader sources a material
// type's techset draws with in game (not those the tools preview with), and
// the constants it gives them, if the name is a material type.
func materialSources(toolsPath, name string, files map[string][]shader.Name) (sources, params []string, err error) {
	w, err := workspace(toolsPath)
	if err != nil || !w.Techsets.Exists(name) {
		return nil, nil, nil // not a material type (or no install to read techsets from): an unknown name
	}
	ts, err := w.Techsets.Resolve(name)
	if err != nil {
		return nil, nil, err
	}
	for _, s := range ts.Sources {
		if strings.HasPrefix(strings.ToLower(s), "toolsgfx/") {
			continue
		}
		if c := sourceFor(s, files); c != "" {
			sources = append(sources, c)
		}
	}
	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("material type %s's in-game shader sources (%s) have no compiled shader in the cache", name, strings.Join(ts.Sources, ", "))
	}
	for _, p := range ts.Params {
		params = append(params, p.Name)
	}
	return sources, params, nil
}

// similarSources suggests the cache's sources whose name contains the query.
func similarSources(name string, files map[string][]shader.Name) string {
	q := strings.ToLower(strings.TrimSuffix(name, ".hlsl"))
	var out []string
	for s := range files {
		if q != "" && strings.Contains(s, q) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	if len(out) > 10 {
		out = append(out[:10], "…")
	}
	return "; sources named like it: " + strings.Join(out, ", ")
}

// shaderOfSource decompiles a source's program for a stage when there is one,
// and lists them when there are several.
func shaderOfSource(dir, source string, names []shader.Name, stage string, pg page) (*shaderOut, error) {
	vs, err := shader.Variants(dir, names, stage)
	if err != nil {
		return nil, err
	}
	switch len(vs) {
	case 0:
		return nil, fmt.Errorf("no %s shader of %s in the cache", stage, source)
	case 1:
		n, _ := shader.ParseName(vs[0].Files[0])
		out, err := decompileFile(dir, n, pg)
		if out != nil {
			out.Alike = len(vs[0].Files) - 1
		}
		return out, err
	}
	out := &shaderOut{Source: source}
	for _, v := range vs {
		out.Variants = append(out.Variants, shaderVariant{v.Stage, v.Files[0], len(v.Files), v.Instructions, v.Globals, v.Resources, v.Inputs})
	}
	out.Note = fmt.Sprintf("%s compiled to %d programs: decompile one by passing its shader as name", source, len(vs))
	if stage == "" {
		out.Note += ", or narrow them with stage"
	}
	return out, nil
}

// decompileShader decompiles one cache file, counting the permutations alike
// on its first page.
func decompileShader(dir string, names []shader.Name, n shader.Name, pg page) (*shaderOut, error) {
	out, err := decompileFile(dir, n, pg)
	if err != nil || pg.offset > 0 {
		return out, err
	}
	vs, err := shader.Variants(dir, names, n.Stage)
	if err != nil {
		return nil, err
	}
	for _, v := range vs {
		for _, f := range v.Files {
			if strings.EqualFold(f, n.File()) {
				out.Alike = len(v.Files) - 1
			}
		}
	}
	return out, nil
}

// page is which part of a long HLSL to return.
type page struct {
	offset, maxChars int
	all              bool
}

func decompileFile(dir string, n shader.Name, pg page) (*shaderOut, error) {
	src, err := shader.Decompile(dir, n.File())
	out := &shaderOut{Shader: n.File(), Source: n.Source, Length: len(src)}
	switch {
	case errors.Is(err, shader.ErrIncomplete):
		out.Incomplete = strings.TrimPrefix(err.Error(), shader.ErrIncomplete.Error()+": ")
	case err != nil:
		return nil, err
	}
	if pg.offset < 0 || pg.offset > 0 && pg.offset >= len(src) {
		return nil, fmt.Errorf("offset %d is outside the HLSL (%d characters)", pg.offset, len(src))
	}
	end := len(src)
	if size := pageSize(pg.maxChars); !pg.all && pg.offset+size < len(src) {
		end = pageEnd(src, pg.offset, pg.offset+size)
		out.NextOffset = end
	}
	out.HLSL = src[pg.offset:end]
	return out, nil
}
