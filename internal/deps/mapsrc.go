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
func readMap(r io.Reader) ([]mapEntity, error) {
	var out []mapEntity
	var cur *mapEntity
	depth, patch := 0, false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "{":
			depth++
			if depth == 1 {
				out = append(out, mapEntity{keys: map[string]string{}})
				cur = &out[len(out)-1]
			}
		case l == "}":
			depth--
		case cur == nil || depth == 0:
		case depth == 1 && strings.HasPrefix(l, `"`):
			if k, v, ok := keyValue(l); ok {
				cur.keys[strings.ToLower(k)] = v
			}
		case l == "curve" || l == "mesh":
			patch = true
		case patch && isWord(l):
			cur.materials = append(cur.materials, l)
			patch = false
		case strings.HasPrefix(l, "("):
			if m := faceMaterial(l); m != "" {
				cur.materials = append(cur.materials, m)
			}
		}
	}
	return out, sc.Err()
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
