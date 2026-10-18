package level

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
	"github.com/McReaper/t7_companion/internal/level/geom"
)

// testMap is a small map Radiant wrote: a worldspawn with a floor brush and an
// arch curve, a volume_sun and three reflection probes.
func testMap(t *testing.T) *mapfile.Map {
	t.Helper()
	b, err := os.ReadFile("testdata/test_claude.map")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mapfile.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestShortIDsAreAGUIDsFirstGroupUnlessAnotherSharesIt(t *testing.T) {
	ids := NewIDs([]string{
		"{5EC8DE49-C58E-11F1-B542-00155DD185EA}",
		"{5EC8DE46-C58E-11F1-B542-00155DD185EA}", // shares 7 digits
		"{91B47D4C-B63F-412F-A5DD-AD23A602EB66}",
		"{91B47D4C-B640-412F-A5DD-AD23A602EB66}", // shares the first group and 2 more
		"{91B47D4C-B63F-412F-A5DD-AD23A602EB66}", // the same GUID again
		"", "not a guid",
	})
	for guid, want := range map[string]string{
		"{5EC8DE49-C58E-11F1-B542-00155DD185EA}": "5ec8de49",
		"{5EC8DE46-C58E-11F1-B542-00155DD185EA}": "5ec8de46",
		"{91B47D4C-B63F-412F-A5DD-AD23A602EB66}": "91b47d4cb63",
		"{91B47D4C-B640-412F-A5DD-AD23A602EB66}": "91b47d4cb64",
		"{00000000-0000-0000-0000-000000000000}": "00000000000000000000000000000000",
	} {
		if got := ids.Short(guid); got != want {
			t.Errorf("Short(%s) = %q, want %q", guid, got, want)
		}
	}
	if ids := NewIDs([]string{"{A}"}); ids.Short("{A}") != "a" {
		t.Errorf("a GUID shorter than an id is its whole self: %q", ids.Short("{A}"))
	}
}

func TestResolveTakesAnyStartOfAGUID(t *testing.T) {
	ids := NewIDs([]string{"{5EC8DE49-C58E-11F1-B542-00155DD185EA}", "{5EC8DE46-C58E-11F1-B542-00155DD185EA}"})
	for _, id := range []string{"5ec8de49", "5EC8DE49", "{5ec8de49-c58e", "5ec8de49c58e11f1b54200155dd185ea", " 5ec8de49 "} {
		if g, err := ids.Resolve(id); err != nil || g != "{5EC8DE49-C58E-11F1-B542-00155DD185EA}" {
			t.Errorf("Resolve(%q) = %q, %v", id, g, err)
		}
	}
	for id, want := range map[string]string{
		"5ec8de4":  "5ec8de4 names 2 entities or brushes (5ec8de46, 5ec8de49): give more of it",
		"5ec8de47": "no entity or brush 5ec8de47 in the map",
		"wall":     `"wall" is no id`,
		"":         `"" is no id`,
	} {
		if _, err := ids.Resolve(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Resolve(%q): err = %v, want %q", id, err, want)
		}
	}
}

func box(min, max mapfile.Vec3) geom.Box { return geom.Box{Min: min, Max: max, Ok: true} }

// face writes a face line through three points.
func face(p0, p1, p2 mapfile.Vec3) string {
	return fmt.Sprintf(" ( %s ) ( %s ) ( %s ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0", fmtVec(p0), fmtVec(p1), fmtVec(p2))
}

func fmtVec(v mapfile.Vec3) string {
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
	return f(v[0]) + " " + f(v[1]) + " " + f(v[2])
}

func brushOf(t *testing.T, faces ...string) *mapfile.Brush {
	t.Helper()
	text := "{\n\"classname\" \"worldspawn\"\n{\n guid \"{00000000-0000-0000-0000-000000000001}\"\n" + strings.Join(faces, "\n") + "\n}\n}\n"
	m, err := mapfile.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return m.Entities[0].Brushes()[0]
}
