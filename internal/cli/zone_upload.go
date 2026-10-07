package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/xpak"
)

// uploadOut is what publishing sends: the Launcher uploads the whole zone/
// folder the link writes, .xpak data no entry reads included.
type uploadOut struct {
	Bytes    int64  `json:"bytes"`
	XpakDead int64  `json:"xpak_dead_at_least,omitempty"`
	Advice   string `json:"advice,omitempty"`
}

// deadWorthShare: below this share of the .xpak bytes, rewriting them all
// isn't worth suggesting.
const deadWorthShare = 0.01

// upload weighs a zone/ folder and the .xpak data in it that earlier links
// replaced. It is nil when the folder can't be read.
func upload(dir string) *uploadOut {
	out := &uploadOut{}
	var xpaks int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out.Bytes += info.Size()
		if strings.EqualFold(filepath.Ext(p), ".xpak") && filepath.Dir(p) == dir {
			xpaks += info.Size()
			if x, err := xpak.Open(p); err == nil {
				out.XpakDead += x.Dead()
			}
		}
		return nil
	})
	if err != nil {
		return nil
	}
	if out.XpakDead > 0 && float64(out.XpakDead) >= deadWorthShare*float64(xpaks) {
		out.Advice = fmt.Sprintf("at least %s of the .xpak is data earlier links replaced: the linker only appends to it, "+
			"and it uploads with zone/. Link the build you publish with build fresh_xpak (CLI --fresh-xpak)", megabytes(out.XpakDead))
	}
	return out
}
