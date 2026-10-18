package level

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

const (
	floorGUID = "{5EC8DE49-C58E-11F1-B542-00155DD185EA}"
	sunBrush  = "{6E73936C-C58E-11F1-B542-00155DD185EA}"
)

// hollow is a fake CSG Hollow: the selected floor gives way to two brushes,
// left selected, under the undo entry "hollow".
func hollow(f *radianttest.Fake, args string, _ []byte) radianttest.Reply {
	if args != "Selection/CSG/CSG Hollow" {
		return radianttest.Err("missing", "no menu entry "+args)
	}
	if !slices.Contains(f.Selection, floorGUID) {
		return radianttest.Err("disabled", args+" is disabled")
	}
	f.Text = strings.Replace(f.Text, floorGUID, "{0B000001-0000-0000-0000-000000000000}", 1)
	two := strings.Replace(f.Text[strings.Index(f.Text, "// brush 0"):strings.Index(f.Text, "// brush 1")], "0B000001", "0B000002", 1)
	f.Text = strings.Replace(f.Text, "// brush 1", two+"// brush 1", 1)
	f.Selection = []string{"{0B000001-0000-0000-0000-000000000000}", "{0B000002-0000-0000-0000-000000000000}"}
	f.UndoLabel, f.Modified = "hollow", true
	return radianttest.OK("", nil)
}

func TestACommandRunsOnWhatTheIDsNameAndGivesTheSelectionBack(t *testing.T) {
	f, c, _ := viewing(t, 301)
	f.Update(func(f *radianttest.Fake) { f.Selection = []string{sunBrush} })
	f.Handle("command", hollow)
	snap, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	ran, err := Command(context.Background(), c, snap, []string{"5ec8de49"}, " /Selection/CSG/CSG Hollow/ ")
	if err != nil {
		t.Fatal(err)
	}
	if ran.Undo != "hollow" || len(ran.Created) != 2 || len(ran.Deleted) != 1 || ran.Deleted[0] != floorGUID || len(ran.Selected) != 2 {
		t.Errorf("ran = %+v", ran)
	}
	if !slices.Equal(f.Selection, []string{sunBrush}) {
		t.Errorf("the user's selection is %q", f.Selection)
	}
	want := []string{"deselect", "select " + floorGUID, "command Selection/CSG/CSG Hollow", "selection", "deselect", "select " + sunBrush}
	got := f.Requests()
	if i := slices.Index(got, "deselect"); i < 0 || !slices.Equal(got[i:i+len(want)], want) {
		t.Errorf("requests %q, want %q in a row", got, want)
	}
	// the same command again: the same label, but the map changed
	f.Update(func(f *radianttest.Fake) {
		f.Text = strings.Replace(f.Text, "{0B000001-0000-0000-0000-000000000000}", floorGUID, 1)
	})
	snap, _ = Read(context.Background(), c)
	if ran, err := Command(context.Background(), c, snap, []string{"5ec8de49"}, "Selection/CSG/CSG Hollow"); err != nil || ran.Undo != "hollow" {
		t.Errorf("again: %+v, %v", ran, err)
	}
}

func TestACommandThatChangesNothingMadeNoUndoStep(t *testing.T) {
	f, c, snap := viewing(t, 302)
	f.Update(func(f *radianttest.Fake) { f.UndoLabel = "something earlier" })
	f.Handle("command", func(*radianttest.Fake, string, []byte) radianttest.Reply { return radianttest.OK("", nil) })
	ran, err := Command(context.Background(), c, snap, nil, "View/Center")
	if err != nil || ran.Undo != "" || len(ran.Created)+len(ran.Deleted) != 0 {
		t.Errorf("ran = %+v, %v", ran, err)
	}
	if slices.ContainsFunc(f.Requests(), func(r string) bool { return strings.HasPrefix(r, "select ") }) {
		t.Errorf("nothing to select, and no user selection to give back: %q", f.Requests())
	}
	// a save clears the modified flag and makes no undo step; an edit that sets it does
	f.Update(func(f *radianttest.Fake) { f.Modified = true })
	f.Handle("command", func(f *radianttest.Fake, _ string, _ []byte) radianttest.Reply {
		f.Modified = false
		return radianttest.OK("", nil)
	})
	snap, _ = Read(context.Background(), c)
	if ran, err := Command(context.Background(), c, snap, nil, "File/Save"); err != nil || ran.Undo != "" {
		t.Errorf("save: %+v, %v", ran, err)
	}
	f.Handle("command", func(f *radianttest.Fake, _ string, _ []byte) radianttest.Reply {
		f.Modified = true
		return radianttest.OK("", nil)
	})
	snap, _ = Read(context.Background(), c)
	if ran, err := Command(context.Background(), c, snap, nil, "Textures/Fit Texture"); err != nil || ran.Undo != "something earlier" {
		t.Errorf("an edit that adds or removes nothing: %+v, %v", ran, err)
	}
}

func TestACommandThatFailsStillGivesTheSelectionBack(t *testing.T) {
	f, c, _ := viewing(t, 303)
	f.Update(func(f *radianttest.Fake) { f.Selection = []string{sunBrush} })
	f.Handle("command", hollow)
	snap, _ := Read(context.Background(), c)
	for path, want := range map[string]string{
		"Selection/CSG/CSG Hollow": "is disabled", // nothing selected that it works on
		"Nope/Nothing":             "no menu entry",
		"  ":                       "no command",
	} {
		_, err := Command(context.Background(), c, snap, []string{"6e73936c"}, path)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v", path, err)
		}
		if !slices.Equal(f.Selection, []string{sunBrush}) {
			t.Errorf("%q: the user's selection is %q", path, f.Selection)
		}
	}
	if _, err := Command(context.Background(), c, snap, []string{"zzz"}, "Selection/CSG/CSG Hollow"); err == nil {
		t.Error("an id that is no id")
	}
}

func TestMenuListsAMenusEntries(t *testing.T) {
	f, c, _ := viewing(t, 304)
	f.Update(func(f *radianttest.Fake) {
		f.Menus = map[string]string{"Selection/CSG": "CSG Hollow\t1\t0\n---\t0\t0\nCSG &Merge\t0\t0\nMore\t1\t1\n"}
	})
	got, err := Menu(context.Background(), c, "/Selection/CSG/")
	want := []MenuEntry{{"CSG Hollow", true, false}, {"CSG Merge", false, false}, {"More", true, true}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("menu = %+v, %v", got, err)
	}
	if _, err := Menu(context.Background(), c, "Nope"); err == nil || !strings.Contains(err.Error(), `no menu "Nope"`) {
		t.Errorf("an unknown menu: %v", err)
	}
}

func TestDiff(t *testing.T) {
	c, d := diff([]string{"{A}", "{B}", "{B}"}, []string{"{b}", "{C}", "{C}", "x"})
	if !slices.Equal(c, []string{"{C}"}) || !slices.Equal(d, []string{"{A}"}) {
		t.Errorf("created %q, deleted %q", c, d)
	}
}
