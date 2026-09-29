package gdt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Issue is one validation finding. Errors block a write; warnings don't.
type Issue struct {
	Level string `json:"level"` // "error" | "warning"
	Field string `json:"field,omitempty"`
	Msg   string `json:"message"`
}

// EditRequest creates or updates one asset in one GDT.
type EditRequest struct {
	File     string            // GDT path, relative to the root (created if missing)
	Asset    string            // asset name
	Type     string            // asset type when creating a full asset ("material", "xmodel", …)
	Parent   string            // create as a derived asset: "Asset" [ "Parent" ]
	CopyFrom string            // create by copying another asset's type and fields
	Set      map[string]string // real (unescaped) values
	Unset    []string
	DryRun   bool
}

// EditResult reports what an edit did or would do.
type EditResult struct {
	File    string   `json:"file"`
	Asset   string   `json:"asset"`
	Type    string   `json:"type"`
	Created bool     `json:"created"`
	Changes []string `json:"changes"`
	Issues  []Issue  `json:"issues"`
	Written bool     `json:"written"`
	Next    []string `json:"next,omitempty"`
}

// lodFields are the xmodel fields a copied donor drags along and that then point
// at the donor's meshes (the `Part … in lower lod … doesn't have the same name` link error).
var lodFields = []string{"mediumLod", "lowLod", "lowestLod", "lod4File", "lod5File", "lod6File", "lod7File"}

// Edit validates and (unless DryRun or an error was found) applies a request.
func (w *Workspace) Edit(req EditRequest) (*EditResult, error) {
	if !strings.EqualFold(filepath.Ext(req.File), ".gdt") {
		return nil, fmt.Errorf("file must be a .gdt: %q", req.File)
	}
	path := w.Abs(req.File)
	res := &EditResult{File: w.Rel(path), Asset: req.Asset}
	if w.IsStock(path) {
		return nil, fmt.Errorf("%s is a stock Treyarch GDT (listed in stock.gdtdef) — don't edit it; "+
			"create a derived asset in your own GDT instead (parent=%q)", res.File, req.Asset)
	}

	var f *File
	if _, err := os.Stat(path); err == nil {
		if f, err = ParseFile(path); err != nil {
			return nil, err
		}
	} else {
		f = &File{Path: path, CRLF: true}
	}

	a := f.Find(req.Asset)
	if a == nil {
		res.Created = true
		if locs, err := w.Find(req.Asset); err == nil && len(locs) > 0 {
			var where []string
			for _, l := range locs {
				where = append(where, fmt.Sprintf("%s:%d", l.File, l.Line))
			}
			return nil, fmt.Errorf("asset %q already exists in %s — a second definition is a `Duplicate asset` link error; "+
				"edit it there, or pick another name", req.Asset, strings.Join(where, ", "))
		}
		a = &Asset{Name: req.Asset}
		switch {
		case req.CopyFrom != "":
			donor, err := w.loadAsset(req.CopyFrom)
			if err != nil {
				return nil, err
			}
			typ, fields, err := w.Resolved(donor)
			if err != nil {
				return nil, err
			}
			a.Type, a.Fields = typ, append([]Field(nil), fields...)
			if typ == "xmodel" {
				for _, k := range lodFields {
					if v, ok := a.Get(k); ok && v != "" {
						a.Set(k, "")
						res.Issues = append(res.Issues, Issue{"warning", k, "cleared the donor's LOD path — set it only if you supply that LOD"})
					}
				}
			}
		case req.Parent != "":
			if _, err := w.loadAsset(req.Parent); err != nil {
				return nil, err
			}
			a.Parent = req.Parent
		case req.Type != "":
			a.Type = req.Type
		default:
			return nil, fmt.Errorf("asset %q doesn't exist in %s: pass type, parent, or copy_from to create it", req.Asset, res.File)
		}
		f.Add(a)
	}

	before := map[string]string{}
	for _, fl := range a.Fields {
		before[fl.Key] = fl.Value
	}
	keys := make([]string, 0, len(req.Set))
	for k := range req.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a.Set(k, Quote(req.Set[k]))
	}
	for _, k := range req.Unset {
		a.Unset(k)
	}

	typ, fields, err := w.Resolved(a)
	if err != nil {
		return nil, err
	}
	res.Type = typ
	res.Issues = append(res.Issues, w.validate(typ, fields, keys)...)
	if res.Issues == nil {
		res.Issues = []Issue{}
	}

	for _, fl := range a.Fields {
		old, had := before[fl.Key]
		switch {
		case !had && !res.Created:
			res.Changes = append(res.Changes, fmt.Sprintf("+ %s = \"%s\"", fl.Key, Unquote(fl.Value)))
		case had && old != fl.Value:
			res.Changes = append(res.Changes, fmt.Sprintf("~ %s: \"%s\" -> \"%s\"", fl.Key, Unquote(old), Unquote(fl.Value)))
		}
		delete(before, fl.Key)
	}
	for k := range before {
		res.Changes = append(res.Changes, "- "+k)
	}
	if res.Created {
		res.Changes = append([]string{fmt.Sprintf("+ asset %q (%d fields)", a.Name, len(a.Fields))}, res.Changes...)
	}

	blocked := false
	for _, is := range res.Issues {
		if is.Level == "error" {
			blocked = true
		}
	}
	if !blocked && !req.DryRun {
		if err := f.Save(); err != nil {
			return nil, err
		}
		w.Touched(path)
		res.Written = true
		res.Next = []string{
			"Build as usual: its `gdtdb /update` pass indexes the changed GDT (it should report processed (1 GDTs)); " +
				"only if it reports 0 GDTs and the linker then can't find the asset, rebuild with gdt_rebuild=true",
		}
		switch {
		case res.Created && typ == "material":
			res.Next = append(res.Next, "Don't zone the material on its own: it is built through what uses it — an xmodel "+
				"(as mc/<name>) or map geometry (as wc/<name>), whose techset variants are the precompiled ones. A bare "+
				"`material,<name>` line asks for an unprefixed variant that isn't in the shader cache and fails to compile")
		case res.Created && typ != "":
			res.Next = append(res.Next, fmt.Sprintf("Add `%s,%s` to the map/mod .zone (or zone something that references it) so the linker packs it", typ, a.Name))
		}
	}
	return res, nil
}

// loadAsset finds a single definition of name and returns it.
func (w *Workspace) loadAsset(name string) (*Asset, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("asset %q not found in any GDT under %s", name, w.Root)
	}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return nil, err
	}
	return f.Find(name), nil
}

// Validate checks an asset's effective fields against its schema.
func (w *Workspace) Validate(a *Asset) ([]Issue, string, error) {
	typ, fields, err := w.Resolved(a)
	if err != nil {
		return nil, "", err
	}
	var keys []string
	for _, f := range fields {
		keys = append(keys, f.Key)
	}
	iss := w.validate(typ, fields, keys)
	if iss == nil {
		iss = []Issue{}
	}
	return iss, typ, nil
}

// validate checks the named keys (the ones being set) against the deffile, and a
// material's texture fields against what its techset exposes.
func (w *Workspace) validate(typ string, fields []Field, keys []string) []Issue {
	var out []Issue
	if typ == "" {
		return append(out, Issue{"warning", "", "asset type unknown (derived from a parent that wasn't found) — fields not validated"})
	}
	sc, err := w.Schema(typ)
	if err != nil {
		return append(out, Issue{"error", "", err.Error()})
	}
	val := map[string]string{}
	for _, f := range fields {
		val[f.Key] = Unquote(f.Value)
	}
	for _, k := range keys {
		v := val[k]
		e := sc.Lookup(k)
		if e == nil {
			out = append(out, Issue{"warning", k, fmt.Sprintf("not declared in %s.awi — APE won't show it; check the spelling", typ)})
			continue
		}
		switch e.Kind {
		case "Float", "Int":
			n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not a number", v)})
				break
			}
			if e.Kind == "Int" && n != float64(int64(n)) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not an integer", v)})
			}
			if e.Min != nil && e.Max != nil && *e.Min < *e.Max && (n < *e.Min || n > *e.Max) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%v is outside %v..%v", n, *e.Min, *e.Max)})
			}
		case "CheckBox":
			if v != "0" && v != "1" {
				out = append(out, Issue{"error", k, fmt.Sprintf("checkbox wants 0 or 1, got %q", v)})
			}
		case "Combo":
			if len(e.Options) > 1 && v != "" && !contains(e.Options, v) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not one of: %s", v, strings.Join(e.Options, " | "))})
			}
		case "AssetCombo":
			if v != "" {
				if locs, err := w.Find(v); err == nil && len(locs) == 0 {
					out = append(out, Issue{"warning", k, fmt.Sprintf("no %s asset named %q in any GDT (fine if it ships in a stock fastfile)", e.AssetType, v)})
				}
			}
		}
	}
	if typ == "material" {
		only := map[string]bool{}
		for _, k := range keys {
			only[k] = true
		}
		out = append(out, w.validateMaterial(val, only)...)
	}
	return out
}

// validateMaterial checks the material type, its category, and — for the keys in
// only — image fields the techset doesn't read and image semantic mismatches.
func (w *Workspace) validateMaterial(val map[string]string, only map[string]bool) []Issue {
	var out []Issue
	mt := val["materialType"]
	if mt == "" {
		return out
	}
	ts, err := w.Techsets.Resolve(mt)
	if err != nil {
		return append(out, Issue{"error", "materialType", err.Error() + " — APE shows INVALID MATERIAL TYPE"})
	}
	if cat := val["materialCategory"]; ts.Category != "" && cat != "" && !strings.EqualFold(cat, ts.Category) {
		out = append(out, Issue{"error", "materialCategory", fmt.Sprintf("%q but materialType %q is category %q — they must agree", cat, mt, ts.Category)})
	}
	exposed := ts.ExposedFields()
	for k, v := range val {
		if v == "" || !isImageField(k) || !only[k] {
			continue
		}
		if !exposed[k] {
			out = append(out, Issue{"warning", k, fmt.Sprintf("materialType %q doesn't read %s — the image is ignored (or the link fails with `doesn't expose a '%s' texture`)", mt, k, k)})
		}
	}
	for _, s := range ts.Textures {
		img := val[s.Field]
		if img == "" || s.Semantic == "" || !only[s.Field] {
			continue
		}
		if ia, err := w.loadAsset(img); err == nil && ia != nil {
			if sem, ok := ia.Get("semantic"); ok && sem != "" && !strings.EqualFold(Unquote(sem), s.Semantic) {
				out = append(out, Issue{"warning", s.Field, fmt.Sprintf("image %q has semantic %q but slot %s expects %q (APE: type mismatch)", img, Unquote(sem), s.Name, s.Semantic)})
			}
		}
	}
	return out
}

// isImageField recognises the material fields that hold image asset names.
func isImageField(k string) bool {
	lk := strings.ToLower(k)
	return strings.HasSuffix(lk, "map") || strings.HasPrefix(lk, "colormap")
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
