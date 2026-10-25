package level

import (
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// Query says which entities Find returns; every filter given must hold.
type Query struct {
	IDs      []string      // entities or brushes, by id: an entity matches when it or one of its brushes is named
	Class    string        // classname, * matching any run of characters, any case
	Key      string        // "key" (has it) or "key=value", * in the value
	Near     *mapfile.Vec3 // with Radius: entities that come that close to the point, by their brushes when they own some
	Radius   float64
	Selected bool // only entities with a brush selected in Radiant
}

// Selection is what Radiant has selected: brush GUIDs and their entities'.
type Selection struct {
	Brushes, Entities map[string]bool
	GUIDs             []string // the selected brushes' GUIDs, in Radiant's order
	Owners            []string // the entity of each of GUIDs
}

// ParseSelection reads the bridge's selection lines, "<brush guid> <entity
// guid> <bounds>…".
func ParseSelection(body string) Selection {
	s := Selection{Brushes: map[string]bool{}, Entities: map[string]bool{}}
	for _, line := range strings.Split(body, "\n") {
		if f := strings.Fields(line); len(f) >= 2 {
			s.Brushes[hexOf(f[0])], s.Entities[hexOf(f[1])] = true, true
			s.GUIDs, s.Owners = append(s.GUIDs, f[0]), append(s.Owners, f[1])
		}
	}
	return s
}

// Brush reports whether the brush with that GUID is selected.
func (s Selection) Brush(guid string) bool { return s.Brushes[hexOf(guid)] }

// In is what the selection names in a snapshot of the map, once each: a
// brush by its GUID, a point entity by its own, as Radiant selects one
// through a brush the map's text doesn't hold.
func (s Selection) In(snap *Snapshot) []string {
	var out []string
	seen := map[string]bool{}
	for i, g := range s.GUIDs {
		if snap.RevOf(g) == "" {
			g = s.Owners[i]
		}
		if h := hexOf(g); !seen[h] {
			seen[h] = true
			out = append(out, g)
		}
	}
	return out
}

// Hit is one entity Find matched.
type Hit struct {
	Entity   *mapfile.Entity
	Box      geom.Box
	Selected bool
	Named    []*mapfile.Brush // the brushes of it the query named by id
	Near     []*mapfile.Brush // with Near, the brushes of it (of Named, when some are) within the radius, nearest first
	Distance float64          // with Near, how far its nearest such brush is, or a point entity's bounds
}

// Find returns the map's entities that match the query, in map order.
func Find(m *mapfile.Map, ids *IDs, q Query, sel Selection) ([]Hit, error) {
	named := map[string]bool{}
	for _, id := range q.IDs {
		g, err := ids.Resolve(id)
		if err != nil {
			return nil, err
		}
		named[hexOf(g)] = true
	}
	var out []Hit
	for _, e := range m.Entities {
		h := Hit{Entity: e, Selected: sel.Entities[hexOf(e.Guid())]}
		if q.matches(e, named, &h) {
			out = append(out, h)
		}
	}
	return out, nil
}

// matches tells whether the entity passes every filter, filling in the hit's
// named brushes and box on the way.
func (q Query) matches(e *mapfile.Entity, named map[string]bool, h *Hit) bool {
	if len(named) > 0 {
		for _, b := range e.Brushes() {
			if named[hexOf(b.Guid())] {
				h.Named = append(h.Named, b)
			}
		}
		if !named[hexOf(e.Guid())] && len(h.Named) == 0 {
			return false
		}
	}
	if (q.Class != "" && !glob(q.Class, e.Class())) || (q.Selected && !h.Selected) || !q.keyMatches(e) {
		return false
	}
	h.Box = geom.EntityBox(e)
	return q.Near == nil || q.near(e, h)
}

// near tells whether the entity comes within the radius of the point. One
// that owns brushes does by its brushes, which go in the hit's Near: the
// worldspawn's bounds span the map and would match any point. A point entity
// does by its bounds.
func (q Query) near(e *mapfile.Entity, h *Hit) bool {
	brushes := h.Named
	if len(brushes) == 0 {
		brushes = e.Brushes()
	}
	type found struct {
		b *mapfile.Brush
		d float64
	}
	var in []found
	owns := false
	for _, b := range brushes {
		box := geom.BrushBox(b)
		if !box.Ok { // a point entity's empty brush has no box
			continue
		}
		owns = true
		if d := box.Distance(*q.Near); d <= q.Radius {
			in = append(in, found{b, d})
		}
	}
	if !owns {
		h.Distance = h.Box.Distance(*q.Near)
		return h.Box.Ok && h.Distance <= q.Radius
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].d < in[j].d })
	for _, f := range in {
		h.Near = append(h.Near, f.b)
	}
	if len(in) > 0 {
		h.Distance = in[0].d
	}
	return len(in) > 0
}

func (q Query) keyMatches(e *mapfile.Entity) bool {
	if q.Key == "" {
		return true
	}
	key, value, hasValue := strings.Cut(q.Key, "=")
	v, ok := e.Key(strings.TrimSpace(key))
	return ok && (!hasValue || glob(strings.TrimSpace(value), v))
}

// glob matches s against a pattern where * is any run of characters, in any
// case.
func glob(pattern, s string) bool {
	parts := strings.Split(strings.ToLower(pattern), "*")
	s = strings.ToLower(s)
	if len(parts) == 1 {
		return s == parts[0]
	}
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(s, p)
		if i < 0 {
			return false
		}
		s = s[i+len(p):]
	}
	return strings.HasSuffix(s, last)
}

// Material is what a brush is drawn with: a patch's material, or the one most
// of a classic brush's faces use (the first of them on a tie).
func Material(b *mapfile.Brush) string {
	switch b.Kind() {
	case mapfile.KindCurve, mapfile.KindMesh:
		if p, err := b.Patch(); err == nil {
			return p.Material
		}
	case mapfile.KindFaces:
		faces, err := b.Faces()
		if err != nil {
			return ""
		}
		count := map[string]int{}
		var order []string
		for _, f := range faces {
			if count[f.Material] == 0 {
				order = append(order, f.Material)
			}
			count[f.Material]++
		}
		sort.SliceStable(order, func(i, j int) bool { return count[order[i]] > count[order[j]] })
		if len(order) > 0 {
			return order[0]
		}
	}
	return ""
}

// KindName is a brush kind as the map tools show it.
func KindName(k mapfile.Kind) string {
	switch k {
	case mapfile.KindCurve:
		return "curve"
	case mapfile.KindMesh:
		return "mesh"
	case mapfile.KindFaces:
		return "brush"
	}
	return "empty"
}

// GUIDs are every entity's and brush's GUID in the map, for NewIDs.
func GUIDs(m *mapfile.Map) []string {
	var out []string
	for _, e := range m.Entities {
		out = append(out, e.Guid())
		for _, b := range e.Brushes() {
			out = append(out, b.Guid())
		}
	}
	return out
}

// Counts are a map's entities and its brushes, a point entity's empty one aside.
func Counts(m *mapfile.Map) (entities, brushes int) {
	for _, e := range m.Entities {
		for _, b := range e.Brushes() {
			if b.Kind() != mapfile.KindEmpty {
				brushes++
			}
		}
	}
	return len(m.Entities), brushes
}
