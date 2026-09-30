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
// not executed here.
type Entry struct {
	Name      string   `json:"name"`
	Pattern   bool     `json:"pattern,omitempty"` // name built at runtime: Name has a * for each runtime part ("autogenLod*Percent")
	Kind      string   `json:"kind"`              // Float, Int, CheckBox, Combo, AssetCombo, String, Path, Vector, Color, …
	Default   string   `json:"default,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Options   []string `json:"options,omitempty"`    // Combo choices (when static)
	AssetType string   `json:"asset_type,omitempty"` // AssetCombo target type ("image", "xanim", …)
	Title     string   `json:"title,omitempty"`
	Tooltip   string   `json:"tooltip,omitempty"`
	Hidden    bool     `json:"hidden,omitempty"`        // declared with .Show( false ) somewhere
	RelPath   string   `json:"relative_path,omitempty"` // Path entries: directory the value is relative to
	Varies    bool     `json:"varies,omitempty"`        // declared again with another kind or target (script branches): not type-checked

	re *regexp.Regexp // Pattern entries: the literal parts in order, anything between
}

// Schema is the set of fields declared for one asset type.
type Schema struct {
	Type    string            `json:"type"`
	File    string            `json:"file"`
	Entries map[string]*Entry `json:"-"`
	order   []string
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
	for _, e := range s.Entries {
		if !e.Pattern && strings.EqualFold(e.Name, key) {
			return e
		}
	}
	// Several patterns can match ("autogenLod*" and "autogenLod*Percent"): the one
	// with the most literal text is the most specific.
	var best *Entry
	bestLit := 0
	for _, e := range s.Entries {
		if !e.Pattern || !e.re.MatchString(key) {
			continue
		}
		if lit := len(strings.ReplaceAll(e.Name, "*", "")); best == nil || lit > bestLit || (lit == bestLit && e.Name < best.Name) {
			best, bestLit = e, lit
		}
	}
	return best
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
	for _, m := range addEntryRE.FindAllStringSubmatchIndex(code, -1) {
		kind := code[m[2]:m[3]]
		args, end := balanced(code, m[1]-1)
		if end < 0 || uiOnly[kind] {
			continue
		}
		parts := splitArgs(args)
		if len(parts) == 0 {
			continue
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
				fillArgs(d, kind, parts[1:])
			} else {
				d.Kind = "Float" // one component of a multi-field control
			}
			fillChain(d, chain)
			if e := s.Entries[name]; e != nil {
				e.merge(d)
				continue
			}
			s.Entries[name] = d
			s.order = append(s.order, name)
		}
	}
	return s, nil
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
			rx.WriteString(".*")
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

func fillArgs(e *Entry, kind string, args []string) {
	str := func(i int) (string, bool) {
		if i >= len(args) {
			return "", false
		}
		a := strings.TrimSpace(args[i])
		if len(a) >= 2 && a[0] == '"' && a[len(a)-1] == '"' {
			return a[1 : len(a)-1], true
		}
		return a, false
	}
	num := func(i int) *float64 {
		if i >= len(args) {
			return nil
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[i]), 64)
		if err != nil {
			return nil
		}
		return &v
	}
	switch kind {
	case "Float", "Int":
		if d := num(0); d != nil {
			e.Default = strconv.FormatFloat(*d, 'f', -1, 64)
		}
		e.Min, e.Max = num(1), num(2)
	case "CheckBox":
		if v, _ := str(0); v == "true" {
			e.Default = "1"
		} else if v == "false" {
			e.Default = "0"
		}
	case "Combo":
		if v, lit := str(0); lit {
			for _, o := range strings.Split(v, "|") {
				o = strings.TrimSpace(o)
				if strings.HasSuffix(o, "*") {
					o = strings.TrimSuffix(o, "*")
					e.Default = o
				}
				if o != "" {
					e.Options = append(e.Options, o)
				}
			}
			if e.Default == "" && len(e.Options) > 0 {
				e.Default = e.Options[0]
			}
		}
	case "AssetCombo":
		if v, lit := str(0); lit {
			e.AssetType = v
		}
	case "String", "Path", "Texture", "Text":
		if v, lit := str(0); lit {
			e.Default = v
		}
	case "Vector", "Color":
		var vs []string
		for i := range args {
			if n := num(i); n != nil {
				vs = append(vs, strconv.FormatFloat(*n, 'f', -1, 64))
			}
		}
		e.Default = strings.Join(vs, " ")
	}
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
