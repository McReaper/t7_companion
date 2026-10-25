package cli

import (
	"context"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level"
	"github.com/McReaper/t7_companion/internal/level/geom"
	"github.com/McReaper/t7_companion/internal/radiant"
)

// mapCommandOut is a map_command answer: what the command did to the map, or
// a menu's entries.
type mapCommandOut struct {
	Map      string      `json:"map"`
	Command  string      `json:"command,omitempty"`
	Undo     string      `json:"undo,omitempty"` // the undo menu's entry it made
	Created  []madeThing `json:"created,omitempty"`
	Deleted  []string    `json:"deleted,omitempty"`
	Selected []string    `json:"selected,omitempty"` // what it left selected, before the user's selection came back
	Menu     string      `json:"menu,omitempty"`
	Entries  []string    `json:"entries,omitempty"` // "label", "label >" for a submenu, "label (disabled)"
}

// madeThing is an entity or brush a command made.
type madeThing struct {
	ID       string    `json:"id"`
	Class    string    `json:"class,omitempty"`    // an entity's
	Material string    `json:"material,omitempty"` // a brush's
	Bounds   []float64 `json:"bounds,omitempty"`
}

// mapCommand runs a Radiant menu entry on what the ids name, or lists a menu
// when menu is given instead, "" the menu bar.
func mapCommand(ctx context.Context, command string, ids []string, menu *string, pid int) (*mapCommandOut, error) {
	c, err := radiant.Connect(ctx, radiantMachine, pid)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	snap, err := level.Read(ctx, c)
	if err != nil {
		return nil, err
	}
	out := &mapCommandOut{Map: baseName(snap.State.Map)}
	command = plainMenuPath(command)
	if command == "" && menu != nil {
		path := plainMenuPath(*menu)
		entries, err := level.Menu(ctx, c, path)
		if err != nil {
			return nil, err
		}
		out.Menu = path
		for _, e := range entries {
			s := e.Label
			if e.Submenu {
				s += " >"
			}
			if !e.Enabled {
				s += " (disabled)"
			}
			out.Entries = append(out.Entries, s)
		}
		return out, nil
	}
	ran, err := level.Command(ctx, c, snap, ids, command)
	if err != nil {
		return nil, err
	}
	out.Command, out.Undo = strings.Trim(strings.TrimSpace(command), "/"), ran.Undo
	for _, g := range ran.Created {
		out.Created = append(out.Created, describeMade(ran.After, g))
	}
	for _, g := range ran.Deleted {
		out.Deleted = append(out.Deleted, snap.IDs.Short(g))
	}
	if !sameSet(ran.Selected, ran.Created) { // most commands leave what they made selected: said once
		for _, g := range ran.Selected {
			out.Selected = append(out.Selected, ran.After.IDs.Short(g))
		}
	}
	return out, nil
}

// plainMenuPath is a menu path as Radiant names it, its labels from the menu
// bar down joined by /. It reads a path written as a menu's listing prints
// its entries too, " >" after or between labels and " (disabled)" after one,
// so an entry pasted back from the listing names it.
func plainMenuPath(path string) string {
	var labels []string
	for _, l := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '>' }) {
		if l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), "(disabled)")); l != "" {
			labels = append(labels, l)
		}
	}
	return strings.Join(labels, "/")
}

// sameSet tells whether two GUID lists hold the same GUIDs, in any case.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	in := map[string]bool{}
	for _, g := range b {
		in[strings.ToUpper(g)] = true
	}
	for _, g := range a {
		if !in[strings.ToUpper(g)] {
			return false
		}
	}
	return true
}

// describeMade is an entity or brush of the map read back, by GUID.
func describeMade(after *level.Snapshot, guid string) madeThing {
	m := madeThing{ID: after.IDs.Short(guid)}
	for _, e := range after.Map.Entities {
		if strings.EqualFold(e.Guid(), guid) {
			m.Class = e.Class()
			if b := geom.EntityBox(e); b.Ok {
				m.Bounds = append(vec(b.Min), vec(b.Max)...)
			}
			return m
		}
		for _, b := range e.Brushes() {
			if strings.EqualFold(b.Guid(), guid) && b.Kind() != mapfile.KindEmpty {
				m.Material = level.Material(b)
				if box := geom.BrushBox(b); box.Ok {
					m.Bounds = append(vec(box.Min), vec(box.Max)...)
				}
				return m
			}
		}
	}
	return m
}
