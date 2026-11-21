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
	Builds   func() []State        // the states of the builds t7kb ran on the install (States); nil for none
	start    time.Time
	seen     map[string]time.Time // each record's mtime, as last seen
	links    map[string]*records  // by target folder, a link not said yet
	compiles map[string]*records  // by map path without its extension, a compile not said yet
	builds   map[string]bool      // t7kb's builds, by target and start: true once said, false while seen running
	states   []State              // t7kb's builds, as Builds last said
	last     map[string]*lastLink // by target folder, what its last link left
	lit      map[string]time.Time // by map target, the .led a link was last said to light it with stale
}

// lastLink is what a target's last link left, so that a link says only what
// changed: whether it broke (it failed or stopped), and the warnings and the
// lookups in no GDT of the last one that built.
type lastLink struct {
	broke    bool
	warnings map[string]bool
	missing  map[string]bool // nil when unknown: the watch reads no .deps written before it began
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
		w.start, w.seen, w.links, w.compiles, w.builds, w.lit = now, map[string]time.Time{}, map[string]*records{}, map[string]*records{}, map[string]bool{}, map[string]time.Time{}
		w.last = w.lastOnDisk()
	}
	for _, kind := range []string{"usermaps", "mods"} {
		for _, ext := range []string{"deps", "errorlog", "deps.partial"} {
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
	if w.Builds != nil {
		w.states = w.Builds()
	}
	w.flush(w.links, linkerExe, now, w.sayLink)
	w.flush(w.compiles, compilerExe, now, w.sayCompile)
	for _, s := range w.states {
		w.sayBuild(s)
	}
}

// ranByT7kb reports whether a build t7kb ran wrote a target's link records
// written since: its own line says the link.
func (w *RecordWatcher) ranByT7kb(target string, since time.Time) bool {
	kind := strings.TrimSuffix(filepath.Base(filepath.Dir(target)), "s") // usermaps, mods
	for _, s := range w.states {
		if s.Kind == kind && strings.EqualFold(s.Target, filepath.Base(target)) && !since.Before(s.Started.Add(-2*time.Second)) &&
			(!s.Done || !since.After(s.Ended.Add(2*time.Second))) {
			return true
		}
	}
	return false
}

// sayBuild says a build t7kb ran once it has ended since the watch began, or
// once its process has ended before it did, if the watch saw it running.
func (w *RecordWatcher) sayBuild(s State) {
	key := s.Kind + "/" + s.Target + "@" + s.Started.String()
	said, seen := w.builds[key]
	switch {
	case said:
	case s.Done && s.Ended.After(w.start):
		w.builds[key] = true
		w.Out(buildLine(s))
	case s.Lost && seen:
		w.builds[key] = true
		w.Out(fmt.Sprintf("build of %s stopped at %s: its process ended before the build did; build's status has what it did", s.Target, lostAt(s)))
	case !s.Done && !s.Lost:
		w.builds[key] = false
	}
}

// buildLine says how a build ended: its stages and time, with what its link
// warned of and linked on without, or the stage that failed and its first
// errors.
func buildLine(s State) string {
	took := s.Ended.Sub(s.Started).Round(time.Second)
	if s.Report.OK {
		names := make([]string, len(s.Report.Stages))
		var more string
		for i, st := range s.Report.Stages {
			names[i] = st.Name
			if len(st.Warnings) > 0 {
				more += fmt.Sprintf("; its %s warned of %s", st.Name, named(st.Warnings, len(st.Warnings)))
			}
			if len(st.Missing) > 0 {
				more += fmt.Sprintf("; its link went on without what no GDT defines: %s", named(st.Missing, len(st.Missing)))
			}
		}
		return fmt.Sprintf("build of %s finished ok in %s: %s%s", s.Target, took, strings.Join(names, ", "), more)
	}
	why := "no error said"
	for _, st := range s.Report.Stages {
		if st.Name == s.Report.FailedStage && len(st.Errors) > 0 {
			why = named(st.Errors, len(st.Errors))
		}
	}
	return fmt.Sprintf("build of %s failed at %s after %s: %s; build's status has the report", s.Target, s.Report.FailedStage, took, why)
}

// lostAt is the stage a lost build was at.
func lostAt(s State) string {
	if s.Running != "" {
		return s.Running
	}
	return "its start"
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

// sayLink says each link once it has ended: one that failed or stopped, and
// one that built, so an agent waiting on a build hears it is done. Each line
// wakes the agent, so a built link names only what its last link didn't have
// and counts the rest. A link t7kb ran is its build's line's to say, and is
// the last link all the same. After a link that built,
// whoever ran it, staleLight says whether its map's lighting predates its
// compile.
func (w *RecordWatcher) sayLink(target string, since time.Time) {
	prev := w.last[target]
	if prev == nil {
		prev = &lastLink{}
	}
	name := w.rel(target)
	log, _ := readLinkLogs(filepath.Join(target, "zone_source"), since)
	for i, s := range log.errors {
		log.errors[i] = w.short(s)
	}
	for i, s := range log.warnings {
		log.warnings[i] = w.short(s)
	}
	where := make([]string, len(log.files))
	for i, f := range log.files {
		where[i] = name + "/zone_source/" + f
	}
	var line string
	cut := w.unfinished(target, since)
	built := len(cut) == 0 && len(log.errors) == 0
	if built {
		missing := w.missing(target, since)
		w.last[target] = &lastLink{warnings: setOf(log.warnings), missing: setOf(missing)}
		line = builtLine(name, prev, log.warnings, missing, where)
	} else {
		w.last[target] = &lastLink{broke: true, warnings: prev.warnings, missing: prev.missing}
		line = brokeLine(name, cut, log, where)
	}
	if !w.ranByT7kb(target, since) {
		w.Out(line)
	}
	if built {
		w.staleLight(target)
	}
}

// ledStamp is how a stale-lighting line dates the bake and the compile.
const ledStamp = "Jan 2 15:04"

// staleLight says, once per bake, that a map links with the lighting of a bake
// older than its last compile: the game lights a map from the .led only the
// light stage writes, so what a compile since brought in (a light, an
// emissive model, a probe) shows as that bake left it until the next one.
func (w *RecordWatcher) staleLight(target string) {
	name := filepath.Base(target)
	pp, _, _ := strings.Cut(name, "_") // zm, mp
	if filepath.Base(filepath.Dir(target)) != "usermaps" {
		return
	}
	maps := filepath.Join(w.Root, "share", "raw", "maps", pp)
	led, bsp := fileMTime(filepath.Join(maps, name+".led")), fileMTime(filepath.Join(maps, name+".d3dbsp"))
	if led.IsZero() || !bsp.After(led.Add(2*time.Second)) || w.lit[target].Equal(led) {
		return
	}
	w.lit[target] = led
	w.Out(fmt.Sprintf("link of %s used lighting baked %s, before the map's compile of %s: lights, emissives and probes changed since show in game only once the map is lit again (Light in the Launcher, or build's light stage)",
		w.rel(target), led.Format(ledStamp), bsp.Format(ledStamp)))
}

// brokeLine says a link that stopped before it finished, or that failed.
func brokeLine(name string, cut []string, log linkLog, where []string) string {
	if len(cut) == 0 {
		return fmt.Sprintf("link of %s failed with %d error(s): %s; all in %s", name, len(log.errors), named(log.errors, len(log.errors)), strings.Join(where, ", "))
	}
	line := fmt.Sprintf("link of %s stopped before it finished (a fatal error, a crash or a stop): %s stayed .partial, and its error went only to "+
		"the Launcher's console: ask for it, or link again with build, which reports it", name, strings.Join(cut, ", "))
	if len(log.errors) > 0 {
		line += fmt.Sprintf("; before that it logged %d error(s): %s", len(log.errors), named(log.errors, len(log.errors)))
	}
	return line
}

// builtLine says a link that built: it names the warnings and lookups in no
// GDT the last link didn't have, counts the ones it had too, and says "again"
// after a link that broke.
func builtLine(name string, prev *lastLink, warnings, missing, where []string) string {
	newWarnings, newMissing := unseen(warnings, prev.warnings), missing
	if prev.missing != nil {
		newMissing = unseen(missing, prev.missing)
	}
	head := "ok"
	if len(warnings) > 0 {
		head = "built"
	}
	if prev.broke {
		head += " again"
	}
	return "link of " + name + " " + head + warnedOf(warnings, newWarnings, where) + linkedWithout(missing, newMissing)
}

// warnedOf says a built link's warnings: the new ones named, and where all
// are.
func warnedOf(warnings, fresh, where []string) string {
	switch {
	case len(warnings) == 0:
		return ""
	case len(fresh) == 0:
		return fmt.Sprintf(", with the same %d warning(s) as before", len(warnings))
	}
	return fmt.Sprintf(", with %d warning(s)%s: %s; all in %s", len(warnings), ofThemNew(warnings, fresh), named(fresh, len(fresh)), strings.Join(where, ", "))
}

// linkedWithout says the lookups in no GDT a built link went on without: the
// new ones named.
func linkedWithout(missing, fresh []string) string {
	switch {
	case len(missing) == 0:
		return ""
	case len(fresh) == 0:
		return fmt.Sprintf(", but it linked on without the same %d asset(s) in no GDT as before", len(missing))
	}
	return fmt.Sprintf(", but %d asset(s) it named are in no GDT and it linked on without them%s: %s", len(missing), ofThemNew(missing, fresh), named(fresh, len(fresh)))
}

// ofThemNew says how many of a list are new, when not all of them are.
func ofThemNew(all, fresh []string) string {
	if len(fresh) == len(all) {
		return ""
	}
	return fmt.Sprintf(", %d of them new", len(fresh))
}

// lastOnDisk reads what each target's last link before the watch left: the
// errorlogs and .deps.partial written since the last link that got through
// its language-neutral pass. Its lookups in no GDT are left unknown, as
// reading every target's .deps would slow the watch's start: the first link
// it sees names them all.
func (w *RecordWatcher) lastOnDisk() map[string]*lastLink {
	last := map[string]*lastLink{}
	for _, kind := range []string{"usermaps", "mods"} {
		dirs, _ := filepath.Glob(filepath.Join(w.Root, kind, "*", "zone_source"))
		for _, zoneSource := range dirs {
			target := filepath.Dir(zoneSource)
			since := lastLinked(zoneSource)
			log, _ := readLinkLogs(zoneSource, since)
			for i, s := range log.warnings {
				log.warnings[i] = w.short(s)
			}
			last[target] = &lastLink{broke: len(log.errors) > 0 || len(w.unfinished(target, since)) > 0, warnings: setOf(log.warnings)}
		}
	}
	return last
}

// lastLinked is when the last link that got through its language-neutral
// pass wrote that pass's .deps, the newest under all/assetinfo: every link
// runs that pass first, and the records of a language pass only an earlier
// link ran stay beside the later ones.
func lastLinked(zoneSource string) time.Time {
	deps, _ := filepath.Glob(filepath.Join(zoneSource, "all", "assetinfo", "*.deps"))
	var newest time.Time
	for _, d := range deps {
		if m := fileMTime(d); m.After(newest) {
			newest = m
		}
	}
	return newest
}

// short writes the paths under the root in a linker message from the root,
// with slashes, as a line names them: the linker writes them whole, as
// <root>\/usermaps\<name>/zone_source/….
func (w *RecordWatcher) short(msg string) string {
	root := strings.TrimRight(strings.ReplaceAll(w.Root, `\`, "/"), "/")
	if root == "" {
		return msg
	}
	root += "/"
	var b strings.Builder
	for {
		i := indexFold(strings.ReplaceAll(msg, `\`, "/"), root)
		if i < 0 {
			b.WriteString(msg)
			return b.String()
		}
		b.WriteString(msg[:i])
		rest := strings.TrimLeft(msg[i+len(root):], `\/`)
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}
		b.WriteString(strings.ReplaceAll(rest[:end], `\`, "/"))
		msg = rest[end:]
	}
}

// indexFold is the index of sub's first occurrence in s, ignoring case, or -1.
func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// unseen is what of a list a set doesn't hold, in order.
func unseen(list []string, set map[string]bool) []string {
	var out []string
	for _, s := range list {
		if !set[s] {
			out = append(out, s)
		}
	}
	return out
}

// setOf is a list's items as a set, empty but not nil for an empty list.
func setOf(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, s := range list {
		set[s] = true
	}
	return set
}

// unfinished names the .deps a link left unfinished: the linker writes a
// zone's .deps.partial as it goes and its .deps once that zone is done, so a
// .deps.partial written by this link and not followed by its .deps is a link
// that stopped there, and a fatal error writes no errorlog.
func (w *RecordWatcher) unfinished(target string, since time.Time) []string {
	parts, _ := filepath.Glob(filepath.Join(target, "zone_source", "*", "assetinfo", "*.deps.partial"))
	var cut []string
	for _, p := range parts {
		m := fileMTime(p)
		done := strings.TrimSuffix(p, ".partial")
		if m.Before(since.Add(-2*time.Second)) || !fileMTime(done).Before(m) {
			continue
		}
		cut = append(cut, w.rel(done))
	}
	return cut
}

// missing names the assets the zones the link wrote looked up and found in no
// GDT: the linker links on without them and logs nothing.
func (w *RecordWatcher) missing(target string, since time.Time) []string {
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
	return ids
}

// sayCompile says a compile's problems: that it stopped before it finished, a
// leak, from the leakfile it wrote, and what its .errlog places. A compile
// writes its records before its .d3dbsp, an ents-only one too, so one whose
// .d3dbsp is older than its records stopped, unless t7kb ran it: its build
// says it. A compile with none says nothing.
func (w *RecordWatcher) sayCompile(base string, since time.Time) {
	var parts []string
	if !fileMTime(base + ".lin").Before(since.Add(-2 * time.Second)) {
		parts = append(parts, fmt.Sprintf("the map leaks, and the leakfile %s.lin traces the hole to the void (t7kb:mapping)", w.rel(base)))
	}
	if placed, _, n := readErrlog(base+".errlog", since); n > 0 {
		parts = append(parts, fmt.Sprintf("%s.errlog places %d problem(s): %s", w.rel(base), n, named(placed, n)))
	}
	head := "compile of " + w.rel(base) + ".map"
	if bsp := w.bspOf(base); fileMTime(bsp).Before(since) && !w.ranByT7kb(filepath.Join(w.Root, "usermaps", filepath.Base(base)), since) {
		head += fmt.Sprintf(" stopped before it finished (a fatal error, a crash or a stop): it didn't write %s, and its error went only to "+
			"the Launcher's console: ask for it, or compile again with build, which reports it", w.rel(bsp))
		if len(parts) > 0 {
			head += "; " + strings.Join(parts, "; ")
		}
		w.Out(head)
		return
	}
	if len(parts) > 0 {
		w.Out(head + ": " + strings.Join(parts, "; "))
	}
}

// bspOf is the .d3dbsp a compile of the map at base writes, as the Launcher
// has it: map_source/<dir>/<map>.map compiles to
// share/raw/maps/<dir>/<map>.d3dbsp.
func (w *RecordWatcher) bspOf(base string) string {
	return filepath.Join(w.Root, "share", "raw", "maps", filepath.Base(filepath.Dir(base)), filepath.Base(base)+".d3dbsp")
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
