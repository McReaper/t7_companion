package shader

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testdata/v7 is a shader cache in miniature: shaders compiled from HLSL
// written for t7_dxbc's tests, named as the mod tools' cache names them.
// sample.hlsl's pixel shader comes in two permutations of one program (A and
// B) and one of another (C), and shares hash A with its vertex shader;
// fx_count.hlsl was in a subdirectory (fx/count.hlsl); material.hlsl reads
// three of its four $Globals.
const cache = "testdata/v7"

func TestParseName(t *testing.T) {
	cases := []struct {
		in   string
		want Name
		ok   bool
	}{
		{"techsetdef_unlit.hlsl_ps_main_CJK6WNS46DJOAEU6RKXSYFZY5D", Name{"techsetdef_unlit.hlsl", "ps", "CJK6WNS46DJOAEU6RKXSYFZY5D"}, true},
		{"fx_count.hlsl_cs_main_D", Name{"fx_count.hlsl", "cs", "D"}, true},
		{"a.fx_gs_main_X", Name{"a.fx", "gs", "X"}, true},
		{"x.hlsl_ps_main_", Name{}, false},   // no hash
		{"x.hlsl_ps_entry_H", Name{}, false}, // not an entry point the cache uses
		{"x.hlsl_zz_main_H", Name{}, false},  // no such stage
		{"_ps_main_H", Name{}, false},        // no source
		{"ps_main_H", Name{}, false},
		{"readme", Name{}, false},
	}
	for _, c := range cases {
		got, ok := ParseName(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseName(%q) = %+v, %v; want %+v, %v", c.in, got, ok, c.want, c.ok)
		}
		if ok && got.File() != c.in {
			t.Errorf("%q: File() = %q", c.in, got.File())
		}
	}
}

func TestSourceName(t *testing.T) {
	for in, want := range map[string]string{
		"techsetdef_unlit.hlsl":        "techsetdef_unlit.hlsl",
		"specialty/thermal_scope.hlsl": "specialty_thermal_scope.hlsl",
		`Postfx\Downsample.hlsl`:       "postfx_downsample.hlsl",
	} {
		if got := SourceName(in); got != want {
			t.Errorf("SourceName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFiles(t *testing.T) {
	dir := copyCache(t)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "x.hlsl_ps_main_DIR"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for src, ns := range files {
		got = append(got, src+":"+strings.Repeat("*", len(ns)))
	}
	slices.Sort(got)
	if want := []string{"fx_count.hlsl:*", "material.hlsl:*", "sample.hlsl:****"}; !slices.Equal(got, want) {
		t.Errorf("Files = %v, want %v", got, want)
	}
	if _, err := Files(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing cache is no error")
	}
}

func TestVariants(t *testing.T) {
	files, err := Files(cache)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := Variants(cache, files["sample.hlsl"], "ps")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || len(ps[0].Files) != 2 || len(ps[1].Files) != 1 || ps[1].Files[0] != "sample.hlsl_ps_main_CCCCCCCCCCCCCCCCCCCCCCCCCC" {
		t.Fatalf("ps variants: %+v", ps)
	}
	first := ps[0]
	if first.Stage != "ps" || first.Instructions == 0 || !slices.Equal(first.Files, []string{
		"sample.hlsl_ps_main_AAAAAAAAAAAAAAAAAAAAAAAAAA", "sample.hlsl_ps_main_BBBBBBBBBBBBBBBBBBBBBBBBBB"}) {
		t.Errorf("first variant: %+v", first)
	}
	if !slices.Equal(first.Resources, []string{"colorSampler@s0", "colorMap@t0", "Material@cb0"}) ||
		!slices.Equal(first.Inputs, []string{"SV_Position0", "TEXCOORD0", "NORMAL0"}) {
		t.Errorf("what the first variant reads: %+v", first)
	}
	all, err := Variants(cache, files["sample.hlsl"], "")
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, v := range all {
		stages = append(stages, v.Stage)
	}
	if !slices.Equal(stages, []string{"ps", "ps", "vs"}) {
		t.Errorf("every stage: %v", stages)
	}
	m, err := Variants(cache, files["material.hlsl"], "ps")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || !slices.Equal(m[0].Globals, []string{"colorTint", "uvScroll", "glow"}) {
		t.Errorf("the $Globals read: %+v", m)
	}
}

func TestVariantsOfAnUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	n := Name{"broken.hlsl", "ps", "H"}
	if err := os.WriteFile(filepath.Join(dir, n.File()), []byte("not a shader"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Variants(dir, []Name{n}, ""); err == nil || !strings.Contains(err.Error(), n.File()) {
		t.Errorf("err = %v", err)
	}
	if _, err := Variants(dir, []Name{{"gone.hlsl", "ps", "H"}}, ""); err == nil {
		t.Error("a missing file is no error")
	}
}

func TestDecompile(t *testing.T) {
	src, err := Decompile(cache, "sample.hlsl_ps_main_AAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil || !strings.Contains(src, "void ps_main(") {
		t.Fatalf("err %v, HLSL:\n%s", err, src)
	}
	for _, bad := range []string{"../v7/sample.hlsl_ps_main_AAAAAAAAAAAAAAAAAAAAAAAAAA", "sample.hlsl", "x.hlsl_ps_main_MISSING"} {
		if _, err := Decompile(cache, bad); err == nil {
			t.Errorf("Decompile(%q): no error", bad)
		}
	}
}

// A shader stripped of its reflection decompiles in part: the HLSL written,
// and an error naming what isn't.
func TestDecompileInPart(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(cache, "sample.hlsl_ps_main_AAAAAAAAAAAAAAAAAAAAAAAAAA"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	name := "stripped.hlsl_ps_main_X"
	if err := os.WriteFile(filepath.Join(dir, name), bytes.Replace(b, []byte("RDEF"), []byte("XDEF"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := Decompile(dir, name)
	if !errors.Is(err, ErrIncomplete) || !strings.Contains(err.Error(), "without reflection") || !strings.Contains(src, "ps_main") {
		t.Errorf("err %v, HLSL %d bytes", err, len(src))
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("DXBC"), 0o644); err != nil {
		t.Fatal(err)
	}
	if src, err := Decompile(dir, name); err == nil || errors.Is(err, ErrIncomplete) || src != "" {
		t.Errorf("an unreadable file: %q, %v", src, err)
	}
}

// copyCache copies testdata/v7 to a temporary directory the test may change.
func copyCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(cache, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
