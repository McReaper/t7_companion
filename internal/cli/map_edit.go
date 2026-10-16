package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapEditOut is a map_edit answer: the undo step it made, and what each op
// made or changed, read back from Radiant.
type mapEditOut struct {
	Map     string    `json:"map"`
	Undo    string    `json:"undo"` // the entry in Radiant's undo menu: one Ctrl+Z undoes the whole batch
	Deleted int       `json:"deleted,omitempty"`
	Made    []madeOut `json:"made,omitempty"`
	// Tiles is the tile each material new boxes and texture ops sized was
	// given, and Textured how many faces each texture op textured.
	Tiles    map[string]tileOut `json:"tiles,omitempty"`
	Textured []texturedOut      `json:"textured,omitempty"`
	// Script is what the zones made need of the map's script, Notes what
	// else they leave to see to.
	Script *scriptOut `json:"script,omitempty"`
	Notes  []string   `json:"notes,omitempty"`
}

// tileOut is a material's tile: world units across and down, and its source.
type tileOut struct {
	Size [2]float64 `json:"size"`
	From string     `json:"from"`
}

type texturedOut struct {
	Op    int `json:"op"`
	Faces int `json:"faces"`
}

// madeOut is what one op made or changed.
type madeOut struct {
	Op     int       `json:"op"`
	ID     string    `json:"id"`
	Was    string    `json:"was,omitempty"`   // its id before, when it was rebuilt under a new one
	Class  string    `json:"class,omitempty"` // none for a world brush
	Origin []float64 `json:"origin,omitempty"`
	Bounds []float64 `json:"bounds,omitempty"`
	Part   string    `json:"part,omitempty"` // what of a layout it is
	Note   string    `json:"note,omitempty"`
}

// decodeOps reads map_edit's ops, refusing a key no op has.
func decodeOps(raw []byte) ([]level.Op, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("ops: a JSON array of ops: %w", err)
	}
	ops := make([]level.Op, len(items))
	for i, it := range items {
		if err := decodeStrict(it, &ops[i]); err != nil {
			return nil, fmt.Errorf("op %d: %w", i, err)
		}
	}
	return ops, nil
}

// mapEdit applies ops to the map open in Radiant as one undo step.
func mapEdit(ctx context.Context, ops []level.Op, label string, pid int) (*mapEditOut, error) {
	c, err := radiant.Connect(ctx, radiantMachine, pid)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	snap, err := level.Read(ctx, c)
	if err != nil {
		return nil, err
	}
	var info func(string) level.MaterialInfo
	if root := installOf(snap.State.Map); root != "" {
		if w, err := workspace(root); err == nil {
			info = materialInfo(w, tilesIn(root))
		}
	}
	d, err := level.EditWith(ctx, c, snap, ops, label, mapTiles(snap.State.Map), info)
	if err != nil {
		return nil, err
	}
	out := &mapEditOut{Map: baseName(d.After.State.Map), Undo: d.Label, Deleted: d.Deleted, Notes: d.Notes}
	if len(d.Links) > 0 {
		out.Script = zoneScript(d.After.State.Map, d.Links)
	}
	for m, t := range d.Tiles {
		if out.Tiles == nil {
			out.Tiles = map[string]tileOut{}
		}
		out.Tiles[m] = tileOut{Size: [2]float64{t.W, t.H}, From: t.From}
	}
	for i := range ops {
		if n, ok := d.Faces[i]; ok {
			out.Textured = append(out.Textured, texturedOut{Op: i, Faces: n})
		}
	}
	for _, m := range d.Made {
		mo := madeOut{Op: m.Op, Class: m.Class, Part: m.Part}
		if m.ID == "" {
			mo.Note = "not found in the map read back: map_find to see what Radiant made"
		} else {
			mo.ID = d.After.IDs.Short(m.ID)
		}
		if m.Was != "" {
			mo.Was = snap.IDs.Short(m.Was)
		}
		switch {
		case !m.Box.Ok:
		case m.Box.Min == m.Box.Max && m.Class != "":
			mo.Origin = vec(m.Box.Min)
		default:
			mo.Bounds = append(vec(m.Box.Min), vec(m.Box.Max)...)
		}
		out.Made = append(out.Made, mo)
	}
	return out, nil
}
