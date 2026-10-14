package level

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// The text map_edit writes for Radiant's import: what Radiant itself writes,
// without the GUIDs, which the import gives.

// texDefaults are a new face's texture numbers: Radiant's defaults, the
// material's scale, no shift or rotation, and the lightmap's.
const texDefaults = "64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0"

// BoxBrush is the text of an axis-aligned box brush, its faces wound as
// Radiant winds them ((p1-p0)×(p2-p0) into the brush).
func BoxBrush(b Box, material, contents string) string {
	lo, hi := b.Min, b.Max
	v := func(x, y, z float64) string { return "( " + num(x) + " " + num(y) + " " + num(z) + " )" }
	faces := [6][3]string{
		{v(lo[0], lo[1], lo[2]), v(lo[0]+1, lo[1], lo[2]), v(lo[0], lo[1]+1, lo[2])}, // bottom
		{v(lo[0], lo[1], hi[2]), v(lo[0], lo[1]+1, hi[2]), v(lo[0]+1, lo[1], hi[2])}, // top
		{v(lo[0], lo[1], lo[2]), v(lo[0], lo[1]+1, lo[2]), v(lo[0], lo[1], lo[2]+1)}, // -x
		{v(hi[0], lo[1], lo[2]), v(hi[0], lo[1], lo[2]+1), v(hi[0], lo[1]+1, lo[2])}, // +x
		{v(lo[0], lo[1], lo[2]), v(lo[0], lo[1], lo[2]+1), v(lo[0]+1, lo[1], lo[2])}, // -y
		{v(lo[0], hi[1], lo[2]), v(lo[0]+1, hi[1], lo[2]), v(lo[0], hi[1], lo[2]+1)}, // +y
	}
	var s strings.Builder
	s.WriteString("{\n")
	if contents != "" {
		s.WriteString(" contents " + strings.TrimSuffix(contents, ";") + ";\n")
	}
	for _, f := range faces {
		s.WriteString(" " + f[0] + " " + f[1] + " " + f[2] + " " + material + " " + texDefaults + "\n")
	}
	s.WriteString("}\n")
	return s.String()
}

// EntityText is an entity's text: its class, its keys (sorted, so the text is
// the same from one call to the next) and its brushes' text.
func EntityText(class string, keys map[string]string, brushes ...string) string {
	var s strings.Builder
	s.WriteString("{\n" + kv("classname", class))
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		s.WriteString(kv(k, keys[k]))
	}
	for _, b := range brushes {
		s.WriteString(b)
	}
	s.WriteString("}\n")
	return s.String()
}

func kv(k, v string) string { return `"` + k + `" "` + v + `"` + "\n" }

// cleanText reports whether a key or value can be written: Radiant's rule,
// no double quote or line break.
func cleanText(s string) bool { return !strings.ContainsAny(s, "\"\r\n") }

// setKeys changes an entity's keys in place: a nil value removes the key, a
// key it lacks goes after its last key.
func setKeys(e *mapfile.Entity, keys map[string]*string) {
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v := keys[k]
		at := -1
		for i, it := range e.Items {
			if it.Brush == nil && keyName(it.Line) == k {
				at = i
				break
			}
		}
		switch {
		case at >= 0 && v == nil:
			e.Items = append(e.Items[:at], e.Items[at+1:]...)
		case at >= 0:
			e.Items[at].Line = lead(e.Items[at].Line) + strings.TrimSuffix(kv(k, *v), "\n") + eol(e.Items[at].Line)
		case v != nil:
			e.Items = insertKey(e.Items, kv(k, *v))
		}
	}
}

// insertKey puts a key line after the entity's last own line before its
// brushes.
func insertKey(items []mapfile.Item, line string) []mapfile.Item {
	at := len(items)
	for i, it := range items {
		if it.Brush != nil {
			at = i
			break
		}
	}
	out := append([]mapfile.Item{}, items[:at]...)
	out = append(out, mapfile.Item{Line: line})
	return append(out, items[at:]...)
}

// keyName is the key of a `"key" "value"` line, "" for any other line.
func keyName(line string) string {
	l := strings.TrimSpace(line)
	if !strings.HasPrefix(l, `"`) {
		return ""
	}
	k, _, ok := strings.Cut(l[1:], `"`)
	if !ok {
		return ""
	}
	return k
}

func lead(l string) string { return l[:len(l)-len(strings.TrimLeft(l, " \t"))] }

func eol(l string) string {
	if strings.HasSuffix(l, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(l, "\n") {
		return "\n"
	}
	return ""
}

// translate moves a brush: its face points and its patch vertices.
func translate(b *mapfile.Brush, d mapfile.Vec3) error {
	return rewrite(b, func(w []string) error {
		for i := range 3 {
			at := i * 5
			for k := range 3 {
				if err := shift(w, at+1+k, d[k]); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(w []string) error {
		for k := range 3 {
			if err := shift(w, 1+k, d[k]); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}

// retexture gives every face of a brush, or its patch, the material.
func retexture(b *mapfile.Brush, material string) error {
	return rewrite(b, func(w []string) error {
		w[15] = material
		return nil
	}, nil, func(w []string) { w[0] = material })
}

// rewrite runs face on each face line's words, vertex on each patch vertex
// line's, and name on a patch's material line, keeping each line's indent
// and line end.
func rewrite(b *mapfile.Brush, face, vertex func([]string) error, name func([]string)) error {
	depth, names := 0, 0
	for i, raw := range b.Body {
		l := strings.TrimSpace(raw)
		switch l {
		case "{":
			depth++
			continue
		case "}":
			depth--
			continue
		}
		w := strings.Fields(l)
		fn := lineAction(l, w, depth, &names, face, vertex, name)
		if fn == nil {
			continue
		}
		if err := fn(w); err != nil {
			return fmt.Errorf("brush %s: %w", b.Guid(), err)
		}
		b.Body[i] = lead(raw) + strings.Join(w, " ") + eol(raw)
	}
	return nil
}

// lineAction is what rewrite runs on one line of a brush, nil to leave it: a
// face line at the brush's own depth, a vertex line or the material line (the
// first single word that is no setting) inside its patch block.
func lineAction(l string, w []string, depth int, names *int, face, vertex func([]string) error, name func([]string)) func([]string) error {
	switch {
	case depth == 0 && strings.HasPrefix(l, "(") && face != nil:
		return func(w []string) error {
			if len(w) < 29 {
				return fmt.Errorf("a face of %d fields", len(w))
			}
			return face(w)
		}
	case depth > 0 && len(w) > 0 && w[0] == "v" && vertex != nil:
		return func(w []string) error {
			if len(w) < 4 {
				return fmt.Errorf("a patch vertex of %d fields", len(w))
			}
			return vertex(w)
		}
	case depth > 0 && len(w) == 1 && !strings.HasSuffix(l, ";") && l != "(" && l != ")":
		if *names++; *names == 1 && name != nil {
			return func(w []string) error { name(w); return nil }
		}
	}
	return nil
}

func shift(w []string, i int, d float64) error {
	v, err := strconv.ParseFloat(w[i], 64)
	if err != nil {
		return fmt.Errorf("%q is no number", w[i])
	}
	w[i] = num(v + d)
	return nil
}

// brushText is a brush's text with its lead comments.
func brushText(b *mapfile.Brush) string {
	return strings.Join(b.Lead, "") + b.Open + strings.Join(b.Body, "") + b.Close
}
