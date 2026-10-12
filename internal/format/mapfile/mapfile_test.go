package mapfile

import (
	"bytes"
	"strings"
	"testing"
)

// sample holds every construct the install's maps use.
const sample = `iwmap 4
"script_startingnumber" 0
"000_Global" flags expanded  active
"000_Global/Zones" flags hidden
"The Map" flags expanded
"_prefabs" flags prefab expanded
// entity 0
{
guid "{11111111-0000-0000-0000-000000000000}"
"classname" "worldspawn"
"lutmaterial" "luts_t7_default"
// brush 0
{
 guid "{22222222-0000-0000-0000-000000000000}"
layer "000_Global/Zones"
  contents nonColliding;
 ( 32 32 0 ) ( -32 32 0 ) ( -32 -32 0 ) caulk 64 64 0 0 0 0 lightmap_gray 16384 16384 0 0 0 0
 ( -32 -32 64 ) ( -32 32 64 ) ( 32 32 64 ) t7_decal_raindrops 64 64 1.5 -2 90 0 lightmap_gray 16384 16384 112 0 0 0 decalLayerSort "Debris (top)" decalEditorSort -1
}
// brush 1
 {
  guid "{33333333-0000-0000-0000-000000000000}"
  curve
  {
layer "The Map"
  contents weaponClip detail;
  toolFlags;
   t7_metal_worn_iron_dark
   lightmap_gray
   smoothing smoothing_hard
   2 3 16 1
   (
	v -96 -64 -16 t 0 0 0 0
	v 0 -64 200 c 255 128 0 64 t 128 0 8 0 f 1
	v 96 -64 -16 t 256 0 16 0
   )
   (
	v -96 64 -16 t 0 128 0 8
	v 0 64 200 t 128 128 8 8
	v 96 64 -16 t 256 128 16 8
   )
   decalLayerSort "Signage - New"
   decalEditorSort -1
  }
 }
// brush 2
 {
  guid "{44444444-0000-0000-0000-000000000000}"
  mesh
  {
  toolFlags;
   t7_dirt_grassy_forest_ground
   lightmap_gray
   1 2 0 8
   (
	v -10 -10 -3 t -1 1 -2 2
	v -10 10 -3 t -1 -1 -2 -2
   )
  }
 }
}
// entity 1
{
guid "{55555555-0000-0000-0000-000000000000}"
layer "000_Global/Zones"
"classname" "reflection_probe"
"origin" "44 108 4"
"name" "probe"
// brush 0
{
 guid "{66666666-0000-0000-0000-000000000000}"
}
}
`

func parse(t *testing.T, s string) *Map {
	t.Helper()
	m, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoundTrip(t *testing.T) {
	for name, s := range map[string]string{
		"lf":            sample,
		"crlf":          strings.ReplaceAll(sample, "\n", "\r\n"),
		"no final eol":  strings.TrimSuffix(sample, "\n"),
		"mixed eol":     strings.Replace(sample, "\n", "\r\n", 7),
		"entity text":   sample[strings.Index(sample, "{"):],
		"trailing text": sample + "\n// end\n",
		"empty":         "",
	} {
		if got := parse(t, s).Bytes(); !bytes.Equal(got, []byte(s)) {
			t.Errorf("%s: Bytes differs from the input:\n%s", name, got)
		}
	}
}

func TestHeader(t *testing.T) {
	m := parse(t, sample)
	if m.Version() != 4 {
		t.Errorf("Version = %d, want 4", m.Version())
	}
	if kv := m.Keys(); len(kv) != 1 || kv[0] != (KeyValue{"script_startingnumber", "0"}) {
		t.Errorf("Keys = %v", kv)
	}
	want := []Layer{{"000_Global", []string{"expanded", "active"}}, {"000_Global/Zones", []string{"hidden"}}, {"The Map", []string{"expanded"}}, {"_prefabs", []string{"prefab", "expanded"}}}
	got := m.Layers()
	if len(got) != len(want) {
		t.Fatalf("Layers = %v", got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || strings.Join(got[i].Flags, ",") != strings.Join(want[i].Flags, ",") {
			t.Errorf("layer %d = %v, want %v", i, got[i], want[i])
		}
	}
	if len(m.Entities) != 2 || len(m.Entities[0].Lead) != 1 || trim(m.Entities[0].Lead[0]) != "// entity 0" {
		t.Errorf("entities %d, first lead %q", len(m.Entities), m.Entities[0].Lead)
	}
}

func TestEntityText(t *testing.T) {
	m := parse(t, "{\nguid \"{A}\"\n\"classname\" \"script_struct\"\n}\n{\n\"classname\" \"light\"\n}\n")
	if m.Version() != 0 || len(m.Header) != 0 || len(m.Entities) != 2 || len(m.Entities[0].Lead) != 0 {
		t.Fatalf("version %d, header %q, %d entities", m.Version(), m.Header, len(m.Entities))
	}
	if e := m.Entities[0]; e.Guid() != "{A}" || e.Class() != "script_struct" {
		t.Errorf("guid %q class %q", e.Guid(), e.Class())
	}
	if e := m.Entities[1]; e.Guid() != "" || e.Layer() != "" || len(e.Brushes()) != 0 {
		t.Errorf("a bare entity has guid %q, layer %q, %d brushes", e.Guid(), e.Layer(), len(e.Brushes()))
	}
}

// Lines that only look like keys are not keys.
func TestNotKeys(t *testing.T) {
	e := parse(t, "{\n\"a\" \"b\" c\n\"unterminated\n\"x\" y\n\"k\" \"v\"\n}\n").Entities[0]
	if kv := e.Keys(); len(kv) != 1 || kv[0] != (KeyValue{"k", "v"}) {
		t.Errorf("Keys = %v, want only k", kv)
	}
}

func TestEntityAccessors(t *testing.T) {
	m := parse(t, sample)
	w, p := m.Entities[0], m.Entities[1]
	if w.Guid() != "{11111111-0000-0000-0000-000000000000}" || w.Class() != "worldspawn" || w.Layer() != "" {
		t.Errorf("worldspawn guid %q class %q layer %q", w.Guid(), w.Class(), w.Layer())
	}
	if len(w.Brushes()) != 3 || len(p.Brushes()) != 1 {
		t.Errorf("brushes %d and %d, want 3 and 1", len(w.Brushes()), len(p.Brushes()))
	}
	if p.Layer() != "000_Global/Zones" {
		t.Errorf("probe layer %q", p.Layer())
	}
	keys := p.Keys()
	if len(keys) != 3 || keys[0].Key != "classname" || keys[2] != (KeyValue{"name", "probe"}) {
		t.Errorf("probe keys %v", keys)
	}
	if v, ok := p.Key("origin"); !ok || v != "44 108 4" {
		t.Errorf("origin %q %v", v, ok)
	}
	if _, ok := p.Key("missing"); ok {
		t.Error("a missing key is found")
	}
	if lead := w.Brushes()[1].Lead; len(lead) != 1 || trim(lead[0]) != "// brush 1" {
		t.Errorf("brush 1 lead %q", lead)
	}
}

func TestFaces(t *testing.T) {
	b := parse(t, sample).Entities[0].Brushes()[0]
	if b.Kind() != KindFaces || b.Guid() != "{22222222-0000-0000-0000-000000000000}" || b.Layer() != "000_Global/Zones" || b.Contents() != "nonColliding" {
		t.Fatalf("kind %v guid %q layer %q contents %q", b.Kind(), b.Guid(), b.Layer(), b.Contents())
	}
	fs, err := b.Faces()
	if err != nil || len(fs) != 2 {
		t.Fatalf("faces %v %v", fs, err)
	}
	f := fs[1]
	if f.Points != [3]Vec3{{-32, -32, 64}, {-32, 32, 64}, {32, 32, 64}} || f.Material != "t7_decal_raindrops" || f.Tex != [6]float64{64, 64, 1.5, -2, 90, 0} {
		t.Errorf("face %+v", f)
	}
	if f.Lightmap != "lightmap_gray" || f.LightmapTex != [6]float64{16384, 16384, 112, 0, 0, 0} || f.Extra != `decalLayerSort "Debris (top)" decalEditorSort -1` {
		t.Errorf("face lightmap %q %v extra %q", f.Lightmap, f.LightmapTex, f.Extra)
	}
	if fs[0].Extra != "" {
		t.Errorf("a face without decal settings has extra %q", fs[0].Extra)
	}
	if _, err := b.Patch(); err == nil {
		t.Error("Patch on a classic brush")
	}
}

func TestCurve(t *testing.T) {
	b := parse(t, sample).Entities[0].Brushes()[1]
	if b.Kind() != KindCurve || b.Layer() != "The Map" || b.Contents() != "weaponClip detail" {
		t.Fatalf("kind %v layer %q contents %q", b.Kind(), b.Layer(), b.Contents())
	}
	p, err := b.Patch()
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindCurve || p.Material != "t7_metal_worn_iron_dark" || p.Lightmap != "lightmap_gray" || p.Dims != [4]int{2, 3, 16, 1} {
		t.Errorf("patch %+v", p)
	}
	if strings.Join(p.Settings, "|") != `layer "The Map"|contents weaponClip detail;|toolFlags;|smoothing smoothing_hard` {
		t.Errorf("settings %q", p.Settings)
	}
	if strings.Join(p.Trailer, "|") != `decalLayerSort "Signage - New"|decalEditorSort -1` {
		t.Errorf("trailer %q", p.Trailer)
	}
	v := p.Rows[0][1]
	if v.Pos != (Vec3{0, -64, 200}) || !v.HasColor || v.Color != [4]uint8{255, 128, 0, 64} || v.UV != [2]float64{128, 0} || v.LM != [2]float64{8, 0} || v.Flags != 1 {
		t.Errorf("coloured vertex %+v", v)
	}
	if w := p.Rows[1][2]; w.HasColor || w.Color != [4]uint8{255, 255, 255, 255} || w.Flags != 0 {
		t.Errorf("plain vertex %+v", w)
	}
	if _, err := b.Faces(); err != nil {
		t.Errorf("Faces on a patch: %v", err)
	}
}

func TestMeshAndStub(t *testing.T) {
	m := parse(t, sample)
	p, err := m.Entities[0].Brushes()[2].Patch()
	if err != nil || p.Kind != KindMesh || len(p.Rows) != 1 || len(p.Rows[0]) != 2 || m.Entities[0].Brushes()[2].Contents() != "" {
		t.Fatalf("mesh %+v %v", p, err)
	}
	stub := m.Entities[1].Brushes()[0]
	if stub.Kind() != KindEmpty || stub.Guid() != "{66666666-0000-0000-0000-000000000000}" || stub.Layer() != "" || stub.Contents() != "" {
		t.Errorf("stub kind %v guid %q", stub.Kind(), stub.Guid())
	}
	if fs, err := stub.Faces(); err != nil || len(fs) != 0 {
		t.Errorf("stub faces %v %v", fs, err)
	}
}

func TestParseErrors(t *testing.T) {
	for name, s := range map[string]string{
		"unclosed entity": "{\n\"classname\" \"x\"\n",
		"unclosed brush":  "{\n{\n ( 0 0 0 )\n}\n",
		"stray close":     "iwmap 4\n}\n",
	} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	_, err := Parse([]byte("\n\n{\n{\n"))
	if err == nil || !strings.Contains(err.Error(), "2 brace(s) still open") || !strings.Contains(err.Error(), "line 3") {
		t.Errorf("unclosed error %v, want the count and the line the block opened on", err)
	}
}

func brushOf(t *testing.T, body string) *Brush {
	t.Helper()
	return parse(t, "{\n{\n"+body+"}\n}\n").Entities[0].Brushes()[0]
}

func TestFaceErrors(t *testing.T) {
	const tex = " mat 1 1 0 0 0 0 lm 1 1 0 0 0 0"
	for name, face := range map[string]string{
		"too short":     " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) mat 1 1 0 0",
		"no parenthese": " ( 0 0 0 ) [ 1 0 0 ] ( 0 1 0 )" + tex,
		"bad point":     " ( 0 x 0 ) ( 1 0 0 ) ( 0 1 0 )" + tex,
		"bad texture":   " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) mat 1 one 0 0 0 0 lm 1 1 0 0 0 0",
		"bad lightmap":  " ( 0 0 0 ) ( 1 0 0 ) ( 0 1 0 ) mat 1 1 0 0 0 0 lm 1 1 0 0 0 z",
	} {
		if _, err := brushOf(t, face+"\n").Faces(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestPatchErrors(t *testing.T) {
	head := "curve\n{\ntoolFlags;\nmat\nlm\n"
	row := "(\nv 0 0 0 t 0 0 0 0\n)\n"
	for name, block := range map[string]string{
		"no dimensions":      "curve\n{\nmat\nlm\n}\n",
		"one name":           "curve\n{\nmat\n1 1 0 0\n" + row + "}\n",
		"rows count":         head + "2 1 0 0\n" + row + "}\n",
		"columns count":      head + "1 2 0 0\n" + row + "}\n",
		"unclosed row":       head + "1 1 0 0\n(\nv 0 0 0 t 0 0 0 0\n}\n",
		"short vertex":       head + "1 1 0 0\n(\nv 0 0 0 t 0 0\n)\n}\n",
		"bad position":       head + "1 1 0 0\n(\nv 0 q 0 t 0 0 0 0\n)\n}\n",
		"no t":               head + "1 1 0 0\n(\nv 0 0 0 u 0 0 0 0\n)\n}\n",
		"bad uv":             head + "1 1 0 0\n(\nv 0 0 0 t 0 q 0 0\n)\n}\n",
		"bad lm":             head + "1 1 0 0\n(\nv 0 0 0 t 0 0 0 q\n)\n}\n",
		"bad colour":         head + "1 1 0 0\n(\nv 0 0 0 c 1 2 300 4 t 0 0 0 0\n)\n}\n",
		"short colour":       head + "1 1 0 0\n(\nv 0 0 0 c 1 2\n)\n}\n",
		"bad flag":           head + "1 1 0 0\n(\nv 0 0 0 t 0 0 0 0 f x\n)\n}\n",
		"unknown tail":       head + "1 1 0 0\n(\nv 0 0 0 t 0 0 0 0 q 1\n)\n}\n",
		"not a vertex":       head + "1 1 0 0\n(\nw 0 0 0 t 0 0 0 0\n)\n}\n",
		"mesh with no names": "mesh\n{\n1 1 0 0\n" + row + "}\n",
	} {
		if _, err := brushOf(t, block).Patch(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte(strings.ReplaceAll(sample, "\n", "\r\n")))
	f.Add([]byte("{\n{\ncurve\n{\nmat\nlm\n1 1 0 0\n(\nv 0 0 0 c 1 2 3 4 t 0 0 0 0 f 1\n)\n}\n}\n}\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := Parse(b)
		if err != nil {
			return
		}
		if !bytes.Equal(m.Bytes(), b) {
			t.Fatalf("Bytes differs from the input")
		}
		_, _, _ = m.Version(), m.Keys(), m.Layers()
		for _, e := range m.Entities {
			_, _, _, _ = e.Guid(), e.Layer(), e.Keys(), e.Class()
			for _, br := range e.Brushes() {
				_, _, _, _ = br.Guid(), br.Layer(), br.Contents(), br.Kind()
				_, _ = br.Faces()
				_, _ = br.Patch()
			}
		}
	})
}
