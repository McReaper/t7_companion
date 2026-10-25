package radianttest

import (
	"strings"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// selectCmd adds the brushes the GUIDs name to the selection (an entity's
// GUID names its brushes), as the plugin's select does, and answers the
// selection; a GUID the map lacks is reported missing.
func selectCmd(f *Fake, args string, _ []byte) Reply {
	want := strings.Fields(args)
	if len(want) == 0 {
		return Err("bad-request", "select <guid>…")
	}
	m := f.parsed()
	var missing []string
	for _, g := range want {
		found := false
		for _, e := range m.Entities {
			brushes := brushesOf(e)
			if strings.EqualFold(e.Guid(), g) {
				f.Selection, found = append(f.Selection, brushes...), true
			}
			for _, b := range brushes {
				if strings.EqualFold(b, g) {
					f.Selection, found = append(f.Selection, b), true
				}
			}
		}
		if !found {
			missing = append(missing, g)
		}
	}
	r := selection(f, "", nil)
	if len(missing) > 0 {
		r.Summary = "missing=" + strings.Join(missing, ",")
	}
	return r
}

// PointBrush is the GUID of the brush through which Radiant selects a point
// entity: one the map's text doesn't hold.
func PointBrush(entity string) string {
	h := strings.Trim(entity, "{}")
	return "{" + h[:min(8, len(h))] + "-FFFF-FFFF-FFFF-FFFFFFFFFFFF}"
}

// brushesOf are the GUIDs Radiant selects an entity through: its brushes',
// or a point entity's PointBrush.
func brushesOf(e *mapfile.Entity) []string {
	var out []string
	for _, b := range e.Brushes() {
		out = append(out, b.Guid())
	}
	if len(out) == 0 {
		out = []string{PointBrush(e.Guid())}
	}
	return out
}

func deselect(f *Fake, _ string, _ []byte) Reply {
	f.Selection = nil
	return OK("", nil)
}

// menus answers the fake's Menus, by path; the Edit menu's undo entry is
// UndoLabel's.
func menus(f *Fake, args string, _ []byte) Reply {
	path := strings.TrimSpace(args)
	if path == "Edit" {
		undo := "Undo\t0\t0\n"
		if f.UndoLabel != "" {
			undo = "Undo " + f.UndoLabel + "\t1\t0\n"
		}
		return OK("", []byte(undo+"Redo\t0\t0\n---\t0\t0\n"+f.Menus[path]))
	}
	return OK("", []byte(f.Menus[path]))
}
