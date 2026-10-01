package gdt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// stockValues caches, per root and asset type, every value each field takes in
// Treyarch's stock GDTs (lower-cased): field -> value -> true.
var stockValues sync.Map

// stockUses reports whether a stock asset of this type gives field this value.
// What stock does still links, so it is never an error — even where the current
// .awi no longer lists the value. The scan runs once per type, and only when a
// value fails the .awi's list.
func (w *Workspace) stockUses(typ, field, value string) bool {
	vals := w.stockFieldValues(typ)
	return vals[strings.ToLower(field)][strings.ToLower(value)]
}

func (w *Workspace) stockFieldValues(typ string) map[string]map[string]bool {
	key := w.Root + "|" + strings.ToLower(typ)
	if v, ok := stockValues.Load(key); ok {
		return v.(map[string]map[string]bool)
	}
	w.refresh()
	if w.scErr != nil {
		return nil
	}
	var files []string
	w.idx.mu.RLock()
	for rel, fe := range w.idx.files {
		if !w.stock[strings.ToLower(rel)] {
			continue
		}
		for _, h := range fe.Assets {
			if strings.EqualFold(h.Type, typ) {
				files = append(files, rel)
				break
			}
		}
	}
	w.idx.mu.RUnlock()

	out := map[string]map[string]bool{}
	var mu sync.Mutex
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range ch {
				b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
				if err != nil {
					continue
				}
				f, err := Parse(b)
				if err != nil {
					continue
				}
				mu.Lock()
				for _, a := range f.Assets {
					if !strings.EqualFold(a.Type, typ) {
						continue
					}
					for _, fl := range a.Fields {
						k := strings.ToLower(fl.Key)
						if out[k] == nil {
							out[k] = map[string]bool{}
						}
						out[k][strings.ToLower(strings.TrimSuffix(Unquote(fl.Value), "*"))] = true
					}
				}
				mu.Unlock()
			}
		}()
	}
	for _, rel := range files {
		ch <- rel
	}
	close(ch)
	wg.Wait()
	v, _ := stockValues.LoadOrStore(key, out)
	return v.(map[string]map[string]bool)
}
