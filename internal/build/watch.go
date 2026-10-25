package build

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/McReaper/t7_companion/internal/zone"
)

// The mod tools that write a build's records as they go: a record is read once
// its tool has quit.
const (
	compilerExe = "cod2map64.exe"
	linkerExe   = "linker_modtools.exe"
)

// maxSaid is how many of a list a line names; the rest are counted.
const maxSaid = 3

// RecordWatcher says each build the mod tools ran, whoever ran it, from the
// records their tools wrote: see the package doc.
type RecordWatcher struct {
	Root     string                // the mod-tools root
	Runs     func(exe string) bool // whether a process of that executable runs
	Out      func(string)          // gets each line
	Settle   time.Duration         // how long a build's records stay as they are before they are read: a mod links its zones one linker after the other
	start    time.Time
	seen     map[string]time.Time // each record's mtime, as last seen
	links    map[string]*records  // by target folder, a link not said yet
	compiles map[string]*records  // by map path without its extension, a compile not said yet
}

// records are the records one build wrote since the watch last said it.
type records struct {
	since   time.Time // the oldest one's mtime
	changed time.Time // when one last changed, by the watch's clock
}

// Tick looks at the records once, and says each build whose tool has quit and
// whose records have settled.
func (w *RecordWatcher) Tick() {
	now := time.Now()
	if w.seen == nil {
		w.start, w.seen, w.links, w.compiles = now, map[string]time.Time{}, map[string]*records{}, map[string]*records{}
	}
	for _, kind := range []string{"usermaps", "mods"} {
		for _, ext := range []string{"deps", "errorlog"} {
			paths, _ := filepath.Glob(filepath.Join(w.Root, kind, "*", "zone_source", "*", "assetinfo", "*."+ext))
			for _, p := range paths {
				w.note(w.links, p, filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(p)))), now)
			}
		}
	}
	for _, ext := range []string{"lin", "errlog"} {
		paths, _ := filepath.Glob(filepath.Join(w.Root, "map_source", "*", "*."+ext))
		for _, p := range paths {
			w.note(w.compiles, p, strings.TrimSuffix(p, filepath.Ext(p)), now)
		}
	}
	w.flush(w.links, linkerExe, now, w.sayLink)
	w.flush(w.compiles, compilerExe, now, w.sayCompile)
}

// note files a record written since the watch began under its build.
func (w *RecordWatcher) note(builds map[string]*records, p, key string, now time.Time) {
	m := fileMTime(p)
	if m.Before(w.start) || m.Equal(w.seen[p]) {
		return
	}
	w.seen[p] = m
	b := builds[key]
	if b == nil {
		b = &records{since: m}
		builds[key] = b
	}
	b.since, b.changed = minTime(b.since, m), now
}

// flush says the builds whose records have settled, unless their tool still
// runs.
func (w *RecordWatcher) flush(builds map[string]*records, exe string, now time.Time, say func(string, time.Time)) {
	if len(builds) == 0 || w.Runs != nil && w.Runs(exe) {
		return
	}
	keys := make([]string, 0, len(builds))
	for k := range builds {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if b := builds[k]; now.Sub(b.changed) >= w.Settle {
			say(k, b.since)
			delete(builds, k)
		}
	}
}

// sayLink says a link's verdict, from the errorlogs it wrote, and after a link
// that built, the assets it looked up and found in no GDT.
func (w *RecordWatcher) sayLink(target string, since time.Time) {
	name := w.rel(target)
	log, _ := readLinkLogs(filepath.Join(target, "zone_source"), since)
	where := make([]string, len(log.files))
	for i, f := range log.files {
		where[i] = name + "/zone_source/" + f
	}
	switch {
	case len(log.errors) > 0:
		w.Out(fmt.Sprintf("link of %s failed with %d error(s): %s; all in %s", name, len(log.errors), named(log.errors, len(log.errors)), strings.Join(where, ", ")))
		return
	case len(log.warnings) > 0:
		w.Out(fmt.Sprintf("link of %s built, with %d warning(s): %s; all in %s%s", name, len(log.warnings), named(log.warnings, len(log.warnings)), strings.Join(where, ", "), w.missing(target, since)))
	default:
		w.Out(fmt.Sprintf("link of %s ok%s", name, w.missing(target, since)))
	}
}

// missing names the assets the zones the link wrote looked up and found in no
// GDT: the linker links on without them and logs nothing.
func (w *RecordWatcher) missing(target string, since time.Time) string {
	dir := filepath.Join(target, "zone_source", "all", "assetinfo")
	deps, _ := filepath.Glob(filepath.Join(dir, "*.deps"))
	var ids []string
	for _, d := range deps {
		if fileMTime(d).Before(since.Add(-2 * time.Second)) {
			continue
		}
		m, _ := zone.MissingLookups(w.Root, dir, strings.TrimSuffix(filepath.Base(d), ".deps"))
		for _, x := range m {
			ids = append(ids, x.ID.String())
		}
	}
	if len(ids) == 0 {
		return ""
	}
	return fmt.Sprintf(", but %d asset(s) it named are in no GDT and it linked on without them: %s", len(ids), named(ids, len(ids)))
}

// sayCompile says a compile's problems: a leak, from the leakfile it wrote,
// and what its .errlog places. A compile with neither says nothing.
func (w *RecordWatcher) sayCompile(base string, since time.Time) {
	var parts []string
	if !fileMTime(base + ".lin").Before(since.Add(-2 * time.Second)) {
		parts = append(parts, fmt.Sprintf("the map leaks, and the leakfile %s.lin traces the hole to the void (t7kb:mapping)", w.rel(base)))
	}
	if placed, _, n := readErrlog(base+".errlog", since); n > 0 {
		parts = append(parts, fmt.Sprintf("%s.errlog places %d problem(s): %s", w.rel(base), n, named(placed, n)))
	}
	if len(parts) > 0 {
		w.Out(fmt.Sprintf("compile of %s.map: %s", w.rel(base), strings.Join(parts, "; ")))
	}
}

// rel is a path as a line names it: from the root, with slashes.
func (w *RecordWatcher) rel(p string) string {
	r, err := filepath.Rel(w.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

// named joins the first maxSaid of a list of total items and counts the rest.
func named(list []string, total int) string {
	s := strings.Join(list[:min(len(list), maxSaid)], " | ")
	if total > maxSaid {
		s += fmt.Sprintf(" | and %d more", total-maxSaid)
	}
	return s
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
