package gdt

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// The asset index maps every asset name to where it is defined, like the
// database gdtdb keeps for APE. It is cached on disk and refreshed incrementally:
// only GDTs whose size or mtime changed are re-read.

type hit struct {
	File   string // relative to the root, forward slashes
	Line   int
	Name   string
	Type   string
	Parent string
}

type fileEntry struct {
	Mod    int64
	Size   int64
	Assets []hit
}

type index struct {
	mu     sync.RWMutex
	files  map[string]*fileEntry
	byName map[string][]hit
}

// indexVersion 2: gob instead of JSON — the cache of a full install decodes in
// ~40 ms instead of ~270 ms, on every CLI call.
const indexVersion = 2

type indexDisk struct {
	Version int
	Root    string
	Files   map[string]*fileEntry
}

// headerRE matches an asset header line: "name" ( "type.gdf" ) or "name" [ "parent" ].
var headerRE = regexp.MustCompile(`^\s*"([^"]+)"\s*(?:\(\s*"([^"]+?)(?:\.gdf)?"\s*\)|\[\s*"([^"]+)"\s*\])\s*$`)

func indexPath(root string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(dir, "t7kb", fmt.Sprintf("gdt-index-%x.gob", h.Sum64()))
}

// loadIndex reads the cached index, walks the tree for GDTs, re-reads only the
// changed ones, and saves the result.
func loadIndex(root string) (*index, error) {
	cache := indexPath(root)
	idx := &index{files: readIndexCache(cache, root)}
	jobs, err := staleGDTs(root, idx.files)
	if err != nil {
		return nil, err
	}
	if len(jobs) > 0 {
		var mu sync.Mutex
		parallel(jobs, func(j indexJob) {
			assets := scanHeaders(filepath.Join(root, filepath.FromSlash(j.rel)), j.rel)
			mu.Lock()
			idx.files[j.rel] = &fileEntry{Mod: j.mod, Size: j.size, Assets: assets}
			mu.Unlock()
		})
		writeIndexCache(cache, indexDisk{Version: indexVersion, Root: root, Files: idx.files})
	}
	idx.rebuildNames()
	return idx, nil
}

// readIndexCache returns the cached entries for root, or an empty map when the
// cache is missing, stale in format, or for another root.
func readIndexCache(cache, root string) map[string]*fileEntry {
	var disk indexDisk
	if b, err := os.ReadFile(cache); err == nil && gob.NewDecoder(bytes.NewReader(b)).Decode(&disk) == nil &&
		disk.Version == indexVersion && strings.EqualFold(disk.Root, root) && disk.Files != nil {
		return disk.Files
	}
	return map[string]*fileEntry{}
}

// writeIndexCache saves the index best-effort, atomically: the MCP server and a
// CLI call may share the cache, and a reader must never see half a file.
func writeIndexCache(cache string, disk indexDisk) {
	var b bytes.Buffer
	if gob.NewEncoder(&b).Encode(disk) != nil || os.MkdirAll(filepath.Dir(cache), 0o755) != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", cache, os.Getpid())
	if os.WriteFile(tmp, b.Bytes(), 0o644) != nil || os.Rename(tmp, cache) != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Remove(strings.TrimSuffix(cache, ".gob") + ".json") // the version-1 cache, if any
}

// indexJob is a GDT to (re)scan: new, or changed since it was cached.
type indexJob struct {
	rel       string
	mod, size int64
}

// staleGDTs walks exactly the directories gdtdb scans for APE
// (bin/converter_gdt_dirs_0.txt), drops cached entries for GDTs that are gone,
// and returns the GDTs whose size or mtime changed.
func staleGDTs(root string, files map[string]*fileEntry) ([]indexJob, error) {
	var mu sync.Mutex
	var jobs []indexJob
	seen := map[string]bool{}
	var roots []string
	for _, dir := range GDTDirs(root) {
		roots = append(roots, filepath.Join(root, dir))
	}
	walkParallel(roots, func(p string, d fs.DirEntry) {
		if !strings.EqualFold(filepath.Ext(p), ".gdt") {
			return
		}
		info, err := d.Info()
		if err != nil {
			return
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		mu.Lock()
		defer mu.Unlock()
		seen[rel] = true
		if fe, ok := files[rel]; !ok || fe.Mod != info.ModTime().UnixNano() || fe.Size != info.Size() {
			jobs = append(jobs, indexJob{rel, info.ModTime().UnixNano(), info.Size()})
		}
	})
	for rel := range files {
		if !seen[rel] {
			delete(files, rel)
		}
	}
	return jobs, nil
}

// walkParallel calls visit for every file under the roots, reading directories
// concurrently: model_export alone holds ~270k files in ~38k directories, and a
// sequential walk spent seconds just listing them. Directories in skipDirs and
// unreadable ones are skipped; symlinked directories aren't followed (as with
// filepath.WalkDir). visit runs concurrently.
func walkParallel(roots []string, visit func(path string, d fs.DirEntry)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4*runtime.NumCPU()) // directories being read at once
	var walk func(dir string)
	walk = func(dir string) {
		defer wg.Done()
		slots <- struct{}{}
		entries, err := os.ReadDir(dir)
		<-slots
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if !e.IsDir() {
				visit(p, e)
			} else if !skipDirs[e.Name()] {
				wg.Add(1)
				go walk(p)
			}
		}
	}
	for _, r := range roots {
		wg.Add(1)
		go walk(r)
	}
	wg.Wait()
}

// scanHeaders reads only the asset header lines of a GDT (fast; no full parse).
func scanHeaders(path, rel string) []hit {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []hit
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		// a header has no second quoted pair; cheap reject of "key" "value" lines
		if len(b) == 0 || (b[len(b)-1] != ')' && b[len(b)-1] != ']' && b[len(b)-1] != '\r') {
			continue
		}
		m := headerRE.FindSubmatch(b)
		if m == nil {
			continue
		}
		out = append(out, hit{File: rel, Line: line, Name: string(m[1]), Type: string(m[2]), Parent: string(m[3])})
	}
	return out
}

func (idx *index) rebuildNames() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.byName = map[string][]hit{}
	for _, fe := range idx.files {
		for _, h := range fe.Assets {
			idx.byName[h.Name] = append(idx.byName[h.Name], h)
		}
	}
}

func (idx *index) lookup(name string) []hit {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return append([]hit(nil), idx.byName[name]...)
}

// restat re-reads the indexed GDTs whose size or mtime changed, and drops the
// ones that disappeared. It does not discover new files (the full walk does).
func (idx *index) restat(root string) {
	idx.mu.RLock()
	var changed, gone []string
	for rel, fe := range idx.files {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		switch {
		case err != nil:
			gone = append(gone, rel)
		case info.ModTime().UnixNano() != fe.Mod || info.Size() != fe.Size:
			changed = append(changed, rel)
		}
	}
	idx.mu.RUnlock()
	if len(changed) == 0 && len(gone) == 0 {
		return
	}
	idx.mu.Lock()
	for _, rel := range gone {
		delete(idx.files, rel)
	}
	for _, rel := range changed {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if info, err := os.Stat(p); err == nil {
			idx.files[rel] = &fileEntry{Mod: info.ModTime().UnixNano(), Size: info.Size(), Assets: scanHeaders(p, rel)}
		}
	}
	idx.mu.Unlock()
	idx.rebuildNames()
}

// replace swaps in a freshly walked index.
func (idx *index) replace(fresh *index) {
	fresh.mu.RLock()
	files := fresh.files
	fresh.mu.RUnlock()
	idx.mu.Lock()
	idx.files = files
	idx.mu.Unlock()
	idx.rebuildNames()
}

// refreshFile re-reads one GDT (after a write) and updates the in-memory index.
func (idx *index) refreshFile(root, rel string) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	idx.mu.Lock()
	if err != nil {
		delete(idx.files, rel)
	} else {
		idx.files[rel] = &fileEntry{Mod: info.ModTime().UnixNano(), Size: info.Size(), Assets: scanHeaders(p, rel)}
	}
	idx.mu.Unlock()
	idx.rebuildNames()
}

// GDTDirs returns the directories gdtdb indexes, read from
// bin/converter_gdt_dirs_0.txt — the same list APE's database is built from.
// Falls back to the stock set when the file is missing.
func GDTDirs(root string) []string {
	var dirs []string
	if f, err := os.Open(filepath.Join(root, "bin", "converter_gdt_dirs_0.txt")); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if d := strings.TrimSpace(sc.Text()); d != "" {
				if _, err := os.Stat(filepath.Join(root, d)); err == nil {
					dirs = append(dirs, d)
				}
			}
		}
		f.Close()
	}
	if len(dirs) == 0 {
		dirs = []string{"source_data", "model_export", "texture_assets", "xanim_export", "usermaps", "mods"}
	}
	return dirs
}
