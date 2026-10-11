package cli

import (
	"fmt"

	"github.com/McReaper/t7_companion/internal/zone"
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
	u, err := zone.UploadSize(dir)
	if err != nil {
		return nil
	}
	return toUploadOut(u)
}

// toUploadOut advises a fresh link once the dead data is a share of the .xpak bytes.
func toUploadOut(u zone.Upload) *uploadOut {
	out := &uploadOut{Bytes: u.Bytes, XpakDead: u.XpakDead}
	if u.XpakDead > 0 && float64(u.XpakDead) >= deadWorthShare*float64(u.Xpak) {
		out.Advice = fmt.Sprintf("at least %s of the .xpak is data earlier links replaced: the linker only appends to it, "+
			"and it uploads with zone/. Link the build you publish with build fresh_xpak (CLI --fresh-xpak)", megabytes(u.XpakDead))
	}
	return out
}

func megabytes(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/1e6) }
