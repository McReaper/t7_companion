// Package mapfile reads Radiant's .map text: a Black Ops 3 map or prefab file,
// and the entity text Radiant writes and pastes (its clipboard, and what the
// t7_radiant_bridge plugin reads out of the open map), which is the same
// grammar without the header.
//
//	iwmap 4                                   header: the version, then
//	"script_startingnumber" 0                 map keys and layer lines
//	"000_Global" flags expanded  active
//	// entity 0
//	{
//	guid "{…}"                                an entity: its guid, an optional
//	layer "000_Global/Zones"                  layer, then its keys
//	"classname" "worldspawn"
//	// brush 0
//	{
//	 guid "{…}"                               a brush: guid, optional layer and
//	 contents nonColliding;                   contents, then one line per face:
//	 ( x y z ) ( x y z ) ( x y z ) <material> <6 numbers> <lightmap> <6 numbers>
//	}
//	// brush 1
//	 {
//	  guid "{…}"
//	  curve                                   or a patch, "curve" or "mesh"
//	  {                                       (terrain), whose block holds flag
//	  toolFlags;                              lines, the material and lightmap,
//	   <material>                             "<rows> <columns> <a> <b>", the
//	   lightmap_gray                          rows of "v x y z t u v lu lv"
//	   3 3 16 1                               vertices, then trailing lines
//	   (                                      such as decalLayerSort
//	    v x y z t u v lu lv
//	   )
//	  }
//	 }
//	}
//
// Parsing is lossless: every line keeps its exact bytes, line end included
// (install files mix CRLF and LF, and carry trailing spaces), so Bytes returns
// the input unchanged and the typed accessors (Keys, Faces, Patch...) read the
// lines on demand. Braces stand on lines of their own; values never hold a
// double quote or a line break.
package mapfile

import (
	"bytes"
	"fmt"
	"strings"
)

// Map is a parsed .map file or entity text.
type Map struct {
	Header   []string // lines before the first entity, with their line ends
	Entities []*Entity
	Trailer  []string // lines after the last entity
}

// Entity is one "{ … }" block at the top level.
type Entity struct {
	Lead  []string // comment lines just before its "{" ("// entity 0")
	Open  string
	Items []Item // its own lines and its brushes, in text order
	Close string
}

// Item is one element of an entity: a line of its own, or a brush.
type Item struct {
	Line  string
	Brush *Brush
}

// Brush is one "{ … }" block inside an entity: a classic brush or a patch.
type Brush struct {
	Lead  []string // comment lines just before its "{" ("// brush 0")
	Open  string
	Body  []string // every line between its braces, nested patch blocks included
	Close string
}

// Parse reads .map text. It fails only on unbalanced braces.
func Parse(b []byte) (*Map, error) {
	p := parser{m: &Map{}}
	for n, line := range splitLines(b) {
		if err := p.line(line, n+1); err != nil {
			return nil, err
		}
	}
	if p.depth != 0 {
		return nil, fmt.Errorf("mapfile: %d brace(s) still open at the end (block opened on line %d)", p.depth, p.openedAt)
	}
	p.m.Trailer = p.pending
	return p.m, nil
}

// Bytes returns the text the map was parsed from.
func (m *Map) Bytes() []byte {
	var buf bytes.Buffer
	write := func(lines []string) {
		for _, l := range lines {
			buf.WriteString(l)
		}
	}
	write(m.Header)
	for _, e := range m.Entities {
		e.writeTo(&buf)
	}
	write(m.Trailer)
	return buf.Bytes()
}

// Bytes returns the entity's text, its lead comments included.
func (e *Entity) Bytes() []byte {
	var buf bytes.Buffer
	e.writeTo(&buf)
	return buf.Bytes()
}

func (e *Entity) writeTo(buf *bytes.Buffer) {
	write := func(lines []string) {
		for _, l := range lines {
			buf.WriteString(l)
		}
	}
	write(e.Lead)
	buf.WriteString(e.Open)
	for _, it := range e.Items {
		if it.Brush == nil {
			buf.WriteString(it.Line)
			continue
		}
		write(it.Brush.Lead)
		buf.WriteString(it.Brush.Open)
		write(it.Brush.Body)
		buf.WriteString(it.Brush.Close)
	}
	buf.WriteString(e.Close)
}

type parser struct {
	m        *Map
	depth    int
	openedAt int
	pending  []string // top-level lines not yet given to the header or an entity
	entity   *Entity
	brush    *Brush
}

func (p *parser) line(line string, n int) error {
	t := trim(line)
	switch p.depth {
	case 0:
		return p.top(line, t, n)
	case 1:
		p.inEntity(line, t)
	default:
		p.inBrush(line, t)
	}
	return nil
}

func (p *parser) top(line, t string, n int) error {
	switch t {
	case "{":
		p.entity = &Entity{Open: line}
		lead := trailingComments(p.pending)
		if len(p.m.Entities) == 0 {
			p.m.Header = p.pending[:len(p.pending)-len(lead)]
			p.entity.Lead = lead
		} else {
			p.entity.Lead = p.pending
		}
		p.pending = nil
		p.m.Entities = append(p.m.Entities, p.entity)
		p.depth, p.openedAt = 1, n
	case "}":
		return fmt.Errorf("mapfile: line %d closes a block that was never opened", n)
	default:
		p.pending = append(p.pending, line)
	}
	return nil
}

func (p *parser) inEntity(line, t string) {
	switch t {
	case "{":
		items := p.entity.Items
		k := len(items)
		// a brush item's Line is empty, so the walk back stops at it
		for k > 0 && strings.HasPrefix(trim(items[k-1].Line), "//") {
			k--
		}
		p.brush = &Brush{Open: line}
		for _, it := range items[k:] {
			p.brush.Lead = append(p.brush.Lead, it.Line)
		}
		p.entity.Items = append(items[:k], Item{Brush: p.brush})
		p.depth = 2
	case "}":
		p.entity.Close = line
		p.entity, p.depth = nil, 0
	default:
		p.entity.Items = append(p.entity.Items, Item{Line: line})
	}
}

func (p *parser) inBrush(line, t string) {
	switch {
	case t == "{":
		p.depth++
	case t == "}" && p.depth == 2:
		p.brush.Close = line
		p.brush, p.depth = nil, 1
		return
	case t == "}":
		p.depth--
	}
	p.brush.Body = append(p.brush.Body, line)
}

// trailingComments returns the "//" lines that end lines.
func trailingComments(lines []string) []string {
	k := len(lines)
	for k > 0 && strings.HasPrefix(trim(lines[k-1]), "//") {
		k--
	}
	return lines[k:]
}

// splitLines cuts b after each '\n', keeping the line ends. It loops over
// what bytes.SplitAfter gives, so no index of its own can stall it (a mutant
// of a hand-advanced one appended the same line until CI's runner ran out of
// memory).
func splitLines(b []byte) []string {
	var out []string
	for _, l := range bytes.SplitAfter(b, []byte{'\n'}) {
		if len(l) > 0 {
			out = append(out, string(l))
		}
	}
	return out
}

func trim(s string) string { return strings.Trim(s, " \t\r\n") }
