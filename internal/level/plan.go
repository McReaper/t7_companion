package level

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// plan is what a batch sends the bridge's edit: the GUIDs to delete, the point
// entities to replace in place (they keep their GUIDs), and the world brushes
// and entities to import (new, or rebuilt: an entity that owns brushes, or a
// world brush, comes back under a new GUID, as only that makes Radiant's undo
// bring back the old one exactly).
type plan struct {
	deletes []string
	replace []*unit
	world   []*unit
	ents    []*unit
}

func (b *batch) plan() (*plan, error) {
	p := &plan{}
	for _, u := range b.units {
		switch u.state {
		case deleted:
			if u.guid != "" { // what the batch made and deleted again is not sent
				p.deletes = append(p.deletes, u.guid)
			}
		case changed:
			switch {
			case u.world:
				p.deletes, p.world = append(p.deletes, u.guid), append(p.world, u)
			case u.point:
				p.replace = append(p.replace, u)
			default:
				p.deletes, p.ents = append(p.deletes, u.guid), append(p.ents, u)
			}
		case created:
			if u.world {
				p.world = append(p.world, u)
			} else {
				p.ents = append(p.ents, u)
			}
		}
	}
	if len(p.deletes)+len(p.replace)+len(p.world)+len(p.ents) == 0 {
		return nil, fmt.Errorf("the ops change nothing")
	}
	return p, nil
}

// inPlace tells an entity of the map Radiant replaces in place, keeping its
// GUID: a point entity, but for the two the bridge refuses. It is judged on
// the map's entity, before the ops: one that loses its brushes is still
// Radiant's brush entity.
func inPlace(e *mapfile.Entity) bool {
	for _, br := range e.Brushes() {
		if br.Kind() != mapfile.KindEmpty {
			return false
		}
	}
	return e.Class() != "reflection_probe" && e.Class() != "misc_prefab"
}

// body is the bridge edit's body.
func (p *plan) body() string {
	var s strings.Builder
	for i := 0; i < len(p.deletes); i += 32 {
		s.WriteString("delete " + strings.Join(p.deletes[i:min(i+32, len(p.deletes))], " ") + "\n")
	}
	for _, u := range p.replace {
		s.WriteString("replace " + u.guid + "\n" + string(u.ent.Bytes()))
	}
	if len(p.world)+len(p.ents) > 0 {
		s.WriteString("import\n{\n\"classname\" \"worldspawn\"\n")
		for _, u := range p.world {
			s.WriteString(u.blockText())
		}
		s.WriteString("}\n")
		for _, u := range p.ents {
			s.WriteString(u.blockText())
		}
	}
	return s.String()
}

func (u *unit) blockText() string {
	if u.world {
		return brushText(u.brush)
	}
	return string(u.ent.Bytes())
}

// Made is one thing a batch made or changed, as Radiant has it afterwards.
type Made struct {
	Op    int
	ID    string // its GUID now
	Was   string // its GUID before, when it was rebuilt under a new one
	Class string // "" for a world brush
	Box   Box
}

// Done is a batch's outcome.
type Done struct {
	Label   string
	Deleted int // brushes, a point entity's own one included
	Made    []Made
	Tiles   map[string]Tile // the tile each material sized by the batch was given
	Faces   map[int]int     // texture op -> how many faces it textured
	After   *Snapshot       // the map as Radiant has it afterwards
}

// Edit applies the ops to the map open in the Radiant as one undo step
// labelled label, then reads the map back and finds in it what each op made.
// tiles sizes the materials of new boxes and texture ops (nil assumes 64
// units).
func Edit(ctx context.Context, c *radiant.Client, snap *Snapshot, ops []Op, label string, tiles Tiles) (*Done, error) {
	b := newBatch(snap, tiles)
	if err := b.apply(ops); err != nil {
		return nil, err
	}
	p, err := b.plan()
	if err != nil {
		return nil, err
	}
	if label = strings.TrimSpace(label); label == "" {
		label = summary(ops)
	}
	r, err := c.Call(ctx, "edit t7kb: "+strings.ReplaceAll(label, "\n", " "), []byte(p.body()))
	if err != nil {
		return nil, err
	}
	after, err := Read(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("the edit is done (one Ctrl+Z undoes it), but the map could not be read back: %w", err)
	}
	d := &Done{Label: "t7kb: " + strings.TrimPrefix(label, "t7kb: "), After: after, Tiles: b.used, Faces: b.faces} // a label already marked once
	d.Deleted, _ = strconv.Atoi(strings.TrimPrefix(r.Summary, "deleted="))
	d.Made = p.match(fresh(string(r.Body)), after)
	sort.SliceStable(d.Made, func(i, j int) bool { return d.Made[i].Op < d.Made[j].Op })
	return d, nil
}

// summary names a batch for Radiant's undo menu: its ops, counted.
func summary(ops []Op) string {
	n := make([]int, len(opKinds))
	for _, op := range ops {
		for _, k := range op.kinds() {
			n[k]++
		}
	}
	var parts []string
	for i, name := range opKinds {
		if n[i] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[i], name))
		}
	}
	return strings.Join(parts, ", ")
}

// fresh are the GUIDs the edit's reply lists: what it imported.
func fresh(body string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.Fields(body) {
		out[hexOf(f)] = true
	}
	return out
}

// match finds, in the map read back, what each unit of the plan became: a
// replaced entity by its GUID, an imported one among the reply's GUIDs by its
// class and box (the reply's order is Radiant's selection's, not the text's).
func (p *plan) match(got map[string]bool, after *Snapshot) []Made {
	var out []Made
	taken := map[string]bool{}
	for _, u := range append(append(append([]*unit{}, p.world...), p.ents...), p.replace...) {
		class, box := u.shape()
		m := Made{Op: u.op, Class: class, Box: box}
		if u.state == changed && u.point {
			m.ID = u.guid
		} else {
			m.ID = findShape(after, got, taken, class, box, u.world)
			if u.state == changed {
				m.Was = u.guid
			}
		}
		taken[hexOf(m.ID)] = true
		out = append(out, m)
	}
	return out
}

// shape is a unit's class ("" for a world brush) and box, from the text sent.
func (u *unit) shape() (string, Box) {
	if u.world {
		return "", BrushBox(u.brush)
	}
	return u.ent.Class(), EntityBox(u.ent)
}

// findShape is the GUID of the first thing read back, among those imported and
// not yet matched, with that class and box.
func findShape(after *Snapshot, got, taken map[string]bool, class string, box Box, world bool) string {
	for _, e := range after.Map.Entities {
		if world {
			if e.Class() != "worldspawn" {
				continue
			}
			for _, br := range e.Brushes() {
				if h := hexOf(br.Guid()); got[h] && !taken[h] && sameBox(BrushBox(br), box) {
					return br.Guid()
				}
			}
			continue
		}
		if h := hexOf(e.Guid()); got[h] && !taken[h] && e.Class() == class && sameBox(EntityBox(e), box) {
			return e.Guid()
		}
	}
	return ""
}

func sameBox(a, b Box) bool {
	if a.Ok != b.Ok {
		return false
	}
	for i := range 3 {
		if math.Abs(a.Min[i]-b.Min[i]) > 0.01 || math.Abs(a.Max[i]-b.Max[i]) > 0.01 {
			return false
		}
	}
	return true
}
