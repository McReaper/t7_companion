package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// GDT authoring: find/read/validate/edit the text databases APE edits, using the
// install's own deffiles (*.awi) and techsetdefs as the schema. Exposed both as
// `t7kb gdt …` and as the gdt_* MCP tools, with identical behaviour.

type gdtAssetView struct {
	Asset     string            `json:"asset"`
	File      string            `json:"file"`
	Line      int               `json:"line"`
	Type      string            `json:"type"`
	Parent    string            `json:"parent,omitempty"`
	Stock     bool              `json:"stock"`
	Own       map[string]string `json:"own_fields"`
	Inherited map[string]string `json:"inherited_fields,omitempty"`
	Hidden    string            `json:"hidden,omitempty"`
	Issues    []gdt.Issue       `json:"issues,omitempty"`
	Others    []gdt.Location    `json:"other_definitions,omitempty"`
}

func gdtFind(w *gdt.Workspace, name string) (any, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	for i := range locs {
		locs[i].Type = w.TypeOf(locs[i])
	}
	out := map[string]any{"asset": name, "definitions": locs}
	if dup, _ := w.Duplicates(name); len(dup) > 0 {
		var types []string
		for t := range dup {
			types = append(types, t)
		}
		sort.Strings(types)
		out["warning"] = fmt.Sprintf("%s defined more than once — the linker fails with `Duplicate '%s' asset`", strings.Join(types, ", "), types[0])
	} else if len(locs) > 1 {
		out["note"] = "one definition per type — names are per type, so this is fine"
	}
	if len(locs) == 0 {
		out["note"] = "not in any GDT under the root — it may still ship inside a stock fastfile"
	}
	return out, nil
}

func gdtGet(w *gdt.Workspace, name, file string, filter string, all bool) (any, error) {
	locs, err := w.Find(name)
	if err != nil {
		return nil, err
	}
	loc, err := pickDefinition(locs, name, file)
	if err != nil {
		return nil, err
	}
	f, err := w.Load(loc.File)
	if err != nil {
		return nil, err
	}
	a := f.AtLine(loc.Line)
	if a == nil || a.Name != name {
		return nil, fmt.Errorf("%s changed since it was indexed (%q is no longer at line %d) — retry", loc.File, name, loc.Line)
	}
	typ, fields, err := w.Resolved(f, a)
	if err != nil {
		return nil, err
	}
	v := gdtAssetView{Asset: name, File: loc.File, Line: loc.Line, Type: typ, Parent: a.Parent, Stock: loc.Stock, Own: map[string]string{}}
	sc, _ := w.Schema(typ)
	ff := &fieldFilter{filter: strings.ToLower(filter), all: all, schema: sc}
	own := map[string]bool{}
	for _, fl := range a.Fields {
		own[fl.Key] = true
		if val := gdt.Unquote(fl.Value); ff.show(fl.Key, val) {
			v.Own[fl.Key] = val
		}
	}
	if a.Parent != "" {
		v.Inherited = map[string]string{}
		for _, fl := range fields {
			if val := gdt.Unquote(fl.Value); !own[fl.Key] && ff.show(fl.Key, val) {
				v.Inherited[fl.Key] = val
			}
		}
	}
	if ff.hidden > 0 {
		v.Hidden = fmt.Sprintf("%d empty, zero or default-valued fields not shown (pass all=true)", ff.hidden)
	}
	if iss, _, err := w.Validate(f, a); err == nil {
		v.Issues = iss
	}
	if len(locs) > 1 {
		v.Others = locs
	}
	return v, nil
}

// pickDefinition chooses the definition to read: the one in file, else the first.
func pickDefinition(locs []gdt.Location, name, file string) (gdt.Location, error) {
	if len(locs) == 0 {
		return gdt.Location{}, fmt.Errorf("asset %q not found in any GDT", name)
	}
	if file == "" {
		return locs[0], nil
	}
	var loc gdt.Location
	found := false
	for _, l := range locs {
		if strings.EqualFold(l.File, strings.ReplaceAll(file, `\`, "/")) {
			loc, found = l, true // the last match, as before
		}
	}
	if !found {
		return gdt.Location{}, fmt.Errorf("asset %q is not defined in %s", name, file)
	}
	return loc, nil
}

// fieldFilter decides which fields gdt_get shows: those whose key contains
// filter, and — unless all — not empty, zero or the .awi's default. It counts
// the ones it hides.
type fieldFilter struct {
	filter string // lower-cased
	all    bool
	schema *gdt.Schema
	hidden int
}

func (ff *fieldFilter) show(k, val string) bool {
	if ff.filter != "" && !strings.Contains(strings.ToLower(k), ff.filter) {
		return false
	}
	if !ff.all && ff.isDefault(k, val) {
		ff.hidden++
		return false
	}
	return true
}

func (ff *fieldFilter) isDefault(k, val string) bool {
	if val == "" || val == "0" {
		return true
	}
	if ff.schema == nil {
		return false
	}
	e := ff.schema.Lookup(k)
	return e != nil && e.Default != "" && sameValue(e.Default, val)
}

func gdtSchema(w *gdt.Workspace, typ, materialType, filter string) (any, error) {
	if typ == "" {
		types, err := gdt.AssetTypes(w.Deffiles)
		return map[string]any{"asset_types": types}, err
	}
	sc, err := w.Schema(typ)
	if err != nil {
		return nil, err
	}
	var keep func(*gdt.Entry) bool
	var ts *gdt.Techset
	if typ == "material" && materialType != "" {
		var err error
		shared, err := w.Techsets.Resolve(materialType)
		if err != nil {
			return nil, err
		}
		cp := *shared // Resolve's result is cached and shared
		cp.File = w.Rel(cp.File)
		ts = &cp
	}
	switch {
	case filter != "":
		f := strings.ToLower(filter)
		keep = func(e *gdt.Entry) bool { return strings.Contains(strings.ToLower(e.Name+" "+e.Title), f) }
	case ts != nil:
		exposed := ts.ExposedFields()
		core := map[string]bool{"materialType": true, "materialCategory": true, "surfaceType": true, "usage": true}
		keep = func(e *gdt.Entry) bool { return exposed[e.Name] || core[e.Name] }
	}
	entries := []*gdt.Entry{}
	var names []string
	for _, e := range sc.Ordered() {
		if keep == nil || keep(e) {
			entries = append(entries, e)
			names = append(names, e.Name)
		}
	}
	out := map[string]any{"type": typ, "deffile": w.Rel(sc.File), "count": len(entries),
		"note": "declarations read statically from the .awi (combo options too when held in a static variable); its script isn't run, but gdt_edit/gdt_check apply the rules its validation callbacks enforce"}
	if keep == nil && len(entries) > schemaDetailLimit {
		// Too big to detail in one answer: list names only; filter for details.
		out["fields"] = strings.Join(names, " ")
		out["hint"] = fmt.Sprintf("%d fields: names only. Pass filter (e.g. \"lod\", \"gloss\") for kinds, ranges, options and defaults", len(entries))
	} else {
		out["entries"] = entries
	}
	if len(sc.Entries) < 3 {
		out["warning"] = "this deffile builds most of its fields from script functions at runtime (e.g. list types via asset_list_helper.h), " +
			"so few or no fields can be read statically — copy an existing asset of this type (gdt_get) rather than relying on this schema"
	}
	if typ == "material" {
		if ts != nil {
			out["techset"] = ts
		} else {
			out["material_types"] = len(w.Techsets.Names())
			out["material_types_hint"] = "pass material_type to get that techset's texture slots and only the fields it reads"
		}
	}
	return out, nil
}

// sameValue compares two GDT values, numerically when both parse as numbers
// ("1" == "1.000000").
func sameValue(a, b string) bool {
	if a == b {
		return true
	}
	fa, ea := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, eb := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return ea == nil && eb == nil && fa == fb
}

// schemaDetailLimit is the most entries gdt_schema details in one answer; above
// it only names are returned (a material has ~660 fields, a weapon ~1,000).
const schemaDetailLimit = 80

func gdtEdit(w *gdt.Workspace, req gdt.EditRequest) (any, error) {
	return w.Edit(req)
}

// gdtEditItem is one asset of a batch edit, as the MCP tool and --batch JSON take it.
type gdtEditItem struct {
	Asset    string         `json:"asset"`
	Type     string         `json:"type"`
	Parent   string         `json:"parent"`
	CopyFrom string         `json:"copy_from"`
	Set      map[string]any `json:"set"`
	Unset    []string       `json:"unset"`
	Image    *gdtImageArg   `json:"image"`
}

type gdtImageArg struct {
	Texture      string `json:"texture"`
	Semantic     string `json:"semantic"`
	MaterialType string `json:"material_type"`
	Field        string `json:"field"`
}

// request converts JSON values the way a GDT stores them: strings as they are,
// numbers without exponent, booleans as 1/0 (checkboxes), null as "unset".
func (it gdtEditItem) request(file string) (gdt.EditRequest, error) {
	set := map[string]string{}
	unset := append([]string(nil), it.Unset...)
	for k, v := range it.Set {
		switch x := v.(type) {
		case nil:
			unset = append(unset, k)
		case string:
			set[k] = x
		case bool:
			set[k] = map[bool]string{true: "1", false: "0"}[x]
		case float64:
			set[k] = strconv.FormatFloat(x, 'f', -1, 64)
		default:
			return gdt.EditRequest{}, fmt.Errorf("set.%s: want a string, number, boolean or null, got %T", k, v)
		}
	}
	r := gdt.EditRequest{File: file, Asset: it.Asset, Type: it.Type, Parent: it.Parent, CopyFrom: it.CopyFrom, Set: set, Unset: unset}
	if it.Image != nil && *it.Image != (gdtImageArg{}) { // an empty image {} means no image, as on the CLI
		r.Image = &gdt.ImageSpec{Texture: it.Image.Texture, Semantic: it.Image.Semantic, MaterialType: it.Image.MaterialType, Field: it.Image.Field}
	}
	return r, nil
}

func gdtEditBatch(w *gdt.Workspace, file string, items []gdtEditItem, dryRun bool) (any, error) {
	reqs := make([]gdt.EditRequest, len(items))
	for i, it := range items {
		r, err := it.request(file)
		if err != nil {
			return nil, fmt.Errorf("assets[%d] (%s): %w", i, it.Asset, err)
		}
		reqs[i] = r
	}
	return w.EditBatch(file, reqs, dryRun)
}

// gdtEditArgs is gdt_edit's whole argument object: one asset's fields plus the
// call's own parameters, decoded strictly so a typo (copyFrom) is an error.
type gdtEditArgs struct {
	gdtEditItem
	File      string            `json:"file"`
	Write     bool              `json:"write"`
	ToolsPath string            `json:"tools_path"`
	Assets    []json.RawMessage `json:"assets"`
}

func gdtCheck(w *gdt.Workspace, file, asset string) (any, error) {
	r, err := w.Check(file, asset)
	if err != nil || len(r.Assets) <= checkLimit {
		return r, err
	}
	// A converter-made GDT can have thousands of issues: show the first assets and
	// what the rest are, grouped, instead of a megabyte of JSON.
	quoted := regexp.MustCompile(`"[^"]*"|\S*/\S+`) // values and paths
	counts := map[string]int{}
	for _, a := range r.Assets {
		for _, is := range a.Issues {
			counts[fmt.Sprintf("%s %s.%s: %s", is.Level, a.Type, is.Field, quoted.ReplaceAllString(is.Msg, "…"))]++
		}
	}
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if counts[kinds[i]] != counts[kinds[j]] {
			return counts[kinds[i]] > counts[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	if len(kinds) > checkLimit {
		kinds = kinds[:checkLimit]
	}
	summary := make([]string, len(kinds))
	for i, k := range kinds {
		summary[i] = fmt.Sprintf("%d× %s", counts[k], k)
	}
	return map[string]any{
		"file": r.File, "assets_checked": r.Checked, "errors": r.Errors, "warnings": r.Warnings,
		"issue_kinds":        summary,
		"assets_with_issues": r.Assets[:checkLimit],
		"truncated":          fmt.Sprintf("showing %d of %d assets with issues — pass asset to check one", checkLimit, len(r.Assets)),
	}, nil
}

// checkLimit caps gdt_check's per-asset detail (and its grouped summary).
const checkLimit = 25

func gdtRefs(w *gdt.Workspace, name string) (any, error) {
	hits, unreadable, err := w.ReferencedBy(name)
	if err != nil {
		return nil, err
	}
	models, err := w.ModelUses(name)
	if err != nil {
		return nil, err
	}
	hits = mergeHits(append(hits, models...))
	out := map[string]any{"asset": name, "referenced_by": hits, "count": len(hits),
		"note": "GDT fields, derived assets, and the materials inside xmodel LOD files; zone and script mentions are not searched"}
	if len(unreadable) > 0 {
		out["unreadable_gdts"] = unreadable // not searched: a reference there would be missed
	}
	if len(hits) > refsLimit {
		out["referenced_by"] = hits[:refsLimit]
		out["truncated"] = fmt.Sprintf("showing %d of %d", refsLimit, len(hits))
	}
	return out, nil
}

// mergeHits folds the hits on one asset into one, its fields joined: an xmodel
// can name a material in skinOverride, in APE's materials copy, and in each LOD
// file. Sorted by file and line, so a cap keeps a stable prefix.
func mergeHits(hits []gdt.RefHit) []gdt.RefHit {
	type key struct {
		file  string
		line  int
		asset string
	}
	by := map[key]int{}
	var out []gdt.RefHit
	for _, h := range hits {
		k := key{h.File, h.Line, h.Asset}
		if i, ok := by[k]; ok {
			out[i].Field += ", " + h.Field
			if out[i].Type == "" {
				out[i].Type = h.Type
			}
			continue
		}
		by[k] = len(out)
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// refsLimit caps gdt_refs' list: a stock image can be used by hundreds of materials.
const refsLimit = 50
