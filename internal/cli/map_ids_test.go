package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// runMap runs `t7kb map <args>` on the fake Radiant and gives its output.
func runMap(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newMapCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// The help lists ids among the tool's arguments, and an agent writes them as
// --ids: map find, view and command take them so, comma- or space-separated,
// beside or in place of the ones after the command.
func TestMapCommandsTakeIDsAsAFlag(t *testing.T) {
	f := testRadiant(t)
	out, err := runMap(t, "find", "--ids", "76780cea, f2d51a52", "5ec8de46")
	var found mapFindOut
	if err != nil || json.Unmarshal([]byte(out), &found) != nil {
		t.Fatalf("find: %v\n%s", err, out)
	}
	var ids []string
	for _, e := range found.Found {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []string{"5ec8de46", "76780cea", "f2d51a52"}) {
		t.Errorf("find --ids and in place: %q", ids)
	}
	png := filepath.Join(t.TempDir(), "v.png")
	if out, err := runMap(t, "view", "--view", "top", "--ids", "6e73936d", "--out", png); err != nil || !strings.Contains(out, `"6e73936d"`) {
		t.Errorf("view --ids: %v\n%s", err, out)
	}
	if _, err := os.Stat(png); err != nil {
		t.Errorf("view --ids wrote no image: %v", err)
	}
	f.Handle("command", splitFloor)
	if out, err := runMap(t, "command", "Selection/CSG/CSG Subtract", "--ids", "5ec8de49"); err != nil || !strings.Contains(out, `"undo": "subtract"`) {
		t.Errorf("command --ids: %v\n%s", err, out)
	}
	if _, err := runMap(t, "command", "--menu=Selection", "--ids", "5ec8de49"); err == nil || !strings.Contains(err.Error(), `nothing after it (got "5ec8de49")`) {
		t.Errorf("--menu with --ids: %v", err)
	}
}
