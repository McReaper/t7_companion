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
	Pattern   bool     `json:"pattern,omitempty"`  // name built at runtime: Name is the literal part
	Anywhere  bool     `json:"anywhere,omitempty"` // runtime part comes first (prefix + "name" + suffix): match Name anywhere in the key
	Kind      string   `json:"kind"`               // Float, Int, CheckBox, Combo, AssetCombo, String, Path, Vector, Color, …
	Default   string   `json:"default,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Options   []string `json:"options,omitempty"`    // Combo choices (when static)
	AssetType string   `json:"asset_type,omitempty"` // AssetCombo target type ("image", "xanim", …)
	Title     string   `json:"title,omitempty"`
	Tooltip   string   `json:"tooltip,omitempty"`
	Hidden    bool     `json:"hidden,omitempty"` // declared with .Show( false ) somewhere
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

// Lookup finds the entry that declares key, including runtime-built names.
func (s *Schema) Lookup(key string) *Entry {
	if e, ok := s.Entries[key]; ok && !e.Pattern {
		return e
	}
	var best *Entry
	for _, e := range s.Entries {
		if !e.Pattern {
			continue
		}
		ok := strings.HasPrefix(key, e.Name)
		if e.Anywhere {
			ok = strings.Contains(key, e.Name)
		}
		if ok && (best == nil || len(e.Name) > len(best.Name)) {
			best = e
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
				if _, _, _, ok := entryName(parts[nNames]); !ok {
					break
				}
				nNames++
			}
		}
		chain := chainAfter(code, end)
		for i := 0; i < nNames; i++ {
			name, pattern, anywhere, ok := entryName(parts[i])
			if !ok {
				continue // name is a variable — nothing static to learn
			}
			e := s.Entries[name]
			if e == nil {
				e = &Entry{Name: name, Pattern: pattern, Anywhere: anywhere, Kind: kind}
				s.Entries[name] = e
				s.order = append(s.order, name)
			}
			if nNames == 1 {
				fillArgs(e, kind, parts[1:])
			} else {
				e.Kind = "Float" // one component of a multi-field control
			}
			fillChain(e, chain)
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

// entryName reads a name argument: "literal", "prefix" + expr, or expr + "core" + expr.
// It returns the longest string literal, whether the name is built at runtime,
// whether a runtime part precedes it, and whether any literal was found.
func entryName(arg string) (lit string, pattern, anywhere, ok bool) {
	arg = strings.TrimSpace(arg)
	if !strings.Contains(arg, `"`) {
		return "", false, false, false
	}
	start := -1
	for i := 0; i < len(arg); i++ {
		if arg[i] != '"' {
			continue
		}
		j := strings.IndexByte(arg[i+1:], '"')
		if j < 0 {
			break
		}
		if s := arg[i+1 : i+1+j]; len(s) > len(lit) {
			lit, start = s, i
		}
		i += j + 1
	}
	if lit == "" {
		return "", false, false, false
	}
	whole := len(arg) == len(lit)+2
	return lit, !whole, start > 0, true
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
