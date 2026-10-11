package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxLogEntries caps each of a link's error and warning lists; the rest stay
// in the errorlog the last entry names.
const maxLogEntries = 30

// linkLog is what one link wrote to its errorlogs: the linker's return codes,
// and every error (a ^1 line) and warning (^3), each with the chain of assets
// the linker indents under it.
type linkLog struct {
	codes, errors, warnings []string
	files                   []string // the errorlogs read, relative to zone_source
}

// readLinkLogs reads every errorlog this link wrote under zoneSource
// (<zone>/zone_source): all/assetinfo/<zone>.errorlog for the
// language-neutral pass and <language>/assetinfo/<prefix>_<zone>.errorlog for
// each language. One counts when it is no more than 2 s older than since; ok
// is false when there is none.
func readLinkLogs(zoneSource string, since time.Time) (log linkLog, ok bool) {
	paths, _ := filepath.Glob(filepath.Join(zoneSource, "*", "assetinfo", "*.errorlog"))
	sort.Strings(paths)
	seen := map[string]bool{}
	for _, p := range paths {
		if fileMTime(p).Before(since.Add(-2 * time.Second)) {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		ok = true
		rel, _ := filepath.Rel(zoneSource, p)
		log.files = append(log.files, filepath.ToSlash(rel))
		log.add(string(b), seen)
	}
	return log, ok
}

// add reads one errorlog: `return <code>`, then entries, each a ^1 or ^3 line
// followed by its indented chain (`  xmodel:x`, `    csv:zone_source/…`),
// written as "message (xmodel:x < csv:…)". An entry seen already is skipped.
func (l *linkLog) add(text string, seen map[string]bool) {
	var cur *string
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
		case strings.HasPrefix(line, "return "):
			l.codes = append(l.codes, strings.TrimPrefix(line, "return "))
		case strings.HasPrefix(line, "^1"), strings.HasPrefix(line, "^3"):
			l.flush(cur, seen)
			e := line
			cur = &e
		case cur != nil && raw != line: // an indented chain line
			if strings.HasSuffix(*cur, ")") && strings.Contains(*cur, " (") {
				*cur = strings.TrimSuffix(*cur, ")") + " < " + line + ")"
			} else {
				*cur += " (" + line + ")"
			}
		}
	}
	l.flush(cur, seen)
}

// flush files the entry under errors or warnings by its colour code.
func (l *linkLog) flush(e *string, seen map[string]bool) {
	if e == nil || seen[*e] {
		return
	}
	seen[*e] = true
	if strings.HasPrefix(*e, "^1") {
		l.errors = append(l.errors, strings.TrimPrefix(*e, "^1"))
	} else {
		l.warnings = append(l.warnings, strings.TrimPrefix(*e, "^3"))
	}
}

// capped keeps the first maxLogEntries of a list and says where the rest are.
func capped(list []string, files []string) []string {
	if len(list) <= maxLogEntries {
		return list
	}
	return append(list[:maxLogEntries:maxLogEntries], fmt.Sprintf("…and %d more in %s", len(list)-maxLogEntries, strings.Join(files, ", ")))
}

// applyLinkLog puts what the link's errorlogs say into its stage result: the
// linker exits non-zero on warnings too, so a link whose errorlogs hold no
// error built its fastfile. prefix names a mod's zone.
func applyLinkLog(sr *stageResult, zoneSource, prefix string, since time.Time) {
	log, ok := readLinkLogs(zoneSource, since)
	if !ok {
		return
	}
	for _, w := range capped(log.warnings, log.files) {
		sr.Warnings = append(sr.Warnings, prefix+w)
	}
	if len(log.errors) > 0 {
		sr.Errors = nil
		for _, e := range capped(log.errors, log.files) {
			sr.Errors = append(sr.Errors, prefix+e)
		}
		return
	}
	if !sr.OK {
		sr.OK, sr.Errors = true, nil
		sr.Note = joinNotes(sr.Note, prefix+"the linker returned "+strings.Join(log.codes, ", ")+" for warnings only: the fastfile is built")
	}
}
