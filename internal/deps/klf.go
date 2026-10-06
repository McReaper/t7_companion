package deps

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/asset"
)

// A lens flare (klf) is packed under the uuid of its file in lensflares/ (the
// map's folder first, then share/raw), and pulls in the images its grimeName
// and textureName keys name.

// klfChildren adds the images of the lens flare uuid.
func (g *Graph) klfChildren(uuid string, out *idSet) {
	g.klfOnce.Do(g.loadKlfs)
	for _, img := range g.klfs[strings.ToLower(uuid)] {
		out.add(asset.ID{Type: "image", Name: img})
	}
}

// loadKlfs indexes every lens flare file by its uuid.
func (g *Graph) loadKlfs() {
	g.klfs = map[string][]string{}
	dirs := []string{filepath.Join(g.w.Root, "share", "raw", "lensflares")}
	if g.mapDir != "" {
		dirs = append([]string{filepath.Join(g.mapDir, "lensflares")}, dirs...)
	}
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.klf"))
		for _, f := range files {
			uuid, images := readKlf(f)
			if k := strings.ToLower(uuid); k != "" && g.klfs[k] == nil { // the map's own wins
				g.klfs[k] = images
			}
		}
	}
}

// readKlf reads a .klf's `key "value"` lines: its uuid and the images it names.
func readKlf(file string) (string, []string) {
	f, err := os.Open(file)
	if err != nil {
		return "", nil
	}
	defer f.Close()
	var uuid string
	var images []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch {
		case !ok || v == "":
		case key == "uuid":
			uuid = v
		case key == "grimeName" || key == "textureName":
			images = append(images, v)
		}
	}
	return uuid, images
}
