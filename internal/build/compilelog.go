package build

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// cod2map exits 0 on what it logs as a problem, so the compile's verdict is
// its output and the .errlog it writes beside the map: an ERROR line fails
// the stage (a prefab it can't find is left out of the map); a WARNING line
// is listed, and each problem the .errlog places, by entity, brush and point.
// Its note that no nav_volume gave a nav volume is left out: that volume is
// for flying AI only.

const maxPlaced = 20 // problems the .errlog places, listed at most

var (
	// errlogLine is one problem of a .errlog: the map's path, entity and brush,
	// the point, a direction, the message.
	errlogLine = regexp.MustCompile(`^"[^"]*"\s+(-?\d+)\s+(-?\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+\S+\s+\S+\s+\S+\s+"(.*)"\s*$`)
	// timesSeen ends a WARNING line that sums up problems the .errlog places.
	timesSeen = regexp.MustCompile(`\s*\(x\d+\)$`)
)

// compileVerdict fails or annotates a compile that exited 0 from what
// cod2map logged, and the .errlog it wrote since start.
func compileVerdict(sr *StageResult, out, errlog string, start time.Time) {
	placed, messages, _ := readErrlog(errlog, start)
	for _, raw := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		l := stripColor(strings.TrimSpace(raw))
		switch {
		case strings.HasPrefix(l, "ERROR:"):
			sr.OK = false
			sr.Errors = append(sr.Errors, l)
		case !strings.HasPrefix(l, "WARNING:"),
			strings.Contains(l, "NavVolume generation is skipped"),
			strings.Contains(l, "PROBLEMS FOUND"),
			messages[strings.TrimSpace(timesSeen.ReplaceAllString(strings.TrimPrefix(l, "WARNING:"), ""))]:
		default:
			sr.Warnings = append(sr.Warnings, l)
		}
	}
	sr.Warnings = append(sr.Warnings, placed...)
}

// readErrlog reads the problems a .errlog written since start places, as
// warnings, maxPlaced at most, the messages they carry, and how many it places.
func readErrlog(path string, start time.Time) ([]string, map[string]bool, int) {
	messages := map[string]bool{}
	if !fileMTime(path).After(start.Add(-2 * time.Second)) {
		return nil, messages, 0
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, messages, 0
	}
	var out []string
	n := 0
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
		m := errlogLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		messages[strings.TrimSpace(m[6])] = true
		if n++; n <= maxPlaced {
			out = append(out, fmt.Sprintf("entity %s brush %s at %s %s %s: %s", m[1], m[2], m[3], m[4], m[5], m[6]))
		}
	}
	if n > maxPlaced {
		out = append(out, fmt.Sprintf("and %d more in %s", n-maxPlaced, path))
	}
	return out, messages, n
}
