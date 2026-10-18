package cli

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapFindArgs are map_find's filters and what to show of each entity.
type mapFindArgs struct {
	IDs      []string
	Class    string
	Key      string
	Near     string // "x y z" or "x,y,z"
	Radius   float64
	Selected bool
	Brushes  bool   // list each entity's brushes
	Keys     string // "" for the naming keys, "all", or a comma list
	Limit    int
	PID      int
}

// mapFindOut is a map_find answer.
type mapFindOut struct {
	Map      string     `json:"map"`
	Entities int        `json:"entities"`
	Brushes  int        `json:"brushes"`
	Selected int        `json:"selected,omitempty"` // brushes selected in Radiant
	Matched  int        `json:"matched"`
	Found    []foundEnt `json:"found"`
	More     string     `json:"more,omitempty"`
}

// foundEnt is one entity map_find matched.
type foundEnt struct {
	ID       string            `json:"id"`
	Rev      string            `json:"rev"` // what map_edit's revs want back
	Class    string            `json:"class"`
	Origin   []float64         `json:"origin,omitempty"`
	Bounds   []float64         `json:"bounds,omitempty"` // min x y z, then max x y z
	Count    int               `json:"brushes,omitempty"`
	Layer    string            `json:"layer,omitempty"`
	Selected bool              `json:"selected,omitempty"`
	Keys     map[string]string `json:"keys,omitempty"`
	List     []foundBrush      `json:"brush_list,omitempty"`
}

// foundBrush is one brush or patch of a matched entity.
type foundBrush struct {
	ID       string    `json:"id"`
	Rev      string    `json:"rev"`
	Kind     string    `json:"kind,omitempty"` // curve or mesh; a classic brush has none
	Material string    `json:"material"`
	Bounds   []float64 `json:"bounds,omitempty"`
	Contents string    `json:"contents,omitempty"`
	Selected bool      `json:"selected,omitempty"`
	// Rows are a patch's control points, row by row, as map_edit's patch op
	// takes them, when its id was named.
	Rows [][][3]float64 `json:"rows,omitempty"`
}

// namingKeys are the keys map_find shows unless asked for others: what names
// an entity and ties it to scripts and other entities.
var namingKeys = []string{"targetname", "target", "script_noteworthy", "script_string", "script_int", "name", "model"}

const (
	defaultFindLimit = 50
	maxBrushRows     = 200
	defaultRadius    = 256
)

// mapFind finds entities of the map open in Radiant.
func mapFind(ctx context.Context, a mapFindArgs) (*mapFindOut, error) {
	q := level.Query{IDs: a.IDs, Class: a.Class, Key: a.Key, Selected: a.Selected, Radius: a.Radius}
	if a.Near != "" {
		v, ok := geom.ParseVec3(strings.ReplaceAll(a.Near, ",", " "))
		if !ok {
			return nil, fmt.Errorf("near %q: give a point as x y z", a.Near)
		}
		q.Near = &v
		if q.Radius <= 0 {
			q.Radius = defaultRadius
		}
	}
	c, err := radiant.Connect(ctx, radiantMachine, a.PID)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	snap, err := level.Read(ctx, c)
	if err != nil {
		return nil, err
	}
	hits, err := level.Find(snap.Map, snap.IDs, q, snap.Sel)
	if err != nil {
		return nil, err
	}
	if q.Near != nil {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Box.Distance(*q.Near) < hits[j].Box.Distance(*q.Near) })
	}
	ents, brushes := level.Counts(snap.Map)
	out := &mapFindOut{Map: baseName(snap.State.Map), Entities: ents, Brushes: brushes, Selected: len(snap.Sel.Brushes), Matched: len(hits), Found: []foundEnt{}}
	limit := a.Limit
	if limit <= 0 {
		limit = defaultFindLimit
	}
	rows := 0
	for _, h := range hits {
		if len(out.Found) == limit || rows >= maxBrushRows {
			out.More = fmt.Sprintf("%d more: narrow the query, or raise limit", len(hits)-len(out.Found))
			break
		}
		f := foundOf(h, snap, a)
		rows += len(f.List)
		out.Found = append(out.Found, f)
	}
	return out, nil
}

func foundOf(h level.Hit, snap *level.Snapshot, a mapFindArgs) foundEnt {
	e := h.Entity
	f := foundEnt{ID: snap.IDs.Short(e.Guid()), Rev: level.Rev(e), Class: e.Class(), Layer: e.Layer(), Selected: h.Selected, Keys: keysOf(e, a.Keys)}
	if o, ok := geom.Origin(e); ok {
		f.Origin = vec(o)
	}
	for _, b := range e.Brushes() {
		if b.Kind() != mapfile.KindEmpty {
			f.Count++
		}
	}
	if f.Count > 0 && h.Box.Ok {
		f.Bounds = append(vec(h.Box.Min), vec(h.Box.Max)...)
	}
	list, named := h.Named, len(h.Named) > 0
	if a.Brushes && len(list) == 0 {
		list = e.Brushes()
	}
	for _, b := range list {
		if b.Kind() == mapfile.KindEmpty {
			continue
		}
		fb := foundBrush{ID: snap.IDs.Short(b.Guid()), Rev: level.BrushRev(b), Material: level.Material(b), Contents: b.Contents(),
			Selected: snap.Sel.Brush(b.Guid())}
		if k := b.Kind(); k != mapfile.KindFaces {
			fb.Kind = level.KindName(k)
			if named {
				fb.Rows = patchRows(b)
			}
		}
		if box := geom.BrushBox(b); box.Ok {
			fb.Bounds = append(vec(box.Min), vec(box.Max)...)
		}
		f.List = append(f.List, fb)
	}
	return f
}

// keysOf is the entity's keys to show: the naming ones by default, every one
// but classname and origin for "all", or those listed.
func keysOf(e *mapfile.Entity, which string) map[string]string {
	out := map[string]string{}
	switch strings.TrimSpace(which) {
	case "":
		for _, k := range namingKeys {
			if v, ok := e.Key(k); ok {
				out[k] = v
			}
		}
	case "all":
		for _, kv := range e.Keys() {
			if kv.Key != "classname" && kv.Key != "origin" {
				out[kv.Key] = kv.Value
			}
		}
	default:
		for _, k := range strings.Split(which, ",") {
			if v, ok := e.Key(strings.TrimSpace(k)); ok {
				out[strings.TrimSpace(k)] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// vec is a point for an answer, to a thousandth of a unit.
func vec(v mapfile.Vec3) []float64 {
	out := make([]float64, 3)
	for i, x := range v {
		out[i] = math.Round(x*1000)/1000 + 0 // + 0 turns -0 into 0
	}
	return out
}

// baseName is a Windows or slash path's last element.
func baseName(p string) string {
	return p[strings.LastIndexAny(p, `/\`)+1:]
}

// patchRows are a patch's control points, row by row.
func patchRows(b *mapfile.Brush) [][][3]float64 {
	p, err := b.Patch()
	if err != nil {
		return nil
	}
	out := make([][][3]float64, len(p.Rows))
	for r, row := range p.Rows {
		for _, v := range row {
			out[r] = append(out[r], [3]float64(v.Pos))
		}
	}
	return out
}
