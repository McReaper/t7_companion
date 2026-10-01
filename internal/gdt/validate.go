package gdt

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Issue is one validation finding. Errors block a write; warnings don't.
type Issue struct {
	Level string `json:"level"` // "error" | "warning"
	Field string `json:"field,omitempty"`
	Msg   string `json:"message"`
	Code  string `json:"-"` // stable kind, for callers that filter (see the code* constants)
}

// Issue codes that callers filter on — never match on Msg text.
const (
	codeUndeclared = "undeclared" // key not declared in the .awi (APE also writes script-built keys)
	codeNoAsset    = "no-asset"   // AssetCombo target in no GDT (gdt_check reports it with the expected type)
	codeStaleSlot  = "stale-slot" // texture field the material type's techset doesn't read
)

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
		return append(out, Issue{"warning", "", "asset type unknown (derived from a parent that wasn't found) — fields not validated", ""})
	}
	sc, err := w.Schema(typ)
	if err != nil {
		return append(out, Issue{"error", "", err.Error(), ""})
	}
	val := map[string]string{}
	for _, f := range fields {
		val[f.Key] = Unquote(f.Value)
	}
	for _, k := range keys {
		v := val[k]
		e := sc.Lookup(k)
		if e == nil {
			out = append(out, Issue{"warning", k, fmt.Sprintf("not declared in %s.awi — APE won't show it; check the spelling", typ), codeUndeclared})
			continue
		}
		if v == "" || e.Varies {
			continue // empty = the default; a Varies field's kind depends on script state
		}
		switch e.Kind {
		case "Float", "Int":
			n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not a number", v), ""})
				break
			}
			if e.Kind == "Int" && n != float64(int64(n)) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not an integer", v), ""})
			}
			if e.Min != nil && e.Max != nil && *e.Min < *e.Max && (n < *e.Min || n > *e.Max) {
				// a warning: the .awi range is the slider's, and stock GDTs go past it (and link)
				out = append(out, Issue{"warning", k, fmt.Sprintf("%v is outside APE's slider range %v..%v", n, *e.Min, *e.Max), ""})
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
				out = append(out, Issue{"error", k, fmt.Sprintf("%q: expected %d space-separated numbers (e.g. %q)", v, n, strings.TrimSpace(strings.Repeat("1 ", n))), ""})
			}
		case "CheckBox":
			// converter-written GDTs also store "True"/"False", which link
			if v != "0" && v != "1" && !strings.EqualFold(v, "true") && !strings.EqualFold(v, "false") {
				out = append(out, Issue{"error", k, fmt.Sprintf("checkbox wants 0 or 1, got %q", v), ""})
			}
		case "Combo":
			// "<none>*": converter-written GDTs keep the .awi's default marker on the value;
			// APE compares choices case-insensitively ("stand" for "Stand"). A value
			// stock GDTs use links, even when the .awi has since dropped it ("legacy
			// support" filter names, "<legacy>" gloss).
			if len(e.Options) > 1 && !e.Editable && !e.openOptions && !optionMatch(e.Options, strings.TrimSuffix(v, "*")) &&
				!w.stockUses(typ, k, strings.TrimSuffix(v, "*")) {
				out = append(out, Issue{"error", k, fmt.Sprintf("%q is not one of: %s", v, strings.Join(e.Options, " | ")), ""})
			}
		case "AssetCombo":
			if strings.HasPrefix(v, "$") { // $white_diffuse etc. are engine built-ins
				break
			}
			if local == nil || local.Find(v) == nil {
				if locs, err := w.Find(v); err == nil && len(locs) == 0 {
					out = append(out, Issue{"warning", k, fmt.Sprintf("no %s asset named %q in any GDT (fine if it ships in a stock fastfile)", e.AssetType, v), codeNoAsset})
				}
			}
		}
	}
	only := map[string]bool{}
	for _, k := range keys {
		only[k] = true
	}
	switch typ {
	case "material":
		if sc.Lookup("surfaceType") != nil {
			out = append(out, surfaceTypeSet(val, only)...)
		}
		out = append(out, w.validateMaterial(local, val, only)...)
	case "xmodel":
		out = append(out, validateLODs(val, only)...)
	}
	return out
}

// surfaceTypeSet warns about a material with no surface type. "<error>" is the
// .awi's first surfaceType, so APE's default. Every stock material sets one; the
// linker stops on a material reaching collision without: `Material '…' - doesn't
// have surfacetype set in Asset Manager`. A warning: 2 stock materials keep
// "<error>" and link (they never reach collision).
func surfaceTypeSet(val map[string]string, only map[string]bool) []Issue {
	st := val["surfaceType"]
	if !(only["surfaceType"] || only["materialType"]) || (st != "" && !strings.EqualFold(st, "<error>")) {
		return nil
	}
	return []Issue{{"warning", "surfaceType", "no surface type (\"<error>\", APE's default) — if the material reaches collision the linker stops: " +
		"`doesn't have surfacetype set in Asset Manager`; set one (metal, wood, <none>, …)", ""}}
}

// The xmodel LOD fields, in order: the model, the distance it switches at, and
// the checkbox that generates it instead (index 0 is the base model).
var (
	lodModels  = []string{"filename", "mediumLod", "lowLod", "lowestLod", "lod4File", "lod5File", "lod6File", "lod7File"}
	lodDists   = []string{"highLodDist", "mediumLodDist", "lowLodDist", "lowestLodDist", "lod4Dist", "lod5Dist", "lod6Dist", "lod7Dist"}
	lodAutogen = []string{"", "autogenMediumLod", "autogenLowLod", "autogenLowestLod", "autogenLod4", "autogenLod5", "autogenLod6", "autogenLod7"}
)

// validateLODs is xmodel.awi's ValidateLODs: a LOD in use (a model, or
// autogenerated) must switch farther out than the one before it, or be 0. APE
// only marks the field red, and 22 of 13,121 stock xmodels break it and still
// link, so it's a warning: such a LOD is never drawn.
func validateLODs(val map[string]string, only map[string]bool) []Issue {
	num := func(k string) float64 {
		n, _ := strconv.ParseFloat(strings.TrimSpace(val[k]), 64)
		return n
	}
	touched := false
	for i := range lodModels {
		touched = touched || only[lodModels[i]] || only[lodDists[i]] || only[lodAutogen[i]]
	}
	if !touched {
		return nil
	}
	var out []Issue
	for i := 1; i < len(lodDists); i++ {
		on := val[lodModels[i]] != "" || val[lodAutogen[i]] == "1" || strings.EqualFold(val[lodAutogen[i]], "true")
		prev, d := num(lodDists[i-1]), num(lodDists[i])
		if on && d != 0 && d <= prev {
			out = append(out, Issue{"warning", lodDists[i], fmt.Sprintf("%v is not farther than %s (%v) — this LOD is never drawn (APE marks it red)", d, lodDists[i-1], prev), ""})
		}
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
		return append(out, Issue{"error", "materialType", err.Error() + " — APE shows INVALID MATERIAL TYPE", ""})
	}
	if cat := val["materialCategory"]; ts.Category != "" && cat != "" && !strings.EqualFold(cat, ts.Category) {
		out = append(out, Issue{"error", "materialCategory", fmt.Sprintf("%q but materialType %q is category %q — they must agree", cat, mt, ts.Category), ""})
	}
	exposed := ts.ExposedFields()
	keys := make([]string, 0, len(val))
	for k := range val {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if val[k] == "" || !isImageField(k) || !only[k] {
			continue
		}
		if !exposed[k] {
			out = append(out, Issue{"warning", k, fmt.Sprintf("materialType %q doesn't read %s — the image is ignored (or the link fails with `doesn't expose a '%s' texture`)", mt, k, k), codeStaleSlot})
		}
	}
	for _, s := range ts.Textures {
		img := val[s.Field]
		if img == "" || s.Semantic == "" || !only[s.Field] {
			continue
		}
		var ia *Asset
		if local != nil {
			for _, c := range local.FindAll(img) {
				if c.Type == "image" {
					ia = c
				}
			}
		}
		if ia == nil {
			_, ia, _ = w.loadAsset(img, "image")
		}
		if ia != nil {
			if sem, ok := ia.Get("semantic"); ok && sem != "" && !strings.EqualFold(Unquote(sem), s.Semantic) {
				out = append(out, Issue{"warning", s.Field, fmt.Sprintf("image %q has semantic %q but slot %s expects %q (APE: type mismatch)", img, Unquote(sem), s.Name, s.Semantic), ""})
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
