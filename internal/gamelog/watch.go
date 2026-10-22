package gamelog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Game is the running game as a watcher sees it, besides its logs.
type Game interface {
	// Running are the pids of the game's processes.
	Running() []int
	// Dialogs are the dialogs a process shows, one line each.
	Dialogs(pid int) []string
	// Exit is the exit code of a process Running listed that has ended;
	// ended is false while it runs.
	Exit(pid int) (code uint32, ended bool)
}

// maxEvents is how many events one launch of the game says at most: past it,
// one line points to game_log.
const maxEvents = 20

// headLen is how much of a log's start tells one launch from the next: its
// `logfile opened on <date>` header.
const headLen = 128

// Watcher follows the game's logs and processes and says each problem once,
// as one line to Out: see the package doc.
type Watcher struct {
	Root  string       // the game's root
	Game  Game         // its processes
	Out   func(string) // gets each event
	start time.Time
	logs  map[string]*follow
	pids  map[int]bool
	said  map[string]bool // dialog lines said
	fatal []string        // the fatal errors the logs said, for the dialogs that repeat them
}

// follow is one log being followed.
type follow struct {
	name   string // as an event names it, relative to the root
	offset int64
	line   int
	head   []byte
	part   []byte // a line not yet ended
	launch *launch
}

// launch is what a log said since its last launch.
type launch struct {
	sorter
	scripts map[string]bool
	events  int
	lookups []Line // the failed lookups since the last map load began
	others  int    // the other Error lines since then
}

func newLaunch() *launch {
	return &launch{sorter: sorter{Summary: Summary{Noise: map[string]int{}}, seen: map[string]bool{}}, scripts: map[string]bool{}}
}

// Run ticks every interval until ctx ends.
func (w *Watcher) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		w.Tick()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick reads what the logs gained and looks at the game's processes once.
func (w *Watcher) Tick() {
	if w.logs == nil {
		w.start, w.logs, w.pids, w.said = time.Now(), map[string]*follow{}, map[int]bool{}, map[string]bool{}
	}
	paths, _ := filepath.Glob(filepath.Join(w.Root, "mods", "*", LogName))
	for _, p := range append(paths, filepath.Join(w.Root, LogName)) {
		w.readLog(p)
	}
	w.watchProcesses()
}

// readLog reads what the log at p gained since the last tick. A log already
// there when the watch began is followed from its end; one rewritten by a new
// launch, from its start.
func (w *Watcher) readLog(p string) {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return
	}
	f := w.logs[p]
	if f == nil {
		rel, _ := filepath.Rel(w.Root, p)
		f = &follow{name: filepath.ToSlash(rel), launch: newLaunch()}
		w.logs[p] = f
		if fi.ModTime().Before(w.start) {
			f.offset, f.line, f.head = fi.Size(), lineCount(p), readHead(p)
		}
	}
	if fi.Size() == f.offset {
		return
	}
	head := readHead(p)
	if fi.Size() < f.offset || !sameStart(head, f.head) {
		*f = follow{name: f.name, launch: newLaunch()}
	}
	if len(head) > len(f.head) {
		f.head = head
	}
	w.readFrom(p, f)
}

// sameStart reports whether two reads of a log's start agree as far as both go:
// a launch that rewrote the log changed its header.
func sameStart(a, b []byte) bool {
	n := min(len(a), len(b))
	return bytes.Equal(a[:n], b[:n])
}

func (w *Watcher) readFrom(p string, f *follow) {
	file, err := os.Open(p)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return
	}
	b, err := io.ReadAll(file)
	if err != nil {
		return
	}
	f.offset += int64(len(b))
	b = append(f.part, b...)
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		f.line++
		w.line(f, Line{f.line, strings.TrimSpace(logPrefix.ReplaceAllString(strings.TrimRight(string(b[:i]), "\r"), ""))})
		b = b[i+1:]
	}
	f.part = append([]byte(nil), b...)
}

// line says what one line of a log is worth saying.
func (w *Watcher) line(f *follow, l Line) {
	ln := f.launch
	switch {
	case strings.HasPrefix(l.Text, "logfile opened on "):
		f.launch = newLaunch()
		return
	case strings.HasSuffix(l.Text, "Game Initialization -------"):
		ln.lookups, ln.others = nil, 0
	case strings.HasPrefix(l.Text, "ModLoad done: "):
		w.loaded(f, l, strings.TrimPrefix(l.Text, "ModLoad done: "))
	case strings.HasPrefix(l.Text, "Com_ERROR: "):
		w.fatal = append(w.fatal, errorText(l.Text))
		w.say(f, fmt.Sprintf("fatal error, %s line %d: %s", f.name, l.N, strings.TrimPrefix(l.Text, "Com_ERROR: ")))
		return
	}
	if _, _, ok := lookupOf(l.Text); ok {
		ln.lookups = append(ln.lookups, l)
	} else if errorLine.MatchString(l.Text) && !cfgExec.MatchString(l.Text) && !strings.HasPrefix(l.Text, "Error: throwing script exception") && ln.sre == nil && !sreStart.MatchString(l.Text) {
		ln.others++
	}
	before := len(ln.Script)
	ln.line(l)
	for _, s := range ln.Script[before:] {
		if !ln.scripts[s.Text] {
			ln.scripts[s.Text] = true
			w.say(f, fmt.Sprintf("%s, %s line %d", s.Text, f.name, s.N))
		}
	}
}

// loaded says, once the map has loaded, the assets its zone lists that the
// game didn't find, and how many other Error lines the load printed.
func (w *Watcher) loaded(f *follow, l Line, name string) {
	ln := f.launch
	zoned := Zoned(w.Root, name, false)
	var mine []string
	for _, lk := range ln.lookups {
		if _, asset, _ := lookupOf(lk.Text); zoned[strings.ToLower(asset)] {
			mine = append(mine, asset)
		}
	}
	switch {
	case len(mine) > 0:
		w.say(f, fmt.Sprintf("%s loaded (%s line %d), but the game didn't find %d asset(s) its zone lists: %s; game_log %s has the lines",
			name, f.name, l.N, len(mine), strings.Join(mine[:min(len(mine), 3)], ", "), name))
	case ln.others > 0:
		w.say(f, fmt.Sprintf("%s loaded (%s line %d) after %d Error line(s) of no known kind; game_log %s has them", name, f.name, l.N, ln.others, name))
	}
	ln.lookups, ln.others = nil, 0
}

// say says one event, up to maxEvents for a launch.
func (w *Watcher) say(f *follow, s string) {
	ln := f.launch
	ln.events++
	switch {
	case ln.events < maxEvents:
		w.Out(s)
	case ln.events == maxEvents:
		w.Out(fmt.Sprintf("more problems in %s since this launch of the game: game_log has them", f.name))
	}
}

// watchProcesses says the dialogs the game opens, those a fatal error the log
// said already repeats left out, and a process that ended with an error code.
func (w *Watcher) watchProcesses() {
	running := map[int]bool{}
	for _, pid := range w.Game.Running() {
		running[pid], w.pids[pid] = true, true
		for _, d := range w.Game.Dialogs(pid) {
			key := fmt.Sprint(pid, d)
			if !w.said[key] && !w.repeatsFatal(d) {
				w.Out(fmt.Sprintf("BlackOps3.exe (pid %d): %s", pid, d))
			}
			w.said[key] = true
		}
	}
	for pid := range w.pids {
		if running[pid] {
			continue
		}
		if code, ended := w.Game.Exit(pid); ended {
			if code != 0 {
				w.Out(fmt.Sprintf("BlackOps3.exe (pid %d) ended with exit code 0x%X: the game's crashes.log may say why", pid, code))
			}
			delete(w.pids, pid)
		}
	}
}

func (w *Watcher) repeatsFatal(dialog string) bool {
	for _, f := range w.fatal {
		if strings.Contains(dialog, f) {
			return true
		}
	}
	return false
}

// errorText is an Error line from its first `Error:`, without the engine's
// prefix: `Com_ERROR: ERROR: Could not find zone …` as its dialog says it.
func errorText(t string) string {
	if i := errorLine.FindStringIndex(t); i != nil {
		return t[i[0]:]
	}
	return t
}

func readHead(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, headLen)
	n, _ := io.ReadFull(f, b)
	return b[:n]
}

func lineCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return bytes.Count(b, []byte{'\n'})
}
