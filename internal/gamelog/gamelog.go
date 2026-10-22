package gamelog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LogName is the game's console log; LogDvar is the logfile value the run
// stage starts the game with when the run sets none: 2 writes each line as it
// comes, so the log holds the load even if the game then crashes.
const (
	LogName = "console_mp.log"
	LogDvar = "2"
)

// maxSRELines is how many of a script error's lines are kept: its message and
// first frames.
const maxSRELines = 12

var (
	logPrefix = regexp.MustCompile(`^\[\s*\d+\]\[[^\]]*\]\s*`)
	notFound  = regexp.MustCompile(`^Error: Could not find (\w+) "([^"]*)"`)
	waited    = regexp.MustCompile(`^Error: Waited \d+ msec for missing asset "([^"]*)"`)
	cfgExec   = regexp.MustCompile(`^Error: couldn't exec \S+\.cfg`)
	// A weapon none of the loaded zones carry.
	unknownWeapon = regexp.MustCompile(`^Error: BG_Cache_RegisterWeapon unknown weapon '([^']*)'`)
	errorLine     = regexp.MustCompile(`\b(?:Error|ERROR):\s*\S`)
	sreStart      = regexp.MustCompile(`^\*+ script error \*+$`)
	sreStack      = regexp.MustCompile(`^\*+ Call stack \*+$`)
	sreEnd        = regexp.MustCompile(`^\*{10,}$`)
)

// File is a log on disk.
type File struct {
	Path     string
	Modified time.Time
}

// Line is one line of a log, numbered from 1 in the file.
type Line struct {
	N    int    `json:"line"`
	Text string `json:"text"`
}

// Find is the current console_mp.log: for a map or mod name the newer of
// mods/<name>/ and the game's root, else the newest of the root's and every
// mod's; and the other logs beside it, newest first (every other
// console_mp.log, then the root's crashes.log and console.log). ok is false
// when there is none.
func Find(game, name string) (cur File, others []File, ok bool) {
	paths, _ := filepath.Glob(filepath.Join(game, "mods", "*", LogName))
	logs := files(append(paths, filepath.Join(game, LogName)))
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].Modified.After(logs[j].Modified) })
	at := -1
	for i, l := range logs {
		if name == "" || l.Path == filepath.Join(game, "mods", name, LogName) || l.Path == filepath.Join(game, LogName) {
			at = i
			break
		}
	}
	others = files([]string{filepath.Join(game, "crashes.log"), filepath.Join(game, "console.log")})
	if at < 0 {
		return File{}, append(logs, others...), false
	}
	rest := append(append([]File(nil), logs[:at]...), logs[at+1:]...)
	return logs[at], append(rest, others...), true
}

// files are the paths that are files, with their times.
func files(paths []string) []File {
	var out []File
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			out = append(out, File{p, fi.ModTime()})
		}
	}
	return out
}

// Written is the console_mp.log a run started at start wrote, "" when it wrote
// none: the newer of mods/<name>/ and the root written since. A file's time
// lags the clock by up to a tick of the file system's, so since is a second
// before start.
func Written(game, name string, start time.Time) string {
	best, at := "", start.Add(-time.Second)
	for _, p := range []string{filepath.Join(game, "mods", name, LogName), filepath.Join(game, LogName)} {
		if fi, err := os.Stat(p); err == nil && !fi.ModTime().Before(at) {
			best, at = p, fi.ModTime()
		}
	}
	return best
}

// Launch is the last launch a log holds.
type Launch struct {
	Opened string // its `logfile opened on …` header, "" if the file has none
	Lines  []Line // the lines after the header, as written
	Total  int    // the file's line count
}

// Read reads the last launch of the log at path.
func Read(path string) (Launch, error) {
	f, err := os.Open(path)
	if err != nil {
		return Launch{}, err
	}
	defer f.Close()
	var l Launch
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		t := strings.TrimRight(sc.Text(), "\r")
		l.Total = n
		if h, ok := strings.CutPrefix(t, "logfile opened on "); ok {
			l.Opened, l.Lines = strings.TrimSpace(h), nil
			continue
		}
		l.Lines = append(l.Lines, Line{n, t})
	}
	return l, sc.Err()
}

// Summary is a launch's lines sorted into what is the map's and the stock
// noise every load prints.
type Summary struct {
	MapLoads []int          // the lines of each `Game Initialization`
	Loaded   int            // the line of `ModLoad done: <name>`, 0 if none
	Script   []Line         // script errors: message and frames on one line, at the block's first line
	Own      []Line         // the map's zoned assets the game didn't find, and Error lines of other kinds, once each
	Noise    map[string]int // failed lookups of assets the map doesn't zone, by type
	NoiseAll int
}

// Sort sorts a launch's lines for the map or mod name ("" for none), whose
// zones list zoned (lower case names).
func Sort(l Launch, name string, zoned map[string]bool) Summary {
	s := sorter{Summary: Summary{Noise: map[string]int{}}, name: name, zoned: zoned, seen: map[string]bool{}}
	for _, ln := range l.Lines {
		s.line(Line{ln.N, strings.TrimSpace(logPrefix.ReplaceAllString(ln.Text, ""))})
	}
	if s.sre != nil {
		s.endSRE()
	}
	return s.Summary
}

type sorter struct {
	Summary
	name  string
	zoned map[string]bool
	seen  map[string]bool
	sre   []string // the script error being read, nil outside one
	sreAt int
}

func (s *sorter) line(l Line) {
	if s.sre != nil && s.sreLine(l.Text) {
		return
	}
	switch {
	case sreStart.MatchString(l.Text):
		s.sre, s.sreAt = []string{}, l.N
	case strings.HasSuffix(l.Text, "Game Initialization -------"):
		s.MapLoads = append(s.MapLoads, l.N)
	case s.name != "" && l.Text == "ModLoad done: "+s.name:
		s.Loaded = l.N
	case strings.HasPrefix(l.Text, "Error: throwing script exception"):
		// the script error block that follows says it
	default:
		s.sortError(l)
	}
}

// sreLine reads a line inside a script error, false when the line is past
// it: a block whose closing line never came ends at the first line of another
// kind.
func (s *sorter) sreLine(t string) bool {
	if t == "" || sreStack.MatchString(t) {
		return true
	}
	body, inside := strings.CutPrefix(t, "Error:")
	if !inside {
		s.endSRE()
		return false
	}
	body = strings.TrimSpace(body)
	switch {
	case sreEnd.MatchString(body):
		s.endSRE()
	case body != "" && len(s.sre) < maxSRELines:
		s.sre = append(s.sre, body)
	}
	return true
}

func (s *sorter) endSRE() {
	s.Script = append(s.Script, Line{s.sreAt, "script error: " + strings.Join(s.sre, " | ")})
	s.sre = nil
}

// sortError files an Error line: the map's, or stock noise.
func (s *sorter) sortError(l Line) {
	if kind, name, ok := lookupOf(l.Text); ok {
		s.lookup(l, kind, name)
		return
	}
	switch {
	case cfgExec.MatchString(l.Text):
		s.Noise["config"]++
		s.NoiseAll++
	case errorLine.MatchString(l.Text):
		s.addOwn(l)
	}
}

// lookupOf reads a failed lookup: the asset's type and name.
func lookupOf(t string) (kind, name string, ok bool) {
	if m := notFound.FindStringSubmatch(t); m != nil {
		return m[1], m[2], true
	}
	if m := waited.FindStringSubmatch(t); m != nil {
		return "asset", m[1], true
	}
	if m := unknownWeapon.FindStringSubmatch(t); m != nil {
		return "weapon", m[1], true
	}
	return "", "", false
}

// lookup files a failed lookup: the map's when its zones list the asset.
func (s *sorter) lookup(l Line, kind, name string) {
	if s.zoned[strings.ToLower(name)] {
		s.addOwn(l)
		return
	}
	s.Noise[kind]++
	s.NoiseAll++
}

func (s *sorter) addOwn(l Line) {
	if !s.seen[l.Text] {
		s.seen[l.Text] = true
		s.Own = append(s.Own, l)
	}
}

// Zoned are the asset names a map's zone, or a mod's zones, list, in lower
// case: a lookup that fails for one of them is the map's problem.
func Zoned(game, name string, isMod bool) map[string]bool {
	files := []string{filepath.Join(game, "usermaps", name, "zone_source", name+".zone")}
	if isMod {
		files, _ = filepath.Glob(filepath.Join(game, "mods", name, "zone_source", "*.zone"))
	}
	names := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "//") || strings.HasPrefix(l, ">") {
				continue
			}
			if _, n, ok := strings.Cut(l, ","); ok {
				names[strings.ToLower(strings.TrimSpace(n))] = true
			}
		}
	}
	return names
}

// Match is a line that matched, with the lines around it.
type Match struct {
	Line
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

// Grep is the launch's lines matching re, at most max, each with context
// lines before and after; more reports whether others matched past max.
func Grep(l Launch, re *regexp.Regexp, context, max int) (ms []Match, more int) {
	for i, ln := range l.Lines {
		if !re.MatchString(ln.Text) {
			continue
		}
		if len(ms) == max {
			more++
			continue
		}
		ms = append(ms, Match{Line: ln, Before: texts(l.Lines[max0(i-context):i]), After: texts(l.Lines[i+1 : min(len(l.Lines), i+1+context)])})
	}
	return ms, more
}

// Tail is the launch's last n lines.
func Tail(l Launch, n int) []Line {
	return l.Lines[max0(len(l.Lines)-n):]
}

func texts(ls []Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func max0(i int) int {
	return max(i, 0)
}

// Kinds is a count by kind, the largest first: "98 fx, 71 material".
func Kinds(n map[string]int) string {
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if n[keys[i]] != n[keys[j]] {
			return n[keys[i]] > n[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%d %s", n[k], k)
	}
	return strings.Join(parts, ", ")
}
