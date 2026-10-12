package mapfile

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every .map under map_source (TA_TOOLS_PATH) round-trips byte for byte, and
// every brush and patch in it parses.
func TestInstallMapsRoundTrip(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" {
		t.Skip("set TA_TOOLS_PATH to read the install's .map files")
	}
	var files, entities, faces, curves, meshes int
	err := filepath.WalkDir(filepath.Join(root, "map_source"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".map") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		m, err := Parse(b)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		if !bytes.Equal(m.Bytes(), b) {
			t.Errorf("%s: Bytes differs from the file", path)
		}
		if m.Version() != 4 {
			t.Errorf("%s: version %d, want 4", path, m.Version())
		}
		for _, e := range m.Entities {
			entities++
			for _, br := range e.Brushes() {
				n, c, ms, err := checkBrush(br)
				if err != nil {
					t.Errorf("%s: %v", path, err)
				}
				faces, curves, meshes = faces+n, curves+c, meshes+ms
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Skip("no .map under map_source")
	}
	t.Logf("%d files, %d entities, %d faces, %d curves, %d meshes", files, entities, faces, curves, meshes)
}

func checkBrush(b *Brush) (faces, curves, meshes int, err error) {
	switch b.Kind() {
	case KindFaces:
		fs, err := b.Faces()
		return len(fs), 0, 0, err
	case KindCurve, KindMesh:
		p, err := b.Patch()
		if err != nil {
			return 0, 0, 0, err
		}
		if p.Kind == KindCurve {
			return 0, 1, 0, nil
		}
		return 0, 0, 1, nil
	}
	return 0, 0, 0, nil
}
