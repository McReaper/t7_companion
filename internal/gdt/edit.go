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
	Image    *ImageSpec // create an image asset from a texture file (see ImageSpec)
	DryRun   bool
}

// ImageSpec creates an image asset from a texture: the semantic comes from the
// slot it will fill (a material type's techset + field) or is given, and the
// other fields are copied from a stock image of that semantic.
type ImageSpec struct {
	Texture      string // texture file, relative to the root (e.g. texture_assets/my/wall_c.tif)
	Semantic     string // diffuseMap, normalMap, … — or derive it from the two fields below
	MaterialType string // techset the image is for, e.g. lit
	Field        string // material field it will fill, e.g. normalMap
}

// EditResult reports what an edit did or would do.
type EditResult struct {
	File    string   `json:"file,omitempty"` // empty inside a batch: the batch names it once
	Asset   string   `json:"asset"`
	Type    string   `json:"type"`
	Created bool     `json:"created"`
	Changes []string `json:"changes"`
	Issues  []Issue  `json:"issues"`
	Written bool     `json:"written"`
	Next    []string `json:"next,omitempty"`
}

// BatchResult reports a multi-asset edit of one GDT: all-or-nothing.
type BatchResult struct {
	File    string        `json:"file"`
	Errors  int           `json:"errors"`
	Written bool          `json:"written"`
	Results []*EditResult `json:"results"`
	Next    []string      `json:"next,omitempty"`
}

// lodFields are the xmodel fields a copied donor drags along and that then point
// at the donor's meshes (the `Part … in lower lod … doesn't have the same name` link error).
var lodFields = []string{"mediumLod", "lowLod", "lowestLod", "lod4File", "lod5File", "lod6File", "lod7File"}

// Edit validates and (unless DryRun or an error was found) applies one request.
func (w *Workspace) Edit(req EditRequest) (*EditResult, error) {
	br, err := w.EditBatch(req.File, []EditRequest{req}, req.DryRun)
	if err != nil {
		return nil, err
	}
	r := br.Results[0]
	r.File, r.Written, r.Next = br.File, br.Written, br.Next
	return r, nil
}

// EditBatch applies several requests to one GDT in memory, validates each, and
// writes once — only if none has an error (and not a dry run). Later items see
// earlier ones, so a batch can create a material and then a derived one.
func (w *Workspace) EditBatch(file string, reqs []EditRequest, dryRun bool) (*BatchResult, error) {
	if !strings.EqualFold(filepath.Ext(file), ".gdt") {
		return nil, fmt.Errorf("file must be a .gdt: %q", file)
	}
	if len(reqs) == 0 {
		return nil, fmt.Errorf("nothing to edit")
	}
	path := w.Abs(file)
	br := &BatchResult{File: w.Rel(path)}
	if w.IsStock(path) {
		return nil, fmt.Errorf("%s is a stock Treyarch GDT (listed in stock.gdtdef) — don't edit it; "+
			"copy_from the asset into your own GDT instead (a parent must be in the same GDT, so you can't derive from a stock one)", br.File)
	}
	var f *File
	if _, err := os.Stat(path); err == nil {
		if f, err = ParseFile(path); err != nil {
			return nil, err
		}
	} else {
		f = &File{Path: path, CRLF: true}
	}
	var created []*EditResult // for the next-step hints
	for _, req := range reqs {
		res, err := w.applyEdit(f, req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", req.Asset, err)
		}
		res.File = ""
		if res.Created {
			created = append(created, res)
		}
		for _, is := range res.Issues {
			if is.Level == "error" {
				br.Errors++
			}
		}
		br.Results = append(br.Results, res)
	}
	if br.Errors == 0 && !dryRun {
		if err := f.Save(); err != nil {
			return nil, err
		}
		w.Touched(path)
		br.Written = true
		for _, r := range br.Results {
			r.Written = true
		}
		br.Next = []string{
			"Build as usual: its `gdtdb /update` pass indexes the changed GDT (it should report processed (1 GDTs)); " +
				"only if it reports 0 GDTs and the linker then can't find the asset, rebuild with gdt_rebuild=true",
		}
		for _, c := range created {
			name, typ := c.Asset, c.Type
			switch typ {
			case "material":
				br.Next = append(br.Next, fmt.Sprintf("Don't zone material %q on its own: it is built through what uses it — an xmodel "+
					"(as mc/<name>) or map geometry (as wc/<name>), whose techset variants are the precompiled ones. A bare "+
					"`material,<name>` line asks for an unprefixed variant that isn't in the shader cache and fails to compile", name))
			case "image":
				// pulled in by the material that uses it
			case "":
			default:
				br.Next = append(br.Next, fmt.Sprintf("Add `%s,%s` to the map/mod .zone (or zone something that references it) so the linker packs it", typ, name))
			}
		}
	}
	return br, nil
}

// applyEdit applies one request to an in-memory file and validates the result.
func (w *Workspace) applyEdit(f *File, req EditRequest) (*EditResult, error) {
	res := &EditResult{File: w.Rel(f.Path), Asset: req.Asset}
	if req.Asset == "" {
		return nil, fmt.Errorf("asset name is required")
	}
	a := f.Find(req.Asset)
	imageNote := ""
	if a == nil {
		res.Created = true
		a = &Asset{Name: req.Asset}
		switch {
		case req.Image != nil:
			fields, issues, note, err := w.imageFields(req.Image)
			if err != nil {
				return nil, err
			}
			a.Type, a.Fields = "image", fields
			res.Issues = append(res.Issues, issues...)
			imageNote = note
		case req.CopyFrom != "":
			df, donor, err := w.assetIn(f, req.CopyFrom)
			if err != nil {
				return nil, err
			}
			typ, fields, err := w.Resolved(df, donor)
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
			// gdtdb only resolves a parent inside the same GDT
			if f.Find(req.Parent) == nil {
				where := "no GDT"
				if locs, _ := w.Find(req.Parent); len(locs) > 0 {
					where = locs[0].File
				}
				return nil, fmt.Errorf("parent %q is not in %s (it's in %s): a derived asset's parent must be in the same GDT, "+
					"or gdtdb fails with `Parent Entity '%s' does not exist in GDT` — use copy_from instead, or create it in that GDT",
					req.Parent, res.File, where, req.Parent)
			}
			a.Parent = req.Parent
		case req.Type != "":
			a.Type = req.Type
		default:
			return nil, fmt.Errorf("asset %q doesn't exist in %s: pass type, parent, copy_from or image to create it", req.Asset, res.File)
		}
		// Names are per type: an image and a material may share one.
		newType, _, err := w.Resolved(f, a)
		if err != nil {
			return nil, err
		}
		if locs, err := w.FindTyped(req.Asset, newType); err == nil && len(locs) > 0 {
			var where []string
			for _, l := range locs {
				where = append(where, fmt.Sprintf("%s:%d", l.File, l.Line))
			}
			return nil, fmt.Errorf("%s %q already exists in %s — a second definition is a `Duplicate '%s' asset` link error; "+
				"edit it there, or pick another name", newType, req.Asset, strings.Join(where, ", "), newType)
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

	typ, fields, err := w.Resolved(f, a)
	if err != nil {
		return nil, err
	}
	res.Type = typ
	res.Issues = append(res.Issues, w.validate(f, typ, fields, keys)...)
	for _, fr := range w.FileRefs(typ, fields) {
		changed := false
		for _, k := range keys {
			changed = changed || k == fr.Field
		}
		if !fr.Exists && (changed || res.Created) {
			res.Issues = append(res.Issues, Issue{"warning", fr.Field, fmt.Sprintf("source file %s does not exist yet", fr.Path)})
		}
	}
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
		head := []string{fmt.Sprintf("+ asset %q (%d fields)", a.Name, len(a.Fields))}
		if imageNote != "" {
			head = append(head, imageNote)
		}
		res.Changes = append(head, res.Changes...)
	}
	return res, nil
}

// assetIn finds an asset (and its file) in the file being edited first, then in the workspace.
func (w *Workspace) assetIn(f *File, name string) (*File, *Asset, error) {
	if a := f.Find(name); a != nil {
		return f, a, nil
	}
	return w.loadAsset(name)
}

// loadAsset finds a single definition of name and returns it.
func (w *Workspace) loadAsset(name string) (*File, *Asset, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, nil, err
	}
	if len(locs) == 0 {
		return nil, nil, fmt.Errorf("asset %q not found in any GDT under %s", name, w.Root)
	}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Find(name), nil
}

// Validate checks an asset's effective fields against its schema.
func (w *Workspace) Validate(f *File, a *Asset) ([]Issue, string, error) {
	typ, fields, err := w.Resolved(f, a)
	if err != nil {
		return nil, "", err
	}
	var keys []string
	for _, f := range fields {
		keys = append(keys, f.Key)
	}
	iss := w.validate(nil, typ, fields, keys)
	if iss == nil {
		iss = []Issue{}
	}
	return iss, typ, nil
}

// validate checks the named keys (the ones being set) against the deffile, and a
// material's texture fields against what its techset exposes. Referenced assets
// are looked up in local (the file being edited, maybe unsaved) before the index.
func (w *Workspace) validate(local *File, typ string, fields []Field, keys []string) []Issue {
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
		if v == "" || e.Varies {
			continue // empty = the default; a Varies field's kind depends on script state
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
				// a warning: the .awi range is the slider's, and stock GDTs go past it (and link)
				out = append(out, Issue{"warning", k, fmt.Sprintf("%v is outside APE's slider range %v..%v", n, *e.Min, *e.Max)})
			}
		case "Color", "Vector":
			parts := strings.Fields(v)
			want := len(strings.Fields(e.Default))
			bad := want > 0 && len(parts) != want
			for _, p := range parts {
				if _, err := strconv.ParseFloat(p, 64); err != nil {
					bad = true
				}
			}
			if bad {
				n := want
				if n == 0 {
					n = 3
				}
				out = append(out, Issue{"error", k, fmt.Sprintf("%q: expected %d space-separated numbers (e.g. %q)", v, n, strings.TrimSpace(strings.Repeat("1 ", n)))})
			}
		case "CheckBox":
			// converter-written GDTs also store "True"/"False", which link
			if v != "0" && v != "1" && !strings.EqualFold(v, "true") && !strings.EqualFold(v, "false") {
				out = append(out, Issue{"error", k, fmt.Sprintf("checkbox wants 0 or 1, got %q", v)})
			}
		case "Combo":
			// "<none>*": converter-written GDTs keep the .awi's default marker on the value;
			// APE compares choices case-insensitively ("stand" for "Stand")
			if len(e.Options) > 1 && !optionMatch(e.Options, strings.TrimSuffix(v, "*")) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not one of: %s", v, strings.Join(e.Options, " | "))})
			}
		case "AssetCombo":
			if local == nil || local.Find(v) == nil {
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
		out = append(out, w.validateMaterial(local, val, only)...)
	}
	return out
}

// validateMaterial checks the material type, its category, and — for the keys in
// only — image fields the techset doesn't read and image semantic mismatches.
func (w *Workspace) validateMaterial(local *File, val map[string]string, only map[string]bool) []Issue {
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
		var ia *Asset
		if local != nil {
			ia = local.Find(img)
		}
		if ia == nil {
			_, ia, _ = w.loadAsset(img)
		}
		if ia != nil {
			if sem, ok := ia.Get("semantic"); ok && sem != "" && !strings.EqualFold(Unquote(sem), s.Semantic) {
				out = append(out, Issue{"warning", s.Field, fmt.Sprintf("image %q has semantic %q but slot %s expects %q (APE: type mismatch)", img, Unquote(sem), s.Name, s.Semantic)})
			}
		}
	}
	return out
}

// optionMatch accepts a combo value by label or, for "Label{value}" options
// ("PitchVelocity{0}", "custom{-1}"), by the value in braces, ignoring case.
func optionMatch(opts []string, v string) bool {
	for _, o := range opts {
		label, inner, braced := strings.Cut(o, "{")
		if strings.EqualFold(o, v) || strings.EqualFold(strings.TrimSpace(label), v) ||
			(braced && strings.TrimSpace(strings.TrimSuffix(inner, "}")) == v) {
			return true
		}
	}
	return false
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
