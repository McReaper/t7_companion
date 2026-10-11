package zone

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/McReaper/t7_companion/internal/format/xpak"
)

// Upload weighs what publishing sends: the Launcher uploads the whole zone/
// folder the link writes, .xpak data no entry reads included.
type Upload struct {
	Bytes    int64 // the folder
	Xpak     int64 // its .xpak files
	XpakDead int64 // at least this much of them is data earlier links replaced
}

// UploadSize weighs the zone/ folder dir (Report.OutDir).
func UploadSize(dir string) (Upload, error) {
	var u Upload
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		u.Bytes += info.Size()
		if strings.EqualFold(filepath.Ext(p), ".xpak") && filepath.Dir(p) == dir {
			u.Xpak += info.Size()
			if x, err := xpak.Open(p); err == nil {
				u.XpakDead += x.Dead()
			}
		}
		return nil
	})
	return u, err
}
