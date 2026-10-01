package gdt

import (
	"strings"
	"sync"
)

// stockValues caches, per root and asset type, every value each combo field
// takes in Treyarch's stock GDTs (lower-cased): field -> value -> true.
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
	isCombo := w.comboFields(typ)
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
		local := comboValues(f, typ, isCombo) // outside the lock: every CPU works at once
		mu.Lock()
		defer mu.Unlock()
		for k, vs := range local {
			if out[k] == nil {
				out[k] = vs
				continue
			}
			for v := range vs {
				out[k][v] = true
			}
		}
	})
	v, _ := stockValues.LoadOrStore(key, out)
	return v.(map[string]map[string]bool)
}

// comboFields returns a concurrency-safe test for "this key is a Combo of typ",
// memoised per key: only combo values are ever looked up, and recording every
// field (texture names, paths…) of every stock asset is what made the scan slow.
func (w *Workspace) comboFields(typ string) func(key string) bool {
	sc, err := w.Schema(typ)
	if err != nil {
		return func(string) bool { return false }
	}
	var memo sync.Map
	return func(key string) bool {
		if v, ok := memo.Load(key); ok {
			return v.(bool)
		}
		e := sc.Lookup(key)
		is := e != nil && e.Kind == "Combo"
		memo.Store(key, is)
		return is
	}
}

// comboValues collects the (lower-cased) combo values of f's assets of typ.
func comboValues(f *File, typ string, isCombo func(string) bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, a := range f.Assets {
		if !strings.EqualFold(a.Type, typ) {
			continue
		}
		for _, fl := range a.Fields {
			if !isCombo(fl.Key) {
				continue
			}
			k := strings.ToLower(fl.Key)
			if out[k] == nil {
				out[k] = map[string]bool{}
			}
			out[k][strings.ToLower(strings.TrimSuffix(Unquote(fl.Value), "*"))] = true
		}
	}
	return out
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
