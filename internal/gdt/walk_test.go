package gdt

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// walkSequential is what walkParallel replaced: filepath.WalkDir with skipDirs.
func walkSequential(roots []string) []string {
	var out []string
	for _, r := range roots {
		_ = filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			out = append(out, p)
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func walkConcurrent(roots []string) []string {
	var mu sync.Mutex
	var out []string
	walkParallel(roots, func(p string, _ fs.DirEntry) {
		mu.Lock()
		out = append(out, p)
		mu.Unlock()
	})
	sort.Strings(out)
	return out
}

func TestWalkParallelMatchesWalkDir(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"a/one.gdt", "a/b/c/deep.gdt", "a/b/c/model.xmodel_bin", "a/b/two.GDT",
		"a/backup/old.gdt", "a/b/_backups/older.gdt", "a/.git/x.gdt", // skipped directories
		"z/three.gdt", "z/empty/.keep",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots := []string{filepath.Join(root, "a"), filepath.Join(root, "z"), filepath.Join(root, "missing")}
	seq, par := walkSequential(roots), walkConcurrent(roots)
	if strings.Join(seq, "\n") != strings.Join(par, "\n") {
		t.Fatalf("parallel walk differs:\n%s\nwant:\n%s", strings.Join(par, "\n"), strings.Join(seq, "\n"))
	}
	if len(par) != 6 {
		t.Fatalf("want the 6 files outside skipped directories, got %v", par)
	}
}

// T7KB_PROBE=1 TA_TOOLS_PATH=<root>: the same check over a real install.
func TestWalkParallelMatchesWalkDirOnInstall(t *testing.T) {
	root := os.Getenv("TA_TOOLS_PATH")
	if os.Getenv("T7KB_PROBE") == "" || root == "" {
		t.Skip()
	}
	var roots []string
	for _, d := range GDTDirs(root) {
		roots = append(roots, filepath.Join(root, d))
	}
	seq, par := walkSequential(roots), walkConcurrent(roots)
	if len(seq) != len(par) || strings.Join(seq, "\n") != strings.Join(par, "\n") {
		t.Fatalf("parallel walk found %d files, WalkDir %d", len(par), len(seq))
	}
	t.Logf("same %d files", len(par))
}

func TestIndexCacheRoundTrips(t *testing.T) {
	w := fixture(t)
	cache := indexPath(w.Root)
	old := strings.TrimSuffix(cache, ".gob") + ".json"
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"v":1}`), 0o644); err != nil { // a version-1 cache
		t.Fatal(err)
	}
	first, err := loadIndex(w.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the version-1 JSON cache should be removed once the gob one is written: %v", err)
	}
	cached := readIndexCache(cache, w.Root)
	if len(cached) == 0 || len(cached) != len(first.files) {
		t.Fatalf("cache holds %d files, index %d", len(cached), len(first.files))
	}
	if jobs, _ := staleGDTs(w.Root, cached); len(jobs) != 0 {
		t.Fatalf("a fresh cache needs no rescan, got %v", jobs)
	}
	second, _ := loadIndex(w.Root)
	if hits := second.lookup("child_mtl"); len(hits) != 1 || hits[0].Parent != "mine_base" || hits[0].File != "source_data/mine.gdt" {
		t.Fatalf("an index read back from the cache keeps its hits: %+v", hits)
	}
	if got := readIndexCache(cache, filepath.Join(w.Root, "other")); len(got) != 0 {
		t.Fatalf("a cache for another root is ignored")
	}
}
