package build

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/McReaper/t7_companion/internal/zone"
)

// maxMissing caps the assets a link's summary lists as missing.
const maxMissing = 10

// missingAfter lists what the linker looked for during the link that started
// at start and found in no GDT: it links on without them and logs nothing,
// so a material shows in game without that texture. Nothing when the zone's
// .deps wasn't written by this link or can't be read.
func missingAfter(root, dir, zoneName string, start time.Time) []string {
	deps := filepath.Join(dir, zoneName+".deps")
	if fileMTime(deps).Before(start.Add(-2 * time.Second)) {
		return nil
	}
	m, err := zone.MissingLookups(root, dir, zoneName)
	if err != nil {
		return nil
	}
	var out []string
	for i, x := range m {
		if i == maxMissing {
			out = append(out, fmt.Sprintf("…and %d more", len(m)-maxMissing))
			break
		}
		out = append(out, x.String())
	}
	return out
}
