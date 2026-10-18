package geom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// The brushes of every .map in the install have boxes: the face winding
// BrushBox reads is the one Radiant writes everywhere. Opt-in on
// TA_TOOLS_PATH.
func TestEveryInstallBrushHasABox(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" {
		t.Skip("TA_TOOLS_PATH not set")
	}
	var maps, brushes, patches int
	var missing []string
	err := filepath.WalkDir(filepath.Join(root, "map_source"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".map") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		m, err := mapfile.Parse(b)
		if err != nil {
			return nil // the reader's own oracle covers parsing
		}
		maps++
		for _, e := range m.Entities {
			for _, br := range e.Brushes() {
				switch br.Kind() {
				case mapfile.KindFaces:
					brushes++
				case mapfile.KindCurve, mapfile.KindMesh:
					patches++
				default:
					continue
				}
				if !BrushBox(br).Ok {
					missing = append(missing, filepath.Base(p)+" "+br.Guid())
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d maps, %d brushes, %d patches", maps, brushes, patches)
	// A brush whose planes enclose nothing has no box, in Radiant too
	// (TestBrushBoxesMatchRadiants): zm_test has two. A wrong winding would
	// leave nearly every brush without one.
	t.Logf("%d without a box: %s", len(missing), strings.Join(missing[:min(10, len(missing))], "; "))
	if len(missing)*1000 > brushes+patches {
		t.Errorf("%d of %d brushes and patches without a box", len(missing), brushes+patches)
	}
}
