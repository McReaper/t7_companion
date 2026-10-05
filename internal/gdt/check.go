package gdt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// Refs returns the typed asset references among an asset's effective fields,
// engine built-ins ($white_diffuse…) included: the linker packs those too.
func (w *Workspace) Refs(typ string, fields []Field) []Ref {
	sc, err := w.Schema(typ)
	if err != nil {
		return nil
	}
	var out []Ref
	for _, f := range fields {
		v := Unquote(f.Value)
		if v == "" {
			continue
		}
		e := sc.Lookup(f.Key)
		if e == nil {
			continue
		}
		// Texture entries are not references: image.awi uses them for the source
		// file (baseImage) and composite channel names — FileRefs covers baseImage.
		if e.Kind == "AssetCombo" && e.AssetType != "" && !e.Varies && inUse(e, f.Key, fields) {
			out = append(out, Ref{Field: f.Key, Target: v, Type: e.AssetType})
		}
	}
	return out
}

// inUse: an item list's entry counts only up to the list's count (APE keeps
// removed items' values in hidden fields, which nothing reads).
func inUse(e *Entry, key string, fields []Field) bool {
	if e.Count == "" {
		return true
	}
	n, err := strconv.Atoi(strings.TrimLeft(key[len(strings.TrimRight(key, "0123456789")):], "0"))
	if err != nil {
		return false
	}
	for _, f := range fields {
		if strings.EqualFold(f.Key, e.Count) {
			count, err := strconv.Atoi(strings.TrimSpace(Unquote(f.Value)))
			return err == nil && n <= count
		}
	}
	return false
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
//
// What it calls an error follows one rule: anything Treyarch's own stock GDTs do
// while still linking is not an error (TestCalibrate tallies issues over a whole
// install; diff its report before and after changing a rule). The rules:
//
//   - names are per type (an image and a material may share one) — only two
//     definitions of the same resolved type are Duplicate '<type>' asset
//     (Workspace.FindTyped/Duplicates; a derived asset takes its parent's type)
//   - a runtime-built field name matches on all its literal parts ("autogenLod" + i vs
//     "autogenLod" + i + "Percent")
//   - a field redeclared in another script branch with another kind or target is
//     Varies and not type-checked
//   - field names and combo values match case-insensitively, Label{value} options by
//     either part, and the .awi default marker ("<none>*"), True/False checkboxes and
//     empty values count as the default
//   - .awi min/max are slider bounds (outside = warning)
//   - a combo value any stock GDT uses for that field is accepted even when the .awi
//     dropped it (stockUses, scanned lazily once per type, combo fields only)
//   - AddEntry_Texture is a source path, not an asset reference
//   - a target-type mismatch is an error only for image/material/xmodel/xanim
//   - a missing file is an error only for xmodel LODs, xanim exports and image
//     textures (FX/surface-FX/collision paths ship compiled)
//   - stale texture slots a techset doesn't read are skipped
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
		rep := w.checkAsset(f, a)
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

// checkAsset runs every diagnostic over one asset of f.
func (w *Workspace) checkAsset(f *File, a *Asset) AssetReport {
	rep := AssetReport{Asset: a.Name, Line: a.Line}
	rep.Issues = append(rep.Issues, w.parentIssues(f, a)...)
	typ, fields, err := w.Resolved(f, a)
	if err != nil { // a derivation cycle: report it on this asset and keep checking the rest
		rep.Issues = append(rep.Issues, Issue{"error", "", err.Error() + " — gdtdb can't resolve this asset", ""})
		typ, fields = "", nil
	}
	rep.Type = typ
	if typ == "" {
		return rep
	}
	rep.Issues = append(rep.Issues, w.duplicateIssues(a.Name, typ)...)
	rep.Issues = append(rep.Issues, w.schemaIssues(f, typ, fields)...)
	for _, r := range w.Refs(typ, fields) {
		if strings.HasPrefix(r.Target, "$") { // an engine built-in: no GDT defines it
			continue
		}
		rep.Issues = append(rep.Issues, w.refIssues(r)...)
	}
	rep.Issues = append(rep.Issues, w.fileIssues(typ, fields)...)
	return rep
}

// parentIssues: a derived asset's parent must be in the same GDT.
func (w *Workspace) parentIssues(f *File, a *Asset) []Issue {
	if a.Parent == "" || f.Find(a.Parent) != nil {
		return nil
	}
	where := "is in no GDT"
	if locs, _ := w.Find(a.Parent); len(locs) > 0 {
		where = "is only in " + locs[0].File
	}
	return []Issue{{"error", "", fmt.Sprintf("parent %q %s, not this GDT — gdtdb: `Parent Entity '%s' does not exist in GDT`", a.Parent, where, a.Parent), ""}}
}

// duplicateIssues: two definitions of the same resolved type fail the link.
func (w *Workspace) duplicateIssues(name, typ string) []Issue {
	locs, _ := w.FindTyped(name, typ)
	if len(locs) < 2 {
		return nil
	}
	var where []string
	for _, l := range locs {
		where = append(where, fmt.Sprintf("%s:%d", l.File, l.Line))
	}
	return []Issue{{"error", "", fmt.Sprintf("%s defined more than once (Duplicate '%s' asset at link): %s", typ, typ, strings.Join(where, ", ")), ""}}
}

// schemaIssues validates every field, minus the kinds gdt_check doesn't report.
func (w *Workspace) schemaIssues(f *File, typ string, fields []Field) []Issue {
	keys := make([]string, 0, len(fields))
	for _, fl := range fields {
		keys = append(keys, fl.Key)
	}
	var out []Issue
	for _, is := range w.validate(f, typ, fields, keys) {
		switch is.Code {
		case codeUndeclared: // script-built fields APE wrote; only new keys get this warning (gdt_edit)
		case codeNoAsset: // reported by refIssues with the expected type
		case codeStaleSlot: // left by an earlier materialType: Treyarch's own GDTs keep ~40k of these and link
		default:
			out = append(out, is)
		}
	}
	return out
}

// refIssues: a typed reference must name an asset, of the type the field expects.
func (w *Workspace) refIssues(r Ref) []Issue {
	locs, _ := w.Find(r.Target)
	if len(locs) == 0 {
		return []Issue{{"warning", r.Field, fmt.Sprintf("%s %q is in no GDT — fine only if it ships in a stock fastfile", r.Type, r.Target), ""}}
	}
	found := ""
	for _, l := range locs {
		lt := w.TypeOf(l) // a derived target takes its parent's type
		if lt == "" || typeIn(lt, r.Type) {
			return nil // unresolvable (don't guess), or the right type
		}
		if found == "" {
			found = lt
		}
	}
	// only the core types are declared precisely: elsewhere the .awi's target is
	// loose (stock "camo" fields name a weaponcamotable where it says weaponcamo)
	level := "warning"
	if coreTypes[strings.ToLower(r.Type)] {
		level = "error"
	}
	return []Issue{{level, r.Field, fmt.Sprintf("%q is a %s, but this field expects a %s", r.Target, found, r.Type), ""}}
}

// typeIn reports whether typ is one of the .awi's "xmodel | character | aitype".
func typeIn(typ, wanted string) bool {
	for _, want := range strings.Split(wanted, "|") {
		if strings.EqualFold(typ, strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// fileIssues: source files must be on disk — an error only for the ones gdt
// knows are needed (LODs, xanim exports, image textures).
func (w *Workspace) fileIssues(typ string, fields []Field) []Issue {
	var out []Issue
	for _, fr := range w.FileRefs(typ, fields) {
		switch _, known := fileFields[typ][fr.Field]; {
		case fr.Exists:
		case known:
			out = append(out, Issue{"error", fr.Field, fmt.Sprintf("source file %s does not exist", fr.Path), ""})
		default: // FX, surface FX, collision maps: stock ones ship compiled, not on disk
			out = append(out, Issue{"warning", fr.Field, fmt.Sprintf("%s is not on disk — fine only if it ships in a stock fastfile", fr.Path), ""})
		}
	}
	return out
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
// all indexed GDTs — what breaks if name is renamed or deleted. It also returns
// the GDTs it couldn't read or parse, so a silent miss can't pass for "unused".
// Source files (models' material names inside .xmodel_bin) are not searched.
func (w *Workspace) ReferencedBy(name string) ([]RefHit, []string, error) {
	files, err := w.indexedFiles(nil)
	if err != nil {
		return nil, nil, err
	}
	needle := []byte(Quote(name)) // unquoted: list fields (skinOverride) hold several names in one value
	var mu sync.Mutex
	var out []RefHit
	var bad []string
	w.readEach(files, func(rel string, b []byte, err error) {
		if err == nil && !bytes.Contains(b, needle) {
			return
		}
		var f *File
		if err == nil {
			f, err = Parse(b)
		}
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", rel, err))
			return
		}
		out = append(out, refsIn(f, rel, name)...)
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	sort.Strings(bad)
	return out, bad, nil
}

// refsIn lists the assets of f that derive from name or have a field naming it:
// the whole value, or one entry of a list value (an xmodel's skinOverride).
func refsIn(f *File, rel, name string) []RefHit {
	var hits []RefHit
	for _, a := range f.Assets {
		if a.Parent == name {
			hits = append(hits, RefHit{File: rel, Line: a.Line, Asset: a.Name, Field: "[parent]"})
		}
		for _, fl := range a.Fields {
			// no self-skip: a material "clip" whose colorMap is the image "clip" is a real reference
			if v := Unquote(fl.Value); v == name || listHas(v, name) {
				hits = append(hits, RefHit{File: rel, Line: a.Line, Asset: a.Name, Type: a.Type, Field: fl.Key})
			}
		}
	}
	return hits
}

// listSep separates the lines of a GDT list value: the escape text \r\n, not a
// line break (`"skinOverride" "mtl_a mtl_b\r\n"`).
const listSep = `\r\n`

// listHas reports whether a list value — lines separated by listSep, each
// holding space-separated names — has name as one entry.
func listHas(v, name string) bool {
	if !strings.Contains(v, listSep) {
		return false
	}
	for _, e := range strings.Fields(strings.ReplaceAll(v, listSep, " ")) {
		if e == name {
			return true
		}
	}
	return false
}
