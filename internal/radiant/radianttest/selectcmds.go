package radianttest

import (
	"strings"
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
			if strings.EqualFold(e.Guid(), g) {
				for _, b := range e.Brushes() {
					f.Selection = append(f.Selection, b.Guid())
				}
				found = true
			}
			for _, b := range e.Brushes() {
				if strings.EqualFold(b.Guid(), g) {
					f.Selection, found = append(f.Selection, b.Guid()), true
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
