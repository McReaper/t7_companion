package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A link only ever appends to a .xpak: the data it replaces stays in the file,
// read by nothing, and uploads with the zone/ folder the Launcher publishes.
// With fresh_xpak the link stage sets the target's .xpak files aside so the
// linker writes them from scratch, then keeps the ones it wrote.

const asideSuffix = ".t7kb-prev"

// xpakAside is a zone folder's .xpak files, moved aside for one link.
type xpakAside struct {
	dir   string
	sizes map[string]int64 // file name -> its size before the link
}

// setXpaksAside moves every .xpak in dir aside, after putting back any a link
// that never finished left aside and didn't replace (one it did replace is
// overwritten by setting the new file aside).
func setXpaksAside(dir string) (*xpakAside, error) {
	left, _ := filepath.Glob(filepath.Join(dir, "*.xpak"+asideSuffix))
	for _, p := range left {
		base := strings.TrimSuffix(p, asideSuffix)
		if _, err := os.Stat(base); err == nil {
			continue
		}
		if err := os.Rename(p, base); err != nil {
			return nil, err
		}
	}
	a := &xpakAside{dir: dir, sizes: map[string]int64{}}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.xpak"))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err == nil {
			err = os.Rename(p, p+asideSuffix)
		}
		if err != nil {
			a.settle(false)
			return nil, fmt.Errorf("can't set %s aside (is the game running?): %w", filepath.Base(p), err)
		}
		a.sizes[filepath.Base(p)] = st.Size()
	}
	return a, nil
}

// settle ends the link. After a successful one it drops each set-aside file
// the link wrote anew and puts the others back (a language this link didn't
// build); after a failed one it puts every one back, to go with the fastfiles
// the link didn't replace. It says what the rewrite saved.
func (a *xpakAside) settle(linked bool) string {
	var before, after int64
	for name, size := range a.sizes {
		p := filepath.Join(a.dir, name)
		if st, err := os.Stat(p); linked && err == nil {
			_ = os.Remove(p + asideSuffix)
			before, after = before+size, after+st.Size()
			continue
		}
		_ = os.Rename(p+asideSuffix, p)
	}
	if before == 0 {
		return ""
	}
	return fmt.Sprintf(".xpak written from scratch: %s -> %s", megabytes(before), megabytes(after))
}

func megabytes(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/1e6) }

// zoneDir is the folder the link writes the target's fastfiles to.
func (p *buildPlan) zoneDir() string {
	kind := "usermaps"
	if p.o.isMod {
		kind = "mods"
	}
	return filepath.Join(p.game, kind, p.name, "zone")
}

func joinNotes(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + "; " + b
}
