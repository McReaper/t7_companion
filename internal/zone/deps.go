package zone

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Changed lists the source files the last link read that have changed since:
// modified after the timestamp the linker recorded in <zone>.deps, or gone.
// The .zone and .zpkg files are among them; GDTs are not (the linker records a
// hash per GDT asset instead), so a GDT edit has to be checked separately.
func Changed(dir, zoneName string) ([]string, error) {
	files, err := depFiles(filepath.Join(dir, zoneName+".deps"))
	if err != nil {
		return nil, err
	}
	var (
		mu      sync.Mutex
		changed []string
		wg      sync.WaitGroup
		next    = make(chan dep)
	)
	for range 16 { // stat is I/O-bound: a few workers hide the latency
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range next {
				if st, err := os.Stat(d.path); err != nil || st.ModTime().Unix() > d.stamp {
					mu.Lock()
					changed = append(changed, d.path)
					mu.Unlock()
				}
			}
		}()
	}
	for _, d := range files {
		next <- d
	}
	close(next)
	wg.Wait()
	sort.Strings(changed)
	return changed, nil
}

type dep struct {
	path  string
	stamp int64
}

// depFiles reads the `\tfile,<path>,<unix time>[,hash…]` lines, each path once.
func depFiles(path string) ([]dep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []dep
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "\tfile,")
		if !ok {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 || seen[parts[0]] {
			continue
		}
		stamp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		seen[parts[0]] = true
		out = append(out, dep{parts[0], stamp})
	}
	return out, sc.Err()
}
