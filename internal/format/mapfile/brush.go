package mapfile

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is what a brush block holds.
type Kind int

const (
	KindEmpty Kind = iota // no face and no patch: the stub brush of a point entity
	KindFaces             // a classic brush: one line per face
	KindCurve             // a "curve" patch
	KindMesh              // a "mesh" patch (terrain)
)

// Vec3 is a point.
type Vec3 [3]float64

// Face is one plane of a classic brush: three points on it, then its material
// and lightmap, each followed by six texture numbers, then optional decal
// settings (decalLayerSort "…" decalEditorSort n).
type Face struct {
	Points      [3]Vec3
	Material    string
	Tex         [6]float64
	Lightmap    string
	LightmapTex [6]float64
	Extra       string // what follows the lightmap numbers, verbatim
}

// Patch is a curve or mesh: a grid of vertices.
type Patch struct {
	Kind     Kind
	Material string
	Lightmap string
	Dims     [4]int     // rows, columns, then two numbers Radiant keeps
	Settings []string   // the other lines before the rows: toolFlags;, contents …;, layer "…", smoothing …
	Rows     [][]Vertex // Dims[0] rows of Dims[1] vertices
	Trailer  []string   // lines after the rows, such as decalLayerSort "…"
}

// Vertex is one patch vertex line: "v x y z [c r g b a] t u v lu lv [f n]".
// The colour (0-255 each) is what terrain material blending reads; a vertex
// without one is opaque white.
type Vertex struct {
	Pos      Vec3
	Color    [4]uint8
	HasColor bool
	UV       [2]float64
	LM       [2]float64
	Flags    int // the "f" value, 0 when absent
}

// Guid is the brush's guid, braces included.
func (b *Brush) Guid() string { lines, _ := b.split(); return field(lines, "guid") }

// Layer is the brush's own layer, "" when it has none. A patch may keep it
// inside its block.
func (b *Brush) Layer() string { return field(b.ownThenBlock(), "layer") }

// Contents is the brush's "contents …;" value without the semicolon
// (nonColliding, "weaponClip detail"…), "" for a structural brush. A patch
// keeps it inside its block.
func (b *Brush) Contents() string {
	return strings.TrimSuffix(field(b.ownThenBlock(), "contents"), ";")
}

func (b *Brush) ownThenBlock() []string {
	own, block := b.split()
	return append(own, block...)
}

// Kind tells a classic brush from a patch.
func (b *Brush) Kind() Kind {
	lines, _ := b.split()
	for _, l := range lines {
		switch {
		case l == "curve":
			return KindCurve
		case l == "mesh":
			return KindMesh
		case strings.HasPrefix(l, "("):
			return KindFaces
		}
	}
	return KindEmpty
}

// Faces parses a classic brush's face lines.
func (b *Brush) Faces() ([]Face, error) {
	lines, _ := b.split()
	var out []Face
	for _, l := range lines {
		if !strings.HasPrefix(l, "(") {
			continue
		}
		f, err := parseFace(l)
		if err != nil {
			return nil, fmt.Errorf("brush %s: %w", b.Guid(), err)
		}
		out = append(out, f)
	}
	return out, nil
}

// Patch parses a curve or mesh brush.
func (b *Brush) Patch() (*Patch, error) {
	kind := b.Kind()
	if kind != KindCurve && kind != KindMesh {
		return nil, fmt.Errorf("brush %s is not a patch", b.Guid())
	}
	_, block := b.split()
	p, err := parsePatch(block)
	if err != nil {
		return nil, fmt.Errorf("brush %s: %w", b.Guid(), err)
	}
	p.Kind = kind
	return p, nil
}

// split returns the brush's own lines and the lines inside its nested patch
// block, both trimmed and without the block's braces.
func (b *Brush) split() (own, block []string) {
	depth := 0
	for _, raw := range b.Body {
		l := trim(raw)
		switch {
		case l == "{":
			depth++
		case l == "}":
			depth--
		case depth == 0:
			own = append(own, l)
		default:
			block = append(block, l)
		}
	}
	return own, block
}

// parseFace reads "( x y z ) ( x y z ) ( x y z ) mat a b c d e f lm a b c d e f".
func parseFace(l string) (Face, error) {
	t := strings.Fields(l)
	var f Face
	if len(t) < 29 {
		return f, fmt.Errorf("face has %d fields, want at least 29: %q", len(t), l)
	}
	f.Extra = strings.Join(t[29:], " ")
	for i := range 3 {
		at := i * 5
		if t[at] != "(" || t[at+4] != ")" {
			return f, fmt.Errorf("face point %d is not ( x y z ): %q", i+1, l)
		}
		if err := floats(t[at+1:at+4], f.Points[i][:]); err != nil {
			return f, err
		}
	}
	f.Material, f.Lightmap = t[15], t[22]
	if err := floats(t[16:22], f.Tex[:]); err != nil {
		return f, err
	}
	return f, floats(t[23:29], f.LightmapTex[:])
}

func parsePatch(lines []string) (*Patch, error) {
	p := &Patch{}
	i, err := p.header(lines)
	if err != nil {
		return nil, err
	}
	for ; i < len(lines) && lines[i] == "("; i++ {
		row, next, err := parseRow(lines, i+1)
		if err != nil {
			return nil, err
		}
		p.Rows = append(p.Rows, row)
		i = next
	}
	p.Trailer = lines[i:]
	if len(p.Rows) != p.Dims[0] {
		return nil, fmt.Errorf("patch has %d rows, its header says %d", len(p.Rows), p.Dims[0])
	}
	for r, row := range p.Rows {
		if len(row) != p.Dims[1] {
			return nil, fmt.Errorf("patch row %d has %d vertices, its header says %d", r+1, len(row), p.Dims[1])
		}
	}
	return p, nil
}

// header reads the lines before the first row: a single word is the material,
// then the lightmap; any other line (with a space, or ending in ";") is a
// setting; four integers are the dimensions. It returns the first row's index.
func (p *Patch) header(lines []string) (int, error) {
	var names []string
	for i, l := range lines {
		var d [4]int
		if n, _ := scanf(l, "%d %d %d %d", &d[0], &d[1], &d[2], &d[3]); n == 4 {
			if len(names) != 2 {
				return 0, fmt.Errorf("patch has %d material/lightmap lines before its dimensions, want 2", len(names))
			}
			p.Material, p.Lightmap, p.Dims = names[0], names[1], d
			return i + 1, nil
		}
		if strings.ContainsAny(l, " \t") || strings.HasSuffix(l, ";") {
			p.Settings = append(p.Settings, l)
			continue
		}
		names = append(names, l)
	}
	return 0, fmt.Errorf("patch ends before its dimensions line")
}

// parseRow reads vertex lines from lines[i] up to the row's ")".
func parseRow(lines []string, i int) ([]Vertex, int, error) {
	var row []Vertex
	for ; i < len(lines); i++ {
		if lines[i] == ")" {
			return row, i, nil
		}
		v, err := parseVertex(lines[i])
		if err != nil {
			return nil, 0, err
		}
		row = append(row, v)
	}
	return nil, 0, fmt.Errorf("patch row is never closed")
}

// parseVertex reads "v x y z [c r g b a] t u v lu lv [f n]".
func parseVertex(l string) (Vertex, error) {
	t := strings.Fields(l)
	v := Vertex{Color: [4]uint8{255, 255, 255, 255}}
	bad := fmt.Errorf("patch vertex is not v x y z [c r g b a] t u v lu lv [f n]: %q", l)
	if len(t) < 9 || t[0] != "v" {
		return v, bad
	}
	if err := floats(t[1:4], v.Pos[:]); err != nil {
		return v, err
	}
	t = t[4:]
	if t[0] == "c" {
		if len(t) < 5 || colors(t[1:5], &v.Color) != nil {
			return v, bad
		}
		v.HasColor, t = true, t[5:]
	}
	if len(t) < 5 || t[0] != "t" {
		return v, bad
	}
	if err := floats(t[1:3], v.UV[:]); err != nil {
		return v, err
	}
	if err := floats(t[3:5], v.LM[:]); err != nil {
		return v, err
	}
	switch t = t[5:]; {
	case len(t) == 0:
		return v, nil
	case len(t) == 2 && t[0] == "f":
		n, err := strconv.Atoi(t[1])
		if err != nil {
			return v, bad
		}
		v.Flags = n
		return v, nil
	}
	return v, bad
}

func colors(src []string, dst *[4]uint8) error {
	for i, s := range src {
		n, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return err
		}
		dst[i] = uint8(n)
	}
	return nil
}

func floats(src []string, dst []float64) error {
	for i, s := range src {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("%q is not a number", s)
		}
		dst[i] = f
	}
	return nil
}

func scanf(s, format string, args ...any) (int, error) { return fmt.Sscanf(s, format, args...) }
