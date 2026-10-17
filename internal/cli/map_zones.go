package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/McReaper/t7_companion/internal/level"
)

// scriptOut is what the zones an edit made need of the map's script: the
// add_adjacent_zone calls it lacks, for the function the map's main names
// level.zone_manager_init_func.
type scriptOut struct {
	File     string   `json:"file"`
	Function string   `json:"function,omitempty"`
	Add      []string `json:"add,omitempty"`
	Present  int      `json:"present,omitempty"` // calls the script already makes
	Note     string   `json:"note,omitempty"`
}

var zoneInitFunc = regexp.MustCompile(`level\.zone_manager_init_func\s*=\s*&\s*([A-Za-z_]\w*)\s*;`)

// zoneScript reads the map's script, usermaps/<map>/scripts/zm/<map>.gsc,
// for the zone links an edit made: which calls are there, which to add.
func zoneScript(mapPath string, links []level.Link) *scriptOut {
	name := strings.TrimSuffix(baseName(mapPath), filepath.Ext(mapPath))
	rel := filepath.ToSlash(filepath.Join("usermaps", name, "scripts", "zm", name+".gsc"))
	out := &scriptOut{File: rel}
	b, err := os.ReadFile(filepath.Join(installOf(mapPath), filepath.FromSlash(rel)))
	if err != nil {
		out.Note = "no script at " + rel + ": these calls go in the function the map's main sets as level.zone_manager_init_func"
	}
	src := string(b)
	if m := zoneInitFunc.FindStringSubmatch(src); m != nil {
		out.Function = m[1]
	} else if err == nil {
		out.Note = "the script sets no level.zone_manager_init_func: set one in main, before zm_zonemgr::manage_zones, and make these calls in it"
	}
	for _, l := range links {
		if hasLink(src, l) {
			out.Present++
			continue
		}
		oneWay := ""
		if l.OneWay {
			oneWay = ", true"
		}
		out.Add = append(out.Add, fmt.Sprintf(`zm_zonemgr::add_adjacent_zone( "%s", "%s", "%s"%s );`, l.A, l.B, l.Flag, oneWay))
	}
	return out
}

// hasLink tells whether a script already links two zones under a flag, in
// either order for a two-way link.
func hasLink(src string, l level.Link) bool {
	call := func(a, b string) *regexp.Regexp {
		return regexp.MustCompile(`add_adjacent_zone\s*\(\s*"` + regexp.QuoteMeta(a) + `"\s*,\s*"` + regexp.QuoteMeta(b) + `"\s*,\s*"` + regexp.QuoteMeta(l.Flag) + `"`)
	}
	return call(l.A, l.B).MatchString(src) || !l.OneWay && call(l.B, l.A).MatchString(src)
}
