package gdt

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
	path := filepath.Clean(w.Abs(file))
	br := &BatchResult{File: w.Rel(path)}
	if !w.InGDTDirs(path) {
		return nil, fmt.Errorf("%s is outside the directories gdtdb indexes (%s, from bin/converter_gdt_dirs_0.txt) — it would never be built",
			br.File, strings.Join(GDTDirs(w.Root), ", "))
	}
	if w.IsStock(path) {
		return nil, fmt.Errorf("%s is a stock Treyarch GDT (listed in stock.gdtdef) — don't edit it; "+
			"copy_from the asset into your own GDT instead (a parent must be in the same GDT, so you can't derive from a stock one)", br.File)
	}
	unlock := w.lockFile(path)
	defer unlock()
	var f *File
	before, statErr := os.Stat(path)
	if statErr == nil {
		var err error
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
		// APE (or anything else) may have saved the file since we read it.
		now, err := os.Stat(path)
		changed := (statErr == nil) != (err == nil) ||
			(err == nil && (now.ModTime() != before.ModTime() || now.Size() != before.Size()))
		if changed {
			return nil, fmt.Errorf("%s changed on disk while it was being edited (saved in APE?) — nothing written; run the edit again", br.File)
		}
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
	if err := ValidName("asset name", req.Asset); err != nil {
		return nil, err
	}
	for what, v := range map[string]string{"parent": req.Parent, "type": req.Type, "copy_from": req.CopyFrom} {
		if v != "" {
			if err := ValidName(what, v); err != nil {
				return nil, err
			}
		}
	}
	for k := range req.Set {
		if err := ValidName("field key", k); err != nil {
			return nil, err
		}
	}
	a, err := w.target(f, req)
	if err != nil {
		return nil, err
	}
	imageNote := ""
	if a == nil {
		res.Created = true
		var issues []Issue
		if a, issues, imageNote, err = w.newAsset(f, req); err != nil {
			return nil, err
		}
		res.Issues = append(res.Issues, issues...)
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
			res.Issues = append(res.Issues, Issue{"warning", fr.Field, fmt.Sprintf("source file %s does not exist yet", fr.Path), ""})
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

// newAsset builds the asset a request creates — from a texture (image), a donor
// (copy_from), a parent in the same GDT, or a bare type — and refuses a second
// asset of the same type and name. It returns issues found on the way and, for an
// image, a note on where its settings came from.
func (w *Workspace) newAsset(f *File, req EditRequest) (a *Asset, out []Issue, note string, err error) {
	a = &Asset{Name: req.Asset}
	switch {
	case req.Image != nil:
		fields, issues, n, err := w.imageFields(req.Image)
		if err != nil {
			return nil, nil, "", err
		}
		a.Type, a.Fields = "image", fields
		out, note = issues, n
	case req.CopyFrom != "":
		df, donor, err := w.assetIn(f, req.CopyFrom, req.Type)
		if err != nil {
			return nil, nil, "", err
		}
		typ, fields, err := w.Resolved(df, donor)
		if err != nil {
			return nil, nil, "", err
		}
		a.Type, a.Fields = typ, append([]Field(nil), fields...)
		if typ == "xmodel" {
			for _, k := range lodFields {
				if v, ok := a.Get(k); ok && v != "" {
					a.Set(k, "")
					out = append(out, Issue{"warning", k, "cleared the donor's LOD path — set it only if you supply that LOD", ""})
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
			return nil, nil, "", fmt.Errorf("parent %q is not in %s (it's in %s): a derived asset's parent must be in the same GDT, "+
				"or gdtdb fails with `Parent Entity '%s' does not exist in GDT` — use copy_from instead, or create it in that GDT",
				req.Parent, w.Rel(f.Path), where, req.Parent)
		}
		a.Parent = req.Parent
	case req.Type != "":
		a.Type = req.Type
	default:
		return nil, nil, "", fmt.Errorf("asset %q doesn't exist in %s: pass type, parent, copy_from or image to create it", req.Asset, w.Rel(f.Path))
	}
	// Names are per type: an image and a material may share one.
	newType, _, err := w.Resolved(f, a)
	if err != nil {
		return nil, nil, "", err
	}
	if locs, err := w.FindTyped(req.Asset, newType); err == nil && len(locs) > 0 {
		var where []string
		for _, l := range locs {
			where = append(where, fmt.Sprintf("%s:%d", l.File, l.Line))
		}
		return nil, nil, "", fmt.Errorf("%s %q already exists in %s — a second definition is a `Duplicate '%s' asset` link error; "+
			"edit it there, or pick another name", newType, req.Asset, strings.Join(where, ", "), newType)
	}
	return a, out, note, nil
}

// target returns the existing asset a request edits, or nil to create one. Names
// are per type, so when the GDT holds several assets of that name the request's
// type (or image / parent) picks one, and without it the request is ambiguous.
func (w *Workspace) target(f *File, req EditRequest) (*Asset, error) {
	all := f.FindAll(req.Asset)
	if len(all) == 0 {
		return nil, nil
	}
	want := req.Type
	switch {
	case req.Image != nil:
		want = "image"
	case want == "" && req.Parent != "":
		if p := f.Find(req.Parent); p != nil {
			want, _, _ = w.Resolved(f, p)
		}
	}
	var match []*Asset
	var types []string
	for _, a := range all {
		t, _, _ := w.Resolved(f, a)
		types = append(types, t)
		if want == "" || strings.EqualFold(t, want) {
			match = append(match, a)
		}
	}
	switch {
	case len(match) == 1:
		return match[0], nil
	case len(match) == 0:
		return nil, nil // a new asset of another type sharing the name
	default:
		return nil, fmt.Errorf("%s holds several assets named %q (%s): pass type to say which", w.Rel(f.Path), req.Asset, strings.Join(types, ", "))
	}
}

// assetIn finds an asset (and its file) in the file being edited first, then in
// the workspace; typ, when known, disambiguates same-name assets of other types.
func (w *Workspace) assetIn(f *File, name, typ string) (*File, *Asset, error) {
	for _, a := range f.FindAll(name) {
		if t, _, _ := w.Resolved(f, a); typ == "" || strings.EqualFold(t, typ) {
			return f, a, nil
		}
	}
	return w.loadAsset(name, typ)
}

// loadAsset finds one definition of name (of type typ, if given) across the
// workspace. Several definitions of different types without typ are ambiguous.
func (w *Workspace) loadAsset(name, typ string) (*File, *Asset, error) {
	var locs []Location
	var err error
	if typ != "" {
		locs, err = w.FindTyped(name, typ)
	} else {
		locs, err = w.Find(name)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(locs) == 0 {
		return nil, nil, fmt.Errorf("asset %q not found in any GDT under %s", name, w.Root)
	}
	if typ == "" {
		seen := map[string]bool{}
		for i := range locs {
			locs[i].Type = w.TypeOf(locs[i])
			seen[locs[i].Type] = true
		}
		if len(seen) > 1 {
			return nil, nil, fmt.Errorf("several assets are named %q (%d types) — say which type", name, len(seen))
		}
	}
	f, err := w.Load(locs[0].File)
	if err != nil {
		return nil, nil, err
	}
	a := f.AtLine(locs[0].Line)
	if a == nil || a.Name != name {
		return nil, nil, fmt.Errorf("asset %q is indexed at %s:%d but isn't there any more — the GDT changed; retry", name, locs[0].File, locs[0].Line)
	}
	return f, a, nil
}
