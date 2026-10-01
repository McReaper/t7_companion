package gdt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// readEach reads the files (relative to the root) on every CPU and calls fn
// with each one's bytes or read error. fn runs concurrently: guard what it shares.
func (w *Workspace) readEach(files []string, fn func(rel string, b []byte, err error)) {
	parallel(files, func(rel string) {
		b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
		fn(rel, b, err)
	})
}

// parallel calls fn for every item, on every CPU, and returns when all are done.
func parallel[T any](items []T, fn func(T)) {
	ch := make(chan T)
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				fn(it)
			}
		}()
	}
	for _, it := range items {
		ch <- it
	}
	close(ch)
	wg.Wait()
}

// indexedFiles lists the indexed GDTs that keep pass (nil keeps all), after a
// refresh. keep is called under the index lock.
func (w *Workspace) indexedFiles(keep func(rel string, fe *fileEntry) bool) ([]string, error) {
	w.refresh()
	if w.scErr != nil {
		return nil, w.scErr
	}
	w.idx.mu.RLock()
	defer w.idx.mu.RUnlock()
	files := make([]string, 0, len(w.idx.files))
	for rel, fe := range w.idx.files {
		if keep == nil || keep(rel, fe) {
			files = append(files, rel)
		}
	}
	return files, nil
}

// isStockRel reports whether an index-relative path is a stock GDT.
func (w *Workspace) isStockRel(rel string) bool { return w.stock[strings.ToLower(rel)] }
