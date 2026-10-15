package level

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/format/mapfile"
)

// toolMaterial tells a tool material by its name, as the stock maps name
// them: what draws nothing in game.
func toolMaterial(m string) bool {
	for _, t := range []string{"caulk", "clip", "trigger", "nodraw", "volume", "umbra", "mantle", "traverse", "portal", "hint", "lightgrid", "ai_", "sound_"} {
		if strings.Contains(m, t) {
			return true
		}
	}
	return false
}

// Treyarch caulks what can't be seen: on the stock prefabs' geometry, the
// faces CheckTextures calls hidden should be caulk or another tool material
// far more often than the faces it doesn't. Opt-in on TA_TOOLS_PATH.
func TestHiddenFacesAreTheOnesTreyarchCaulked(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if root == "" {
		t.Skip("TA_TOOLS_PATH unset")
	}
	info := func(m string) MaterialInfo {
		see := !strings.Contains(m, "glass") && !strings.Contains(m, "window") && !strings.Contains(m, "decal")
		return MaterialInfo{Known: true, Drawn: true, Opaque: m == "caulk" || m == "sky" || (see && !toolMaterial(m))}
	}
	var hidden, hiddenTool, faces, tools, files int
	for _, dir := range []string{"zm/zm_giant", "mp/mp_combine", "mp/mp_sector"} {
		_ = filepath.WalkDir(filepath.Join(root, "map_source", "_prefabs", filepath.FromSlash(dir)), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".map") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			m, err := mapfile.Parse(b)
			if err != nil {
				return nil
			}
			files++
			snap := &Snapshot{Map: m, IDs: NewIDs(GUIDs(m))}
			found, _ := CheckTextures(snap, nil, info)
			for _, f := range found {
				if f.Kind == "hidden" {
					hidden++
					if toolMaterial(f.Material) {
						hiddenTool++
					}
				}
			}
			for _, e := range m.Entities {
				if e.Class() != "worldspawn" {
					continue
				}
				for _, br := range e.Brushes() {
					fs, _ := br.Faces()
					for _, fc := range fs {
						faces++
						if toolMaterial(fc.Material) {
							tools++
						}
					}
				}
			}
			return nil
		})
	}
	share := float64(hiddenTool) / float64(max(hidden, 1))
	base := float64(tools) / float64(max(faces, 1))
	t.Logf("%d prefabs, %d world faces, %.0f%% of them tool materials; %d called hidden, %.0f%% of those tool materials", files, faces, base*100, hidden, share*100)
	if hidden == 0 || share < 0.8 || share < base+0.2 {
		t.Errorf("hidden faces are tool materials %.0f%% of the time, faces at large %.0f%%: the check doesn't find what Treyarch caulked", share*100, base*100)
	}
}
