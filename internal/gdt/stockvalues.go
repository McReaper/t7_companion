package gdt

import (
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
	files, err := w.indexedFiles(func(rel string, fe *fileEntry) bool { return w.isStockRel(rel) && fe.has(typ) })
	if err != nil {
		return nil
	}
	out := map[string]map[string]bool{}
	var mu sync.Mutex
	w.readEach(files, func(_ string, b []byte, err error) {
		if err != nil {
			return
		}
		f, err := Parse(b)
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		addFieldValues(f, typ, out)
	})
	v, _ := stockValues.LoadOrStore(key, out)
	return v.(map[string]map[string]bool)
}

// addFieldValues records every (lower-cased) field value of f's assets of typ.
func addFieldValues(f *File, typ string, out map[string]map[string]bool) {
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
}

// has reports whether the file defines an asset of typ.
func (fe *fileEntry) has(typ string) bool {
	for _, h := range fe.Assets {
		if strings.EqualFold(h.Type, typ) {
			return true
		}
	}
	return false
}
