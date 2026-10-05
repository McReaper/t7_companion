package gdt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Entry is one field APE's property page declares for an asset type, read
// statically from the AngelScript deffile (deffiles/<type>.awi). Only the
// declaration is known: the .awi's own validation functions are script and are
// not executed here — the few that matter are ported (validateLODs,
// surfaceTypeSet, apeeffects.go).
type Entry struct {
	Name      string   `json:"name"`
	Pattern   bool     `json:"pattern,omitempty"` // name built at runtime: Name has a * for each runtime part ("autogenLod*Percent")
	Kind      string   `json:"kind"`              // Float, Int, CheckBox, Combo, AssetCombo, String, Path, Vector, Color, …
	Default   string   `json:"default,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Options   []string `json:"options,omitempty"`    // Combo choices (when static: a literal or a static variable)
	Editable  bool     `json:"editable,omitempty"`   // Combo that also takes free text: Options are suggestions
	AssetType string   `json:"asset_type,omitempty"` // AssetCombo target type ("image", "xanim", …)
	Title     string   `json:"title,omitempty"`
	Tooltip   string   `json:"tooltip,omitempty"`
	Hidden    bool     `json:"hidden,omitempty"`        // declared with .Show( false ) somewhere
	RelPath   string   `json:"relative_path,omitempty"` // Path entries: directory the value is relative to
	Varies    bool     `json:"varies,omitempty"`        // declared again with another kind or target (script branches): not type-checked
	Count     string   `json:"count_field,omitempty"`   // an item list's entries (bodyType01…): the field counting the ones in use

	// a Combo whose options (in this or another declaration) aren't static: any
	// value may be valid, so none is checked
	openOptions bool
	re          *regexp.Regexp // Pattern entries: the literal parts in order, anything between
}

// Schema is the set of fields declared for one asset type.
type Schema struct {
	Type    string            `json:"type"`
	File    string            `json:"file"`
	Entries map[string]*Entry `json:"-"`
	order   []string

	lower    map[string]*Entry // lower-cased literal names (built once loaded)
	patterns []*Entry          // runtime-built names, most specific first

	glossPresets map[string][2]string // material: glossSurfaceType -> glossRangeMin/Max (apeeffects.go)
}

// index builds the lookup tables once the schema is complete.
func (s *Schema) index() {
	s.lower = map[string]*Entry{}
	s.patterns = nil
	for _, n := range s.order {
		e := s.Entries[n]
		if e.Pattern {
			s.patterns = append(s.patterns, e)
		} else if _, dup := s.lower[strings.ToLower(n)]; !dup {
			s.lower[strings.ToLower(n)] = e
		}
	}
	// Several patterns can match ("autogenLod*" and "autogenLod*Percent"): the one
	// with the most literal text is the most specific.
	lit := func(e *Entry) int { return len(strings.ReplaceAll(e.Name, "*", "")) }
	sort.SliceStable(s.patterns, func(i, j int) bool {
		if lit(s.patterns[i]) != lit(s.patterns[j]) {
			return lit(s.patterns[i]) > lit(s.patterns[j])
		}
		return s.patterns[i].Name < s.patterns[j].Name
	})
}

// Ordered returns the entries in declaration order.
func (s *Schema) Ordered() []*Entry {
	out := make([]*Entry, 0, len(s.order))
	for _, n := range s.order {
		out = append(out, s.Entries[n])
	}
	return out
}

// merge folds a second declaration of the same field into e. .awi scripts
// declare a field in several branches (xanim's note "actionparam1" is a weapon,
// an FX path, a tagfx… depending on the note's action): when kind or target
// differ, which one applies is decided at runtime, so the entry is marked Varies.
// Combo options are unioned and numeric ranges widened.
func (e *Entry) merge(d *Entry) {
	if d.Kind != e.Kind || d.AssetType != e.AssetType {
		e.Varies = true
	}
	for _, o := range d.Options {
		if !containsFold(e.Options, o) {
			e.Options = append(e.Options, o)
		}
	}
	if e.Min != nil && d.Min != nil && *d.Min < *e.Min {
		e.Min = d.Min
	}
	if e.Max != nil && d.Max != nil && *d.Max > *e.Max {
		e.Max = d.Max
	}
	e.Hidden = e.Hidden || d.Hidden
	e.Editable = e.Editable || d.Editable
	e.openOptions = e.openOptions || d.openOptions
}

func containsFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// Lookup finds the entry that declares key, including runtime-built names.
// APE matches field names case-insensitively (scriptbundle.awi declares "type",
// GDTs store "Type"), so an exact match is tried first, then any case.
func (s *Schema) Lookup(key string) *Entry {
	if e, ok := s.Entries[key]; ok && !e.Pattern {
		return e
	}
	if s.lower == nil {
		s.index()
	}
	if e, ok := s.lower[strings.ToLower(key)]; ok {
		return e
	}
	for _, e := range s.patterns {
		if e.re.MatchString(key) {
			return e
		}
	}
	return nil
}

// uiOnly kinds are page decoration, never saved to the GDT.
var uiOnly = map[string]bool{"Label": true, "ButtonGroup": true}

var addEntryRE = regexp.MustCompile(`AddEntry_(\w+)\s*\(`)

// LoadSchema parses deffiles/<assetType>.awi.
func LoadSchema(deffiles, assetType string) (*Schema, error) {
	path := filepath.Join(deffiles, assetType+".awi")
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no deffile for asset type %q: %w", assetType, err)
	}
	s := &Schema{Type: assetType, File: path, Entries: map[string]*Entry{}}
	code := expandIncludes(deffiles, stripComments(string(src)), map[string]bool{path: true})
	vars := parseAwiVars(code)
	s.glossPresets = glossPresets(code)
	for _, m := range addEntryRE.FindAllStringSubmatchIndex(code, -1) {
		s.declare(code, m, vars)
	}
	for _, d := range itemLists(code) {
		s.add(d)
	}
	s.index() // before the schema is shared: Lookup must not build it concurrently
	return s, nil
}

// declare reads one AddEntry_<kind>( … ) call, at m in code.
func (s *Schema) declare(code string, m []int, vars *awiVars) {
	kind := code[m[2]:m[3]]
	args, end := balanced(code, m[1]-1)
	if end < 0 || uiOnly[kind] {
		return
	}
	parts := splitArgs(args)
	if len(parts) == 0 {
		return
	}
	// VectorN / multi-field controls declare one field per leading string literal:
	// AddEntry_Vector2( "glossRangeMin", "glossRangeMax", … )
	nNames := 1
	if strings.HasPrefix(kind, "Vector") && len(kind) > len("Vector") {
		for nNames < len(parts) {
			if _, _, ok := entryName(parts[nNames]); !ok {
				break
			}
			nNames++
		}
	}
	chain := chainAfter(code, end)
	for i := 0; i < nNames; i++ {
		name, re, ok := entryName(parts[i])
		if !ok {
			continue // name is a variable — nothing static to learn
		}
		d := &Entry{Name: name, Pattern: re != nil, Kind: kind, re: re}
		if nNames == 1 {
			fillArgs(d, kind, parts[1:], vars)
		} else {
			d.Kind = "Float" // one component of a multi-field control
		}
		fillChain(d, chain)
		s.add(d)
	}
}

// add declares an entry, merged into an earlier declaration of the same name.
func (s *Schema) add(d *Entry) {
	if e := s.Entries[d.Name]; e != nil {
		e.merge(d)
		return
	}
	s.Entries[d.Name] = d
	s.order = append(s.order, d.Name)
}

var itemListRE = regexp.MustCompile(`GenerateItemList\(\s*Asset\s*,\s*"([^"]+)"\s*,\s*"[^"]*"\s*,\s*"([^"]+)"`)

// itemLists declares the fields of each GenerateItemList( Asset, "<type>",
// "<title>", "<prefix>", max ) call (asset_list_helper.h), whose own
// declarations use variables: the items <prefix>01, <prefix>02… referencing a
// <type> each, and <prefix>Count, how many are in use.
func itemLists(code string) []*Entry {
	var out []*Entry
	for _, m := range itemListRE.FindAllStringSubmatch(code, -1) {
		typ, prefix := m[1], m[2]
		out = append(out,
			&Entry{Name: prefix + "*", Pattern: true, Kind: "AssetCombo", AssetType: typ, Count: prefix + "Count",
				re: regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(prefix) + `\d+$`)},
			&Entry{Name: prefix + "Count", Kind: "Int", Hidden: true})
	}
	return out
}

var awiIncludeRE = regexp.MustCompile(`(?m)^\s*#include\s+"([^"]+)"`)

// expandIncludes inlines `#include "x"` files from the deffiles directory (list
// types such as accoladelist declare their fields in asset_list_helper.h).
func expandIncludes(deffiles, code string, seen map[string]bool) string {
	return awiIncludeRE.ReplaceAllStringFunc(code, func(m string) string {
		name := awiIncludeRE.FindStringSubmatch(m)[1]
		p := filepath.Join(deffiles, name)
		if seen[p] {
			return ""
		}
		seen[p] = true
		b, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		return expandIncludes(deffiles, stripComments(string(b)), seen)
	})
}

// entryName reads a name argument: "literal", or an expression mixing string
// literals with runtime parts ("layer" + i, prefix + "viewModel" + suffix,
// "autogenLod" + lodIndex + "Percent"). A runtime-built name comes back with a *
// for each runtime part and a regexp matching the literals in order.
func entryName(arg string) (name string, re *regexp.Regexp, ok bool) {
	arg = strings.TrimSpace(arg)
	var disp, rx strings.Builder
	lits := 0
	gap := func(s string) { // anything but "+" between literals is a runtime part
		if strings.Trim(s, " \t\r\n+") != "" {
			disp.WriteString("*")
			rx.WriteString(".+") // a runtime part is never empty: "*name" must not claim a plain "name" key
		}
	}
	for i := 0; ; {
		j := strings.IndexByte(arg[i:], '"')
		if j < 0 {
			gap(arg[i:])
			break
		}
		gap(arg[i : i+j])
		k := strings.IndexByte(arg[i+j+1:], '"')
		if k < 0 {
			return "", nil, false
		}
		lit := arg[i+j+1 : i+j+1+k]
		disp.WriteString(lit)
		rx.WriteString(regexp.QuoteMeta(lit))
		lits += len(lit)
		i += j + 1 + k + 1
	}
	if lits == 0 {
		return "", nil, false
	}
	name = disp.String()
	if !strings.Contains(name, "*") {
		return name, nil, true
	}
	return name, regexp.MustCompile("(?i)^" + rx.String() + "$"), true
}

func fillArgs(e *Entry, kind string, args []string, vars *awiVars) {
	switch kind {
	case "Float", "Int":
		if d := numArg(args, 0); d != nil {
			e.Default = strconv.FormatFloat(*d, 'f', -1, 64)
		}
		e.Min, e.Max = numArg(args, 1), numArg(args, 2)
	case "CheckBox":
		switch v, _ := strArg(args, 0); v {
		case "true":
			e.Default = "1"
		case "false":
			e.Default = "0"
		}
	case "Combo":
		fillCombo(e, args, vars)
	case "AssetCombo":
		if v, lit := strArg(args, 0); lit {
			e.AssetType = v
		}
	case "String", "Path", "Texture", "Text":
		if v, lit := strArg(args, 0); lit {
			e.Default = v
		}
	case "Vector", "Color":
		var vs []string
		for i := range args {
			if n := numArg(args, i); n != nil {
				vs = append(vs, strconv.FormatFloat(*n, 'f', -1, 64))
			}
		}
		e.Default = strings.Join(vs, " ")
	}
}

// fillCombo reads a combo's options: a literal or a static variable
// (awivars.go) — a variable assigned in several branches contributes every value
// it can hold. A "*" marks the default, else it's the first option.
func fillCombo(e *Entry, args []string, vars *awiVars) {
	var lists []string
	ok := false
	if len(args) > 0 {
		lists, ok = vars.eval(args[0])
	}
	e.openOptions = !ok
	for _, v := range lists {
		for _, o := range strings.Split(v, "|") {
			e.addOption(strings.TrimSpace(o))
		}
	}
	if e.Default == "" && len(e.Options) > 0 {
		e.Default = e.Options[0]
	}
	// AddEntry_Combo( name, options, true ): an editable combo — a scene object's
	// Name is any targetname, a notetrack's function any script function
	if len(args) > 1 && strings.TrimSpace(args[1]) == "true" {
		e.Editable = true
	}
}

func (e *Entry) addOption(o string) {
	if strings.HasSuffix(o, "*") {
		o = strings.TrimSuffix(o, "*")
		if e.Default == "" {
			e.Default = o
		}
	}
	if o != "" && !contains(e.Options, o) {
		e.Options = append(e.Options, o)
	}
}

// strArg returns argument i unquoted, and whether it was a string literal.
func strArg(args []string, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	a := strings.TrimSpace(args[i])
	if len(a) >= 2 && a[0] == '"' && a[len(a)-1] == '"' {
		return a[1 : len(a)-1], true
	}
	return a, false
}

// numArg returns argument i as a number, or nil.
func numArg(args []string, i int) *float64 {
	if i >= len(args) {
		return nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(args[i]), 64)
	if err != nil {
		return nil
	}
	return &v
}

var chainRE = regexp.MustCompile(`^\s*\.\s*(\w+)\s*\(`)

// chainAfter returns the ".Method( args )" calls chained after position i.
func chainAfter(code string, i int) map[string]string {
	calls := map[string]string{}
	for {
		m := chainRE.FindStringSubmatchIndex(code[i:])
		if m == nil {
			return calls
		}
		args, end := balanced(code, i+m[1]-1)
		if end < 0 {
			return calls
		}
		calls[code[i+m[2]:i+m[3]]] = args
		i = end
	}
}

func fillChain(e *Entry, calls map[string]string) {
	lit := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
		return ""
	}
	if v, ok := calls["SetTitle"]; ok && e.Title == "" {
		e.Title = strings.TrimSpace(lit(v))
	}
	if v, ok := calls["SetToolTip"]; ok && e.Tooltip == "" {
		e.Tooltip = lit(v)
	}
	if v, ok := calls["SetDefaultValue"]; ok {
		if d := lit(v); d != "" {
			e.Default = d
		}
	}
	if v, ok := calls["SetRelativePath"]; ok {
		e.RelPath = strings.TrimRight(lit(v), "/")
	}
	if v, ok := calls["Show"]; ok && strings.TrimSpace(v) == "false" {
		e.Hidden = true
	}
}

// balanced returns the text inside the parenthesis opening at open and the
// index just past its closing parenthesis (-1 if unbalanced). Strings are skipped.
func balanced(code string, open int) (string, int) {
	depth := 0
	for i := open; i < len(code); i++ {
		switch code[i] {
		case '"':
			for i++; i < len(code) && code[i] != '"'; i++ {
				if code[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return code[open+1 : i], i + 1
			}
		}
	}
	return "", -1
}

// splitArgs splits a call's arguments on top-level commas.
func splitArgs(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if strings.TrimSpace(s[start:]) != "" || len(out) > 0 {
		out = append(out, s[start:])
	}
	return out
}

// stripComments removes // and /* */ comments outside string literals.
func stripComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				j = len(s) - 1
			}
			b.WriteString(s[i : j+1])
			i = j
		case strings.HasPrefix(s[i:], "//"):
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case strings.HasPrefix(s[i:], "/*"):
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 3
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// AssetTypes lists the asset types that have a deffile.
func AssetTypes(deffiles string) ([]string, error) {
	m, err := filepath.Glob(filepath.Join(deffiles, "*.awi"))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(m))
	for _, p := range m {
		out = append(out, strings.TrimSuffix(filepath.Base(p), ".awi"))
	}
	sort.Strings(out)
	return out, nil
}
