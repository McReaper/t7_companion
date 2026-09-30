// Package gdt reads, validates and writes Black Ops 3 GDT files — the text
// databases APE edits — using the mod tools' own schema sources: the deffiles
// (*.awi) that build APE's property pages, and the techsetdefs that decide which
// fields a material type actually exposes.
//
// A GDT is a single brace block of assets:
//
//	{
//		"name" ( "type.gdf" )      // a full asset
//		{
//			"key" "value"
//		}
//		"child" [ "parent" ]         // a derived asset: only overrides are listed
//		{
//		}
//	}
//
// Values are stored exactly as written (backslashes already doubled); use Unquote
// and Quote to move between the file form and the real string.
package gdt

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Field is one "key" "value" line; Value keeps the file's escaped form.
type Field struct {
	Key   string
	Value string
}

// Asset is one entry of a GDT.
type Asset struct {
	Name   string
	Type   string // gdf name without extension ("material"); empty for a derived asset
	Parent string // non-empty for a derived asset: "name" [ "parent" ]
	Fields []Field
	Line   int // 1-based line of the asset header in its file

	// span of the asset's text in the parsed source (start of its header line to
	// the end of its closing-brace line); -1 for an asset added since parsing.
	start, end int
	dirty      bool // changed since parsing: re-rendered on write
}

// Get returns a field's escaped value and whether it is present.
func (a *Asset) Get(key string) (string, bool) {
	for _, f := range a.Fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return "", false
}

// Set writes a field (escaped form), keeping its position if it exists and
// otherwise inserting it in key order, the way APE saves them.
func (a *Asset) Set(key, escaped string) {
	a.dirty = true
	for i := range a.Fields {
		if a.Fields[i].Key == key {
			a.Fields[i].Value = escaped
			return
		}
	}
	i := 0
	for i < len(a.Fields) && strings.ToLower(a.Fields[i].Key) < strings.ToLower(key) {
		i++
	}
	a.Fields = append(a.Fields, Field{})
	copy(a.Fields[i+1:], a.Fields[i:])
	a.Fields[i] = Field{Key: key, Value: escaped}
}

// Unset removes a field; it reports whether it was present.
func (a *Asset) Unset(key string) bool {
	for i := range a.Fields {
		if a.Fields[i].Key == key {
			a.dirty = true
			a.Fields = append(a.Fields[:i], a.Fields[i+1:]...)
			return true
		}
	}
	return false
}

// File is a parsed GDT.
type File struct {
	Path   string
	Assets []*Asset
	CRLF   bool // the file used CRLF line endings (stock GDTs do)

	raw      []byte // the parsed source, reused verbatim for every unchanged asset; nil = render it all
	closeOff int    // offset of the start of the line holding the final "}"
	byName   map[string][]*Asset
}

// Add appends a new asset (rendered in APE's layout before the closing brace).
func (f *File) Add(a *Asset) {
	a.start, a.end, a.dirty = -1, -1, true
	f.Assets = append(f.Assets, a)
	f.byName = nil
}

// FindAll returns every asset with that name, in file order. Names are per type,
// so one GDT can hold an image and a material of the same name.
func (f *File) FindAll(name string) []*Asset {
	if f.byName == nil {
		f.byName = make(map[string][]*Asset, len(f.Assets))
		for _, a := range f.Assets {
			f.byName[a.Name] = append(f.byName[a.Name], a)
		}
	}
	return f.byName[name]
}

// Find returns the first asset with that name, or nil. Use FindAll (or a type)
// where an image and a material may share the name.
func (f *File) Find(name string) *Asset {
	if all := f.FindAll(name); len(all) > 0 {
		return all[0]
	}
	return nil
}

// AtLine returns the asset whose header is on that line, or nil.
func (f *File) AtLine(line int) *Asset {
	for _, a := range f.Assets {
		if a.Line == line {
			return a
		}
	}
	return nil
}

// ValidName rejects what the GDT format can't hold in a name, parent, type or key.
func ValidName(what, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is empty", what)
	}
	if strings.ContainsAny(s, "\"\\\r\n\t") {
		return fmt.Errorf("%s %q contains a quote, backslash, tab or line break", what, s)
	}
	return nil
}

// ParseFile reads and parses a GDT from disk.
func ParseFile(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.Path = path
	return f, nil
}

type token struct {
	kind byte // '"' for a string, else the punctuation character
	text string
	line int
	off  int // byte offset of the token's first character
}

func lex(src []byte) ([]token, error) {
	var toks []token
	line := 1
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
		case c == ' ' || c == '\t' || c == '\r':
		case i == 0 && bytes.HasPrefix(src, []byte("\xEF\xBB\xBF")):
			i += 2 // UTF-8 byte order mark
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			line++
		case strings.IndexByte("{}()[]", c) >= 0:
			toks = append(toks, token{kind: c, text: string(c), line: line, off: i})
		case c == '"':
			start, l0, o0 := i+1, line, i
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i++ // keep the escape pair verbatim
				}
				if src[i] == '\n' {
					line++
				}
				i++
			}
			if i >= len(src) {
				return nil, fmt.Errorf("line %d: unterminated string", l0)
			}
			toks = append(toks, token{kind: '"', text: string(src[start:i]), line: l0, off: o0})
		default:
			return nil, fmt.Errorf("line %d: unexpected character %q", line, c)
		}
	}
	return toks, nil
}

// Parse parses GDT source.
func Parse(src []byte) (*File, error) {
	f := &File{CRLF: bytes.Contains(src, []byte("\r\n")), raw: src}
	lineStart := func(off int) int {
		for off > 0 && src[off-1] != '\n' {
			off--
		}
		return off
	}
	lineEnd := func(off int) int {
		for off < len(src) && src[off] != '\n' {
			off++
		}
		if off < len(src) {
			off++
		}
		return off
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := 0
	expect := func(k byte) (token, error) {
		if p >= len(toks) {
			return token{}, fmt.Errorf("unexpected end of file, expected %q", k)
		}
		t := toks[p]
		if t.kind != k {
			return t, fmt.Errorf("line %d: expected %q, got %q", t.line, k, t.text)
		}
		p++
		return t, nil
	}
	if len(toks) == 0 {
		f.raw = nil // empty or comment-only: render a fresh file
		return f, nil
	}
	open, err := expect('{')
	if err != nil {
		return nil, err
	}
	for p < len(toks) && toks[p].kind != '}' {
		nameTok, err := expect('"')
		if err != nil {
			return nil, err
		}
		a := &Asset{Name: nameTok.text, Line: nameTok.line, start: lineStart(nameTok.off)}
		if p >= len(toks) {
			return nil, fmt.Errorf("line %d: asset %q has no type", nameTok.line, a.Name)
		}
		switch toks[p].kind {
		case '(':
			p++
			t, err := expect('"')
			if err != nil {
				return nil, err
			}
			a.Type = strings.TrimSuffix(t.text, ".gdf")
			if _, err := expect(')'); err != nil {
				return nil, err
			}
		case '[':
			p++
			t, err := expect('"')
			if err != nil {
				return nil, err
			}
			a.Parent = t.text
			if _, err := expect(']'); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("line %d: asset %q: expected ( type ) or [ parent ]", toks[p].line, a.Name)
		}
		if _, err := expect('{'); err != nil {
			return nil, err
		}
		for p < len(toks) && toks[p].kind == '"' {
			k := toks[p]
			p++
			v, err := expect('"')
			if err != nil {
				return nil, fmt.Errorf("asset %q key %q: %w", a.Name, k.text, err)
			}
			a.Fields = append(a.Fields, Field{Key: k.text, Value: v.text})
		}
		closeTok, err := expect('}')
		if err != nil {
			return nil, fmt.Errorf("asset %q: %w", a.Name, err)
		}
		a.end = lineEnd(closeTok.off)
		f.Assets = append(f.Assets, a)
	}
	last, err := expect('}')
	if err != nil {
		return nil, err
	}
	f.closeOff = lineStart(last.off)
	// Surgical writes splice whole lines. If an asset shares a line with a brace
	// or another asset (a one-line or minified GDT), fall back to rendering the
	// whole file in APE's layout rather than splicing overlapping spans.
	pos := lineEnd(open.off)
	for _, a := range f.Assets {
		if a.start < pos || a.end > f.closeOff {
			f.raw = nil
			break
		}
		pos = a.end
	}
	return f, nil
}

// Bytes renders the file. A parsed file keeps its source byte for byte: only
// assets changed since parsing are re-rendered (in APE's layout), and new ones
// are inserted before the closing brace — so an edit never reformats the rest of
// a hand-formatted GDT. A new file is rendered entirely in APE's layout.
func (f *File) Bytes() []byte {
	nl := "\n"
	if f.CRLF {
		nl = "\r\n"
	}
	var b strings.Builder
	if f.raw == nil {
		b.WriteString("{" + nl)
		for _, a := range f.Assets {
			renderAsset(&b, a, nl)
		}
		b.WriteString("}" + nl)
		return []byte(b.String())
	}
	pos := 0
	var added []*Asset
	for _, a := range f.Assets {
		if a.start < 0 {
			added = append(added, a)
			continue
		}
		b.Write(f.raw[pos:a.start])
		if a.dirty {
			renderAsset(&b, a, nl)
		} else {
			b.Write(f.raw[a.start:a.end])
		}
		pos = a.end
	}
	b.Write(f.raw[pos:f.closeOff])
	for _, a := range added {
		renderAsset(&b, a, nl)
	}
	b.Write(f.raw[f.closeOff:])
	return []byte(b.String())
}

func renderAsset(b *strings.Builder, a *Asset, nl string) {
	if a.Parent != "" {
		fmt.Fprintf(b, "\t\"%s\" [ \"%s\" ]%s", a.Name, a.Parent, nl)
	} else {
		fmt.Fprintf(b, "\t\"%s\" ( \"%s.gdf\" )%s", a.Name, a.Type, nl)
	}
	b.WriteString("\t{" + nl)
	for _, fl := range a.Fields {
		fmt.Fprintf(b, "\t\t\"%s\" \"%s\"%s", fl.Key, fl.Value, nl)
	}
	b.WriteString("\t}" + nl)
}

// Save writes the file atomically (temp file + rename), keeping one backup of
// the previous content as <file>.bak when the file already existed. It refuses
// to write output that doesn't parse back to the same assets.
func (f *File) Save() error {
	out := f.Bytes()
	back, err := Parse(out)
	if err != nil {
		return fmt.Errorf("refusing to write %s: the result doesn't parse (%v)", f.Path, err)
	}
	if len(back.Assets) != len(f.Assets) {
		return fmt.Errorf("refusing to write %s: %d assets in memory, %d in the rendered file", f.Path, len(f.Assets), len(back.Assets))
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	if old, err := os.ReadFile(f.Path); err == nil {
		if err := writeAtomic(f.Path+".bak", old); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	return writeAtomic(f.Path, out)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".t7kb-gdt-*")
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmp)
	_, werr := w.Write(data)
	if ferr := w.Flush(); werr == nil {
		werr = ferr
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path) // fails on Windows if another program holds the file
	}
	if werr != nil {
		os.Remove(tmp.Name())
	}
	return werr
}

// Quote turns a real string into its GDT file form (backslashes and quotes escaped).
func Quote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// Unquote turns a GDT file-form value back into the real string.
func Unquote(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(s)
}
