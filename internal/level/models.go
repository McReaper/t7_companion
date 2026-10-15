package level

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScanModels counts, by model, the entities of the install's maps that place
// it (their "model" key: misc_model, script_model…), prefabs included. A map
// or folder that can't be read is left out.
func ScanModels(root string) map[string]int {
	counts := map[string]int{}
	_ = filepath.WalkDir(filepath.Join(root, "map_source"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".map") {
			scanModels(p, counts)
		}
		return nil
	})
	return counts
}

// scanModels counts a map's "model" keys by model.
func scanModels(path string, counts map[string]int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), `"model" "`)
		if !ok {
			continue
		}
		if name, ok := strings.CutSuffix(rest, `"`); ok && name != "" && !strings.Contains(name, `"`) {
			counts[name]++
		}
	}
}
