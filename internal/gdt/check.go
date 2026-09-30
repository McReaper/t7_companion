package gdt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Ref is a field of one asset that names another asset, typed by the deffile
// (AssetCombo "image", Texture, …).
type Ref struct {
	Field  string `json:"field"`
	Target string `json:"target"`
	Type   string `json:"expects"`
}

// FileRef is a field that names a source file on disk.
type FileRef struct {
	Field  string `json:"field"`
	Path   string `json:"path"` // relative to the root
	Exists bool   `json:"exists"`
}

// fileFields maps asset type -> field -> the directory its path is relative to.
// Every other Path field with a SetRelativePath and a file extension is checked
// against that directory.
var fileFields = map[string]map[string]string{
	"xmodel": {"filename": "model_export", "mediumLod": "model_export", "lowLod": "model_export", "lowestLod": "model_export",
		"lod4File": "model_export", "lod5File": "model_export", "lod6File": "model_export", "lod7File": "model_export"},
	"xanim": {"filename": "xanim_export"},
	"image": {"baseImage": "."},
}

// coreTypes are the asset types whose AssetCombo targets the .awis declare
// precisely enough for a type mismatch to be an error.
var coreTypes = map[string]bool{"image": true, "material": true, "xmodel": true, "xanim": true}

// Refs returns the typed asset references among an asset's effective fields.
func (w *Workspace) Refs(typ string, fields []Field) []Ref {
	sc, err := w.Schema(typ)
	if err != nil {
		return nil
	}
	var out []Ref
	for _, f := range fields {
		v := Unquote(f.Value)
		if v == "" || strings.HasPrefix(v, "$") { // $white_diffuse etc. are engine built-ins
			continue
		}
		e := sc.Lookup(f.Key)
		if e == nil {
			continue
		}
		// Texture entries are not references: image.awi uses them for the source
		// file (baseImage) and composite channel names — FileRefs covers baseImage.
		if e.Kind == "AssetCombo" && e.AssetType != "" && !e.Varies {
			out = append(out, Ref{Field: f.Key, Target: v, Type: e.AssetType})
		}
	}
	return out
}

// FileRefs returns the source-file references among an asset's fields, with
// whether each exists on disk.
func (w *Workspace) FileRefs(typ string, fields []Field) []FileRef {
	sc, _ := w.Schema(typ)
	known := fileFields[typ]
	var out []FileRef
	for _, f := range fields {
		v := Unquote(f.Value)
		if v == "" {
			continue
		}
		base, ok := known[f.Key]
		if !ok && sc != nil {
			if e := sc.Lookup(f.Key); e != nil && e.Kind == "Path" && !e.Varies && e.RelPath != "" && filepath.Ext(v) != "" {
				base, ok = e.RelPath, true
			}
		}
		if !ok {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(v, `\`, "/"))))
		_, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(rel)))
		out = append(out, FileRef{Field: f.Key, Path: rel, Exists: err == nil})
	}
	return out
}

// AssetReport is the diagnostics for one asset.
type AssetReport struct {
	Asset  string  `json:"asset"`
	Type   string  `json:"type,omitempty"`
	Line   int     `json:"line"`
	Issues []Issue `json:"issues"`
}

// CheckResult is gdt_check's answer for one GDT.
type CheckResult struct {
	File     string        `json:"file"`
	Checked  int           `json:"assets_checked"`
	Errors   int           `json:"errors"`
	Warnings int           `json:"warnings"`
	Assets   []AssetReport `json:"assets_with_issues"`
}

// Check runs every diagnostic over a GDT (or one asset in it): schema values,
// material/techset rules, typed references (exists, right type), source files on
// disk, parent chain, and duplicate definitions across the workspace.
func (w *Workspace) Check(file, asset string) (*CheckResult, error) {
	f, err := w.Load(file)
	if err != nil {
		return nil, err
	}
	res := &CheckResult{File: w.Rel(f.Path), Assets: []AssetReport{}}
	for _, a := range f.Assets {
		if asset != "" && a.Name != asset {
			continue
		}
		res.Checked++
		rep := AssetReport{Asset: a.Name, Line: a.Line}
		if a.Parent != "" && f.Find(a.Parent) == nil {
			where := "is in no GDT"
			if locs, _ := w.Find(a.Parent); len(locs) > 0 {
				where = "is only in " + locs[0].File
			}
			rep.Issues = append(rep.Issues, Issue{"error", "", fmt.Sprintf("parent %q %s, not this GDT — gdtdb: `Parent Entity '%s' does not exist in GDT`", a.Parent, where, a.Parent)})
		}
		typ, fields, err := w.Resolved(f, a)
		if err != nil {
			return nil, err
		}
		rep.Type = typ
		if locs, _ := w.FindTyped(a.Name, typ); typ != "" && len(locs) > 1 {
			var where []string
			for _, l := range locs {
				where = append(where, fmt.Sprintf("%s:%d", l.File, l.Line))
			}
			rep.Issues = append(rep.Issues, Issue{"error", "", fmt.Sprintf("%s defined more than once (Duplicate '%s' asset at link): %s", typ, typ, strings.Join(where, ", "))})
		}
		if typ != "" {
			var keys []string
			for _, fl := range fields {
				keys = append(keys, fl.Key)
			}
			for _, is := range w.validate(f, typ, fields, keys) {
				if is.Level == "warning" && strings.Contains(is.Msg, "not declared in") {
					continue // script-built fields APE wrote; only new keys get this warning (gdt_edit)
				}
				if strings.Contains(is.Msg, "in any GDT (fine if it ships") {
					continue // reported below with the expected type
				}
				if strings.Contains(is.Msg, "doesn't read") {
					continue // stale slot from an earlier materialType: Treyarch's own GDTs keep ~40k of these and link
				}
				rep.Issues = append(rep.Issues, is)
			}
			for _, r := range w.Refs(typ, fields) {
				locs, _ := w.Find(r.Target)
				switch {
				case len(locs) == 0:
					rep.Issues = append(rep.Issues, Issue{"warning", r.Field, fmt.Sprintf("%s %q is in no GDT — fine only if it ships in a stock fastfile", r.Type, r.Target)})
				default:
					ok := false
					for _, l := range locs {
						for _, want := range strings.Split(r.Type, "|") { // "xmodel | character | aitype"
							if l.Type == "" || strings.EqualFold(l.Type, strings.TrimSpace(want)) {
								ok = true
							}
						}
					}
					if !ok {
						// only the core types are declared precisely: elsewhere the .awi's target is
						// loose (stock "camo" fields name a weaponcamotable where it says weaponcamo)
						level := "warning"
						if coreTypes[strings.ToLower(r.Type)] {
							level = "error"
						}
						rep.Issues = append(rep.Issues, Issue{level, r.Field, fmt.Sprintf("%q is a %s, but this field expects a %s", r.Target, locs[0].Type, r.Type)})
					}
				}
			}
			for _, fr := range w.FileRefs(typ, fields) {
				if !fr.Exists {
					if _, known := fileFields[typ][fr.Field]; known {
						rep.Issues = append(rep.Issues, Issue{"error", fr.Field, fmt.Sprintf("source file %s does not exist", fr.Path)})
					} else { // FX, surface FX, collision maps: stock ones ship compiled, not on disk
						rep.Issues = append(rep.Issues, Issue{"warning", fr.Field, fmt.Sprintf("%s is not on disk — fine only if it ships in a stock fastfile", fr.Path)})
					}
				}
			}
		}
		for _, is := range rep.Issues {
			if is.Level == "error" {
				res.Errors++
			} else {
				res.Warnings++
			}
		}
		if len(rep.Issues) > 0 {
			res.Assets = append(res.Assets, rep)
		}
	}
	if asset != "" && res.Checked == 0 {
		return nil, fmt.Errorf("asset %q is not in %s", asset, res.File)
	}
	return res, nil
}

// RefHit is one place that references an asset.
type RefHit struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Asset string `json:"asset"`
	Type  string `json:"type,omitempty"`
	Field string `json:"field"` // "[parent]" for a derived asset
}

// ReferencedBy finds every asset whose field value (or parent) is name, across
// all indexed GDTs — what breaks if name is renamed or deleted. Source files
// (models' material names inside .xmodel_bin) are not searched.
func (w *Workspace) ReferencedBy(name string) ([]RefHit, error) {
	w.scan()
	if w.scErr != nil {
		return nil, w.scErr
	}
	w.idx.mu.RLock()
	files := make([]string, 0, len(w.idx.files))
	for rel := range w.idx.files {
		files = append(files, rel)
	}
	w.idx.mu.RUnlock()

	needle := []byte(`"` + Quote(name) + `"`)
	var mu sync.Mutex
	var out []RefHit
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range ch {
				b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
				if err != nil || !bytes.Contains(b, needle) {
					continue
				}
				f, err := Parse(b)
				if err != nil {
					continue
				}
				for _, a := range f.Assets {
					var hits []RefHit
					if a.Parent == name {
						hits = append(hits, RefHit{File: rel, Line: a.Line, Asset: a.Name, Field: "[parent]"})
					}
					for _, fl := range a.Fields {
						if Unquote(fl.Value) == name && a.Name != name {
							hits = append(hits, RefHit{File: rel, Line: a.Line, Asset: a.Name, Type: a.Type, Field: fl.Key})
						}
					}
					if len(hits) > 0 {
						mu.Lock()
						out = append(out, hits...)
						mu.Unlock()
					}
				}
			}
		}()
	}
	for _, rel := range files {
		ch <- rel
	}
	close(ch)
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}
