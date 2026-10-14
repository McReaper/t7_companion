package level

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// CopyOp copies entities and world brushes: count copies, the first by By
// from the original, each next one By further, as a row of pillars or of wall
// panels is built from one.
type CopyOp struct {
	IDs   []string   `json:"ids"`
	By    [3]float64 `json:"by"`
	Count int        `json:"count,omitempty"` // none: 1
}

// maxCopies keeps a batch to what Radiant imports in one go without a pause
// the user would notice.
const maxCopies = 256

// copies runs a copy op: each copy is a new unit of op i, keys and materials
// as the original has them at that point of the batch, GUIDs left for the
// import to give.
func (b *batch) copies(i int, o *CopyOp) error {
	n := o.Count
	switch {
	case n == 0:
		n = 1
	case n < 0:
		return fmt.Errorf("copy: count %d: how many copies, 1 or more", n)
	}
	if o.By == (mapfile.Vec3{}) {
		return fmt.Errorf("copy: by is [0, 0, 0]: the copies would sit on the original")
	}
	if len(o.IDs) == 0 {
		return fmt.Errorf("copy: no ids")
	}
	var from []target
	for _, id := range o.IDs {
		ts, err := b.targets(id)
		if err != nil {
			return err
		}
		for _, t := range ts {
			if t.br != nil {
				return fmt.Errorf("copy: %s is one brush of a %s: copy the entity", id, t.u.ent.Class())
			}
		}
		from = append(from, ts...)
	}
	if n*len(from) > maxCopies {
		return fmt.Errorf("copy: %d copies of %d things is over %d in one batch", n, len(from), maxCopies)
	}
	for k := 1; k <= n; k++ {
		by := mapfile.Vec3{o.By[0] * float64(k), o.By[1] * float64(k), o.By[2] * float64(k)}
		for _, t := range from {
			if err := b.copyOf(i, t.u, by); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyOf adds a copy of a unit, moved by by.
func (b *batch) copyOf(i int, u *unit, by mapfile.Vec3) error {
	text := unguided(u.blockText())
	if u.world {
		text = EntityText("worldspawn", nil, text)
	}
	if err := b.made(i, text, u.world); err != nil {
		return err
	}
	c := b.units[len(b.units)-1]
	return move(c, nil, by)
}

// unguided is a unit's text without its GUID lines, the entity's and its
// brushes': Radiant's import gives the copy its own.
func unguided(text string) string {
	lines := strings.SplitAfter(text, "\n")
	out := lines[:0]
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), `guid "`) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "")
}

// target is what an id names: a unit, and the brush when the id names one
// brush of an entity.
type target struct {
	u  *unit
	br *mapfile.Brush
}

// targets is what an id names: an entity or brush of the map by its id, or
// "@n", everything op n of the batch made (each of a copy op's copies).
func (b *batch) targets(id string) ([]target, error) {
	n, ok := strings.CutPrefix(id, "@")
	if !ok {
		u, br, err := b.resolve(id)
		if err != nil {
			return nil, err
		}
		return []target{{u, br}}, nil
	}
	k, err := strconv.Atoi(n)
	if err != nil {
		return nil, fmt.Errorf("%s: @ and an earlier op's index, to name what it made", id)
	}
	var out []target
	gone := false
	for _, u := range b.units {
		if u.guid != "" || u.op != k {
			continue
		}
		if u.state == deleted {
			gone = true
			continue
		}
		out = append(out, target{u: u})
	}
	switch {
	case len(out) > 0:
		return out, nil
	case gone:
		return nil, fmt.Errorf("%s is deleted by an earlier op", id)
	}
	return nil, fmt.Errorf("%s: no earlier op %d made a box, an entity or a copy", id, k)
}
