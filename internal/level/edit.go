package level

import (
	"fmt"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// Op is one edit of a batch: exactly one of its fields is set.
type Op struct {
	Box      *BoxOp      `json:"box,omitempty"`
	Entity   *EntityOp   `json:"entity,omitempty"`
	Delete   []string    `json:"delete,omitempty"`
	Set      *SetOp      `json:"set,omitempty"`
	Move     *MoveOp     `json:"move,omitempty"`
	Material *MaterialOp `json:"material,omitempty"`
}

// BoxOp adds an axis-aligned box: a world brush, or the brush of a new brush
// entity of class.
type BoxOp struct {
	Min      [3]float64        `json:"min"`
	Max      [3]float64        `json:"max"`
	Material string            `json:"material"`
	Contents string            `json:"contents,omitempty"`
	Class    string            `json:"class,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`
}

// EntityOp adds a point entity.
type EntityOp struct {
	Class  string            `json:"class"`
	Origin [3]float64        `json:"origin"`
	Angles *[3]float64       `json:"angles,omitempty"`
	Keys   map[string]string `json:"keys,omitempty"`
}

// SetOp sets an entity's keys; a null value removes one.
type SetOp struct {
	ID   string             `json:"id"`
	Keys map[string]*string `json:"keys"`
}

// MoveOp moves entities and brushes by an offset.
type MoveOp struct {
	IDs []string   `json:"ids"`
	By  [3]float64 `json:"by"`
}

// MaterialOp gives brushes, or all of an entity's, a material.
type MaterialOp struct {
	IDs      []string `json:"ids"`
	Material string   `json:"material"`
}

// unit is what a batch edits as a whole: a world brush, or an entity with its
// brushes.
type unit struct {
	guid  string          // the map's GUID, "" for a new unit
	ent   *mapfile.Entity // an entity's text
	brush *mapfile.Brush  // or a world brush's
	text  string          // a new unit's text: an entity block, or a world brush block
	world bool            // a world brush
	point bool            // a point entity of the map, which Radiant replaces in place
	state state
	op    int // the op that made it, or changed it last
}

type state int

const (
	kept state = iota
	changed
	deleted
	created
)

// batch is the map as a batch of ops leaves it, unit by unit.
type batch struct {
	snap  *Snapshot
	units []*unit
	by    map[string]*unit          // GUID hex (entity, world brush, entity brush) -> its unit
	brush map[string]*mapfile.Brush // GUID hex -> the brush, for an entity's brush named alone
	world string                    // worldspawn's GUID hex
}

func newBatch(snap *Snapshot) *batch {
	b := &batch{snap: snap, by: map[string]*unit{}, brush: map[string]*mapfile.Brush{}}
	for _, e := range snap.Map.Entities {
		if e.Class() == "worldspawn" {
			b.world = hexOf(e.Guid())
			for _, br := range e.Brushes() {
				if br.Kind() != mapfile.KindEmpty {
					u := &unit{guid: br.Guid(), brush: br, world: true}
					b.units = append(b.units, u)
					b.by[hexOf(br.Guid())] = u
				}
			}
			continue
		}
		u := &unit{guid: e.Guid(), ent: e, point: inPlace(e)}
		b.units = append(b.units, u)
		b.by[hexOf(e.Guid())] = u
		for _, br := range e.Brushes() {
			b.by[hexOf(br.Guid())], b.brush[hexOf(br.Guid())] = u, br
		}
	}
	return b
}

// apply runs the ops in order over the batch.
func (b *batch) apply(ops []Op) error {
	if len(ops) == 0 {
		return fmt.Errorf("no ops: give box, entity, delete, set, move or material ops")
	}
	for i, op := range ops {
		if err := b.applyOne(i, op); err != nil {
			return fmt.Errorf("op %d: %w", i, err)
		}
	}
	return nil
}

func (b *batch) applyOne(i int, op Op) error {
	n := 0
	for _, set := range []bool{op.Box != nil, op.Entity != nil, op.Delete != nil, op.Set != nil, op.Move != nil, op.Material != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("an op is exactly one of box, entity, delete, set, move, material (this one has %d)", n)
	}
	switch {
	case op.Box != nil:
		return b.box(i, op.Box)
	case op.Entity != nil:
		return b.entity(i, op.Entity)
	case op.Delete != nil:
		return b.delete(i, op.Delete)
	case op.Set != nil:
		return b.set(i, op.Set)
	case op.Move != nil:
		return b.each(i, op.Move.IDs, func(u *unit, br *mapfile.Brush) error { return move(u, br, op.Move.By) })
	default:
		if !word(op.Material.Material) {
			return fmt.Errorf("material %q: one word, a material's name", op.Material.Material)
		}
		return b.each(i, op.Material.IDs, func(u *unit, br *mapfile.Brush) error { return paint(u, br, op.Material.Material) })
	}
}

func (b *batch) box(i int, o *BoxOp) error {
	for k := range 3 {
		if o.Max[k] <= o.Min[k] {
			return fmt.Errorf("box: max %v must exceed min %v on every axis", o.Max, o.Min)
		}
	}
	if !word(o.Material) {
		return fmt.Errorf("box: material %q: one word, a material's name", o.Material)
	}
	if o.Contents != "" && !cleanText(o.Contents) {
		return fmt.Errorf("box: contents %q", o.Contents)
	}
	brush := BoxBrush(Box{Min: o.Min, Max: o.Max, Ok: true}, o.Material, o.Contents)
	if o.Class == "" {
		if len(o.Keys) > 0 {
			return fmt.Errorf("box: keys belong to a brush entity: give its class")
		}
		b.units = append(b.units, &unit{text: brush, world: true, state: created, op: i})
		return nil
	}
	if err := checkKeys(o.Class, o.Keys); err != nil {
		return fmt.Errorf("box: %w", err)
	}
	b.units = append(b.units, &unit{text: EntityText(o.Class, o.Keys, brush), state: created, op: i})
	return nil
}

func (b *batch) entity(i int, o *EntityOp) error {
	if err := checkKeys(o.Class, o.Keys); err != nil {
		return fmt.Errorf("entity: %w", err)
	}
	keys := map[string]string{"origin": vecText(o.Origin)}
	for k, v := range o.Keys {
		keys[k] = v
	}
	if o.Angles != nil {
		keys["angles"] = vecText(*o.Angles)
	}
	b.units = append(b.units, &unit{text: EntityText(o.Class, keys), state: created, op: i})
	return nil
}

func (b *batch) delete(i int, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("delete: no ids")
	}
	return b.each(i, ids, func(u *unit, br *mapfile.Brush) error {
		if br == nil {
			u.state = deleted
			return nil
		}
		for k, it := range u.ent.Items { // one brush of an entity: the entity is rebuilt without it
			if it.Brush == br {
				u.ent.Items = append(u.ent.Items[:k], u.ent.Items[k+1:]...)
				break
			}
		}
		return nil
	})
}

func (b *batch) set(i int, o *SetOp) error {
	if len(o.Keys) == 0 {
		return fmt.Errorf("set: no keys")
	}
	u, _, err := b.resolve(o.ID)
	if err != nil {
		return err
	}
	if u.world {
		return fmt.Errorf("set: %s is a world brush: keys are an entity's", o.ID)
	}
	for k, v := range o.Keys {
		switch {
		case k == "classname" || k == "guid" || k == "":
			return fmt.Errorf("set: %q cannot be set", k)
		case !cleanText(k) || (v != nil && !cleanText(*v)):
			return fmt.Errorf("set: key %q or its value holds a double quote or a line break", k)
		}
	}
	setKeys(u.ent, o.Keys)
	u.state, u.op = changed, i
	return nil
}

// each runs fn on what each id names: its unit, and the brush when the id
// names one brush of an entity.
func (b *batch) each(i int, ids []string, fn func(*unit, *mapfile.Brush) error) error {
	if len(ids) == 0 {
		return fmt.Errorf("no ids")
	}
	for _, id := range ids {
		u, br, err := b.resolve(id)
		if err != nil {
			return err
		}
		if err := fn(u, br); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		if u.state != deleted {
			u.state = changed
		}
		u.op = i
	}
	return nil
}

func (b *batch) resolve(id string) (*unit, *mapfile.Brush, error) {
	g, err := b.snap.IDs.Resolve(id)
	if err != nil {
		return nil, nil, err
	}
	h := hexOf(g)
	if h == b.world {
		return nil, nil, fmt.Errorf("%s is worldspawn: name its brushes", id)
	}
	u := b.by[h]
	switch {
	case u == nil:
		return nil, nil, fmt.Errorf("%s is no entity or brush map_edit can edit", id)
	case u.state == deleted:
		return nil, nil, fmt.Errorf("%s is deleted by an earlier op", id)
	}
	return u, b.brush[h], nil
}

// move moves a world brush, one brush of an entity, or a whole entity (its
// brushes and its origin).
func move(u *unit, br *mapfile.Brush, by mapfile.Vec3) error {
	switch {
	case u.world:
		return translate(u.brush, by)
	case br != nil:
		return translate(br, by)
	}
	for _, x := range u.ent.Brushes() {
		if err := translate(x, by); err != nil {
			return err
		}
	}
	if o, ok := Origin(u.ent); ok {
		v := vecText(mapfile.Vec3{o[0] + by[0], o[1] + by[1], o[2] + by[2]})
		setKeys(u.ent, map[string]*string{"origin": &v})
	}
	return nil
}

// paint gives a world brush, one brush of an entity, or all of an entity's, a
// material.
func paint(u *unit, br *mapfile.Brush, material string) error {
	switch {
	case u.world:
		return retexture(u.brush, material)
	case br != nil:
		return retexture(br, material)
	}
	n := 0
	for _, x := range u.ent.Brushes() {
		if x.Kind() == mapfile.KindEmpty {
			continue
		}
		if err := retexture(x, material); err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("a %s has no brush to give a material", u.ent.Class())
	}
	return nil
}

func checkKeys(class string, keys map[string]string) error {
	if !word(class) {
		return fmt.Errorf("class %q: one word, an entity class", class)
	}
	for k, v := range keys {
		if k == "" || k == "classname" || k == "guid" || !cleanText(k) || !cleanText(v) {
			return fmt.Errorf("key %q: not one map_edit can write (classname and guid are its own; no double quote or line break)", k)
		}
	}
	return nil
}

// word is a material or class name: one word Radiant can read.
func word(s string) bool { return s != "" && !strings.ContainsAny(s, " \t\"\r\n{}()") }

func vecText(v mapfile.Vec3) string { return num(v[0]) + " " + num(v[1]) + " " + num(v[2]) }
