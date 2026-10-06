package deps

import (
	"bufio"
	"io"
	"strings"
)

// mapEntity is an entity of a Radiant map source: its keys (lower-cased), and
// the materials its brushes and patches draw.
type mapEntity struct {
	keys      map[string]string
	materials []string
}

// readMap parses a Radiant map source (iwmap 4): entities are brace blocks of
// `"key" "value"` lines, holding brush blocks — one `( p ) ( p ) ( p )
// material …` line per face, or a patch (`curve` or `mesh`, then a block whose
// first bare line, after toolFlags, is the material). The lightmap material
// that follows each one is not an asset.
//
// The header declares the layers (`"name" flags …`); an entity or a brush in a
// layer flagged ignore, or under one, is not compiled and is left out.
func readMap(r io.Reader) ([]mapEntity, error) {
	p := mapParser{ignored: map[string]bool{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		p.line(strings.TrimSpace(sc.Text()))
	}
	return p.out, sc.Err()
}

type mapParser struct {
	out     []mapEntity
	ignored map[string]bool // layers flagged ignore
	depth   int
	patch   bool
	entity  mapEntity
	layer   string   // the entity's
	brush   []string // the current brush's materials
	brushIn string   // the current brush's layer
}

func (p *mapParser) line(l string) {
	switch {
	case l == "{":
		p.open()
	case l == "}":
		p.close()
	case p.depth == 0:
		if name, flags, ok := layerDecl(l); ok && strings.Contains(" "+flags+" ", " ignore ") {
			p.ignored[name] = true
		}
	case strings.HasPrefix(l, "layer "):
		if p.depth == 1 {
			p.layer = strings.Trim(l[len("layer "):], `" `)
		} else {
			p.brushIn = strings.Trim(l[len("layer "):], `" `)
		}
	case p.depth == 1 && strings.HasPrefix(l, `"`):
		if k, v, ok := keyValue(l); ok {
			p.entity.keys[strings.ToLower(k)] = v
		}
	case l == "curve" || l == "mesh":
		p.patch = true
	case p.patch && isWord(l):
		p.brush = append(p.brush, l)
		p.patch = false
	case strings.HasPrefix(l, "("):
		if m := faceMaterial(l); m != "" {
			p.brush = append(p.brush, m)
		}
	}
}

func (p *mapParser) open() {
	p.depth++
	switch p.depth {
	case 1:
		p.entity, p.layer = mapEntity{keys: map[string]string{}}, ""
	case 2:
		p.brush, p.brushIn = nil, ""
	}
}

func (p *mapParser) close() {
	switch p.depth {
	case 1:
		if !p.isIgnored(p.layer) {
			p.out = append(p.out, p.entity)
		}
	case 2:
		if !p.isIgnored(p.brushIn) {
			p.entity.materials = append(p.entity.materials, p.brush...)
		}
		p.patch = false
	}
	p.depth--
}

// isIgnored: the layer, or a layer above it ("a" above "a/b"), is flagged ignore.
func (p *mapParser) isIgnored(layer string) bool {
	for layer != "" {
		if p.ignored[layer] {
			return true
		}
		i := strings.LastIndexByte(layer, '/')
		if i < 0 {
			return false
		}
		layer = layer[:i]
	}
	return false
}

// layerDecl reads a header layer line, `"000_Global/No Comp" flags hidden ignore`.
func layerDecl(l string) (name, flags string, ok bool) {
	parts := strings.SplitN(l, `"`, 3)
	if len(parts) < 3 || parts[0] != "" {
		return "", "", false
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(parts[2]), "flags")
	return parts[1], strings.TrimSpace(rest), ok
}

// keyValue reads `"key" "value"`.
func keyValue(l string) (string, string, bool) {
	parts := strings.SplitN(l, `"`, 5)
	if len(parts) < 5 || strings.TrimSpace(parts[2]) != "" {
		return "", "", false
	}
	return parts[1], parts[3], true
}

// faceMaterial is the material of a brush face line: the first token after
// its three points.
func faceMaterial(l string) string {
	for range 3 {
		i := strings.IndexByte(l, ')')
		if i < 0 {
			return ""
		}
		l = l[i+1:]
	}
	if f := strings.Fields(l); len(f) > 0 {
		return f[0]
	}
	return ""
}

// isWord: a bare material name, not toolFlags, a guid or the patch's numbers.
func isWord(l string) bool {
	if l == "" || strings.ContainsAny(l, " ;(){}\"") {
		return false
	}
	c := l[0]
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
