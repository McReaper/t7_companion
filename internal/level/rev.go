package level

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// A rev is what was read of an entity or a brush: a hash of its text as
// Radiant has it, its comments left out (Radiant renumbers its "// brush N"
// and "// entity N" lines when anything before them is added or deleted).
// map_edit takes the revs the agent read and refuses a batch that names
// something whose text is no longer the same: the user changed it meanwhile,
// in the Radiant they share.

// Rev is an entity's rev, its brushes included.
func Rev(e *mapfile.Entity) string { return revOf(e.Bytes()) }

// BrushRev is a brush's rev.
func BrushRev(b *mapfile.Brush) string { return revOf([]byte(brushText(b))) }

func revOf(text []byte) string {
	h := fnv.New32a()
	for _, l := range bytes.SplitAfter(text, []byte("\n")) {
		if t := bytes.TrimSpace(l); len(t) > 0 && !bytes.HasPrefix(t, []byte("//")) {
			h.Write(t)
			h.Write([]byte{'\n'})
		}
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

// RevOf is the rev of the entity or brush with that GUID in the snapshot, ""
// when it has none.
func (s *Snapshot) RevOf(guid string) string {
	h := hexOf(guid)
	for _, e := range s.Map.Entities {
		if hexOf(e.Guid()) == h {
			return Rev(e)
		}
		for _, b := range e.Brushes() {
			if hexOf(b.Guid()) == h {
				return BrushRev(b)
			}
		}
	}
	return ""
}

// WithRevs makes the edit refuse a batch naming something changed since it
// was read: revs are map_find's, by id; with require, every entity or brush
// of the map an op names must be given its rev.
func WithRevs(revs map[string]string, require bool) EditOption {
	return func(b *batch) { b.revs, b.requireRevs = revs, require }
}

// names are the entities and brushes of the map an op names (@n, what the
// batch made, is its own).
func (o Op) names() []string {
	out := append([]string(nil), o.Delete...)
	if o.Set != nil {
		out = append(out, o.Set.ID)
	}
	for _, ids := range [][]string{idsOf(o.Move), idsOf(o.Material), idsOf(o.Texture), idsOf(o.Copy)} {
		out = append(out, ids...)
	}
	return out
}

func idsOf(op any) []string {
	switch o := op.(type) {
	case *MoveOp:
		if o != nil {
			return o.IDs
		}
	case *MaterialOp:
		if o != nil {
			return o.IDs
		}
	case *TextureOp:
		if o != nil {
			return o.IDs
		}
	case *CopyOp:
		if o != nil {
			return o.IDs
		}
	}
	return nil
}

// revAt is the current rev of what an id names, and its GUID's key.
func (b *batch) revAt(id string) (string, string, error) {
	g, err := b.snap.IDs.Resolve(id)
	if err != nil {
		return "", "", err
	}
	h := hexOf(g)
	u := b.by[h]
	switch {
	case u == nil || h == b.world:
		return "", "", fmt.Errorf("%s is no entity or brush", id)
	case u.world:
		return BrushRev(u.brush), h, nil
	case b.brush[h] != nil:
		return BrushRev(b.brush[h]), h, nil
	}
	return Rev(u.ent), h, nil
}

// checkRevs refuses the batch, before any op runs, when what it names was
// changed or deleted since it was read, or (required) was named unread.
func (b *batch) checkRevs(ops []Op) error {
	read := map[string]bool{}
	var changed, gone, unread []string
	for id, rev := range b.revs {
		if g, err := b.snap.IDs.Resolve(id); err == nil && hexOf(g) == b.world {
			continue // the worldspawn's text is every world brush's: its rev says nothing of a brush an op names
		}
		cur, key, err := b.revAt(id)
		switch {
		case err != nil:
			gone = append(gone, id)
		case cur != rev:
			changed = append(changed, id)
		}
		read[key] = true
	}
	if b.requireRevs {
		for i, op := range ops {
			for _, id := range op.names() {
				if _, key, err := b.revAt(id); err == nil && !read[key] && !strings.HasPrefix(id, "@") {
					unread = append(unread, fmt.Sprintf("op %d's %s", i, id))
				}
			}
		}
	}
	var why []string
	for _, k := range []struct {
		what string
		ids  []string
	}{{"changed since it was read", changed}, {"gone", gone}, {"named without the rev map_find gave it", unread}} {
		if len(k.ids) > 0 {
			sort.Strings(k.ids)
			why = append(why, strings.Join(k.ids, ", ")+": "+k.what)
		}
	}
	if len(why) > 0 {
		return fmt.Errorf("map_edit: %s. Nothing was sent: map_find them again, check the plan still holds, and resend with their revs", strings.Join(why, "; "))
	}
	return nil
}
