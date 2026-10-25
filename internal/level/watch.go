package level

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant"
)

// retryAfter is how long the watch waits before connecting again to a bridge
// that refused it (an older plugin, a Radiant closing).
const retryAfter = 30 * time.Second

// Watcher says, one line each, what happens in each Radiant whose bridge
// runs: see the package doc's "Watching Radiant".
type Watcher struct {
	Machine radiant.Machine
	Out     func(string)
	Quiet   time.Duration // how long Radiant's console stays quiet before a burst of its errors is said
	follows map[int]*follow
	failed  map[int]time.Time // a bridge that refused the watch, and when
}

// follow is one Radiant the watch is connected to.
type follow struct {
	c       *radiant.Client
	mu      sync.Mutex
	events  []radiant.Event // received since the last tick
	closed  bool
	mapPath string
	depth   int
	errors  []string        // the console's errors since the last burst said, each once
	repeats int             // errors said again within the burst
	lastErr time.Time       // when the last error came
	bake    []string        // the warnings of the bake running
	seen    map[string]bool // the burst's errors
}

// Tick connects to each Radiant whose bridge listens and isn't followed yet,
// and says what the followed ones sent since the last tick.
func (w *Watcher) Tick() {
	if w.follows == nil {
		w.follows, w.failed = map[int]*follow{}, map[int]time.Time{}
	}
	pids, _ := w.Machine.Bridges()
	for _, pid := range pids {
		if w.follows[pid] == nil && time.Since(w.failed[pid]) >= retryAfter {
			w.connect(pid)
		}
	}
	pids = pids[:0]
	for pid := range w.follows {
		pids = append(pids, pid)
	}
	slices.Sort(pids)
	for _, pid := range pids {
		w.drain(pid, w.follows[pid])
	}
}

// connect follows a Radiant from now on: what it has open already is no news.
func (w *Watcher) connect(pid int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := radiant.Connect(ctx, w.Machine, pid)
	if err != nil {
		w.failed[pid] = time.Now()
		return
	}
	st, err := c.State(ctx)
	if err == nil {
		_, err = c.Call(ctx, "subscribe console map", nil)
	}
	if err != nil {
		c.Close()
		w.failed[pid] = time.Now()
		return
	}
	f := &follow{c: c, mapPath: st.Map, depth: st.PrefabDepth, seen: map[string]bool{}}
	go f.pump()
	w.follows[pid] = f
}

// pump keeps the connection's events until it closes, so a burst of the
// console between two ticks is never dropped.
func (f *follow) pump() {
	for e := range f.c.Events() {
		f.mu.Lock()
		f.events = append(f.events, e)
		f.mu.Unlock()
	}
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

// drain says what one Radiant sent since the last tick.
func (w *Watcher) drain(pid int, f *follow) {
	f.mu.Lock()
	events, closed := f.events, f.closed
	f.events = nil
	f.mu.Unlock()
	for _, e := range events {
		switch e.Name {
		case "map":
			w.mapEvent(pid, f, e.Args)
		case "console":
			w.console(pid, f, strings.TrimSpace(e.Args))
		}
	}
	if len(f.errors) > 0 && (closed || time.Since(f.lastErr) >= w.Quiet) {
		total := len(f.errors) + f.repeats
		w.Out(fmt.Sprintf("Radiant (pid %d) printed %d error(s): %s", pid, total, joinCapped(f.errors, len(f.errors))))
		f.errors, f.repeats, f.seen = nil, 0, map[string]bool{}
	}
	if closed {
		w.Out(fmt.Sprintf("Radiant (pid %d), on %s, closed: radiant_open opens a map again", pid, mapName(f.mapPath)))
		delete(w.follows, pid)
	}
}

// mapEvent says a map Radiant opened, or a prefab it entered or left; a save
// says nothing.
func (w *Watcher) mapEvent(pid int, f *follow, args string) {
	kind, rest, _ := strings.Cut(args, " ")
	switch kind {
	case "loaded":
		if !strings.EqualFold(strings.ReplaceAll(rest, `\`, "/"), strings.ReplaceAll(f.mapPath, `\`, "/")) {
			w.Out(fmt.Sprintf("Radiant (pid %d) opened %s: map_find reads what is in it", pid, mapName(rest)))
		}
		f.mapPath, f.depth = rest, 0
	case "scope":
		d, path, _ := strings.Cut(rest, " ")
		var depth int
		if _, err := fmt.Sscan(d, &depth); err != nil || depth == f.depth {
			return
		}
		if depth > 0 {
			w.Out(fmt.Sprintf("Radiant (pid %d) entered the prefab %s: the map tools act on it until Radiant leaves it", pid, mapName(path)))
		} else {
			w.Out(fmt.Sprintf("Radiant (pid %d) left its prefabs, back on %s", pid, mapName(path)))
		}
		f.depth = depth
	}
}

// console keeps the console's errors for their burst's line, and says a bake
// once it ends, with what it warned of.
func (w *Watcher) console(pid int, f *follow, line string) {
	switch {
	case strings.HasPrefix(line, "ERROR: "):
		msg := strings.TrimPrefix(line, "ERROR: ")
		if f.seen[msg] {
			f.repeats++
		} else {
			f.seen[msg] = true
			f.errors = append(f.errors, msg)
		}
		f.lastErr = time.Now()
	case strings.HasPrefix(line, "All Probes Done"):
		s := fmt.Sprintf("Radiant (pid %d) finished a bake (%s)", pid, line)
		if warned := bakeWarnings(f.bake); len(warned) > 0 {
			s += ", warning of " + joinCapped(warned, len(warned))
		}
		w.Out(s + ": map_view under real lighting shows it")
		f.bake = nil
	case bakeWarning.MatchString(line):
		f.bake = append(f.bake, line)
	}
}

// joinCapped joins the first three of a list of total items and counts the
// rest.
func joinCapped(list []string, total int) string {
	s := strings.Join(list[:min(len(list), 3)], " | ")
	if total > 3 {
		s += fmt.Sprintf(" | and %d more", total-3)
	}
	return s
}

// mapName is a map as a line names it: from map_source on when it is under
// one (a prefab's folders tell it from another), else its file's name.
// Radiant's paths are Windows ones, read here on any OS.
func mapName(path string) string {
	if path == "" {
		return "no map"
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if i := strings.Index(strings.ToLower(p), "/map_source/"); i >= 0 {
		return p[i+len("/map_source/"):]
	}
	return p[strings.LastIndex(p, "/")+1:]
}
