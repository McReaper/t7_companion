package level

import (
	"context"
	"fmt"
	"strings"

	"github.com/McReaper/t7_companion/internal/radiant"
)

// Ran is a Radiant command's outcome, read from the map before and after.
type Ran struct {
	Undo     string   // the undo menu's entry it made, "" when it made none
	Created  []string // GUIDs of the entities and brushes it made
	Deleted  []string // and of those it took away
	Selected []string // the brushes it left selected (its result, for most), before the user's selection came back
	After    *Snapshot
}

// Command runs one of Radiant's menu entries (Selection/CSG/CSG Hollow) on
// the entities and brushes the ids name, selected alone for it, then gives
// the user's selection back.
func Command(ctx context.Context, c *radiant.Client, snap *Snapshot, ids []string, path string) (*Ran, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil, fmt.Errorf("no command: a menu path, as menus lists it (Selection/CSG/CSG Hollow)")
	}
	var guids []string
	for _, id := range ids {
		g, err := snap.IDs.Resolve(id)
		if err != nil {
			return nil, err
		}
		guids = append(guids, g)
	}
	before := undoEntry(ctx, c)
	if err := call(ctx, c, "deselect"); err != nil {
		return nil, err
	}
	run := func() (*Ran, error) {
		if len(guids) > 0 {
			if err := call(ctx, c, "select "+strings.Join(guids, " ")); err != nil {
				return nil, err
			}
		}
		if err := call(ctx, c, "command "+path); err != nil {
			return nil, err
		}
		r, err := c.Call(ctx, "selection", nil)
		if err != nil {
			return nil, err
		}
		return &Ran{Selected: ParseSelection(string(r.Body)).GUIDs}, nil
	}
	ran, err := run()
	restoreSelection(ctx, c, snap.Sel.GUIDs) // whatever happened
	if err != nil {
		return nil, err
	}
	if ran.After, err = Read(ctx, c); err != nil {
		return nil, fmt.Errorf("%s ran, but the map could not be read back: %w", path, err)
	}
	ran.Created, ran.Deleted = diff(GUIDs(snap.Map), GUIDs(ran.After.Map))
	// two commands alike in a row leave the same label: the map's change tells it too (a
	// save clears the modified flag and makes no undo step)
	if u := undoEntry(ctx, c); u != before || len(ran.Created)+len(ran.Deleted) > 0 || (!snap.State.Modified && ran.After.State.Modified) {
		ran.Undo = u
	}
	return ran, nil
}

func call(ctx context.Context, c *radiant.Client, line string) error {
	_, err := c.Call(ctx, line, nil)
	return err
}

// restoreSelection selects again what the user had selected and still exists.
func restoreSelection(ctx context.Context, c *radiant.Client, guids []string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	if call(ctx, c, "deselect") == nil && len(guids) > 0 {
		_ = call(ctx, c, "select "+strings.Join(guids, " ")) // a GUID the command took away is only reported missing
	}
}

// undoEntry is the label of the undo menu's entry, "" when there is none or
// the menu can't be read.
func undoEntry(ctx context.Context, c *radiant.Client) string {
	r, err := c.Call(ctx, "menus Edit", nil)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(r.Body), "\n") {
		label, _, _ := strings.Cut(line, "\t")
		if rest, ok := strings.CutPrefix(label, "Undo "); ok {
			return rest
		}
	}
	return ""
}

// diff is what after has that before lacks, and the other way.
func diff(before, after []string) (created, deleted []string) {
	in := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, g := range list {
			m[hexOf(g)] = true
		}
		return m
	}
	b, a := in(before), in(after)
	for _, g := range after {
		if h := hexOf(g); h != "" && !b[h] {
			created, b[h] = append(created, g), true
		}
	}
	for _, g := range before {
		if h := hexOf(g); h != "" && !a[h] {
			deleted, a[h] = append(deleted, g), true
		}
	}
	return created, deleted
}

// Menu lists a menu's entries, as the bridge's menus gives them: one
// "label<TAB>enabled<TAB>has_submenu" line each, "---" a separator.
func Menu(ctx context.Context, c *radiant.Client, path string) ([]MenuEntry, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	r, err := c.Call(ctx, "menus "+path, nil)
	if err != nil {
		return nil, err
	}
	if out := parseMenu(r.Body); len(out) > 0 {
		return out, nil
	}
	return nil, noMenu(ctx, c, path)
}

func parseMenu(body []byte) []MenuEntry {
	var out []MenuEntry
	for _, line := range strings.Split(string(body), "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 3 || f[0] == "---" {
			continue
		}
		out = append(out, MenuEntry{Label: strings.ReplaceAll(f[0], "&", ""), Enabled: f[1] == "1", Submenu: f[2] == "1"})
	}
	return out
}

// noMenu says why a path lists nothing: its last label is an entry of the
// menu above it, one that runs, or no menu is named so.
func noMenu(ctx context.Context, c *radiant.Client, path string) error {
	parent, last := "", path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		parent, last = path[:i], path[i+1:]
	}
	if r, err := c.Call(ctx, "menus "+parent, nil); err == nil {
		for _, e := range parseMenu(r.Body) {
			if e.Label == last && !e.Submenu {
				return fmt.Errorf("%q is an entry, not a menu: map command %q runs it", path, path)
			}
		}
	}
	return fmt.Errorf("no menu %q: a menu is named by its labels from the menu bar down, joined by / (Selection/CSG)", path)
}

// MenuEntry is one entry of a Radiant menu.
type MenuEntry struct {
	Label   string
	Enabled bool
	Submenu bool
}
