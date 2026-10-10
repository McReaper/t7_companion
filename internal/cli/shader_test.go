package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// shaderRoot is a fake mod-tools root with internal/shader's miniature cache
// and three material types: mtl_sample draws with sample.hlsl (and previews
// with a ToolsGfx source), mtl_two with sample.hlsl and fx/count.hlsl,
// mtl_gone with a source the cache doesn't have.
func shaderRoot(t *testing.T) string {
	t.Helper()
	root := fakeToolsRoot(t)
	t.Setenv("TA_TOOLS_PATH", "")
	cache := filepath.Join(root, "share", "assetconvert", "shaders", "pc", "v7")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "shader", "testdata", "v7")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	techsets := filepath.Join(root, "share", "raw", "techsetdefs_stable", "geometry")
	if err := os.MkdirAll(techsets, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"mtl_sample": `Globals() { category = "Geometry" }
Color( "colorTint" ) { value = "<colorTint>" }
float1( "hdrScale" ) { x = <scaleRGB> }
Technique( "lit" )
{
	vs = VertexShader() { source = "ToolsGfx/simple.hlsl" }
	ps = PixelShader() { source = "sample.hlsl" }
}
`,
		"mtl_two": `Technique( "lit" )
{
	vs = VertexShader() { source = "fx/count.hlsl" }
	ps = PixelShader() { source = "sample.hlsl" }
}
`,
		"mtl_gone": `Technique( "lit" ) { ps = PixelShader() { source = "gone.hlsl" } }
`,
	} {
		if err := os.WriteFile(filepath.Join(techsets, name+".techsetdef"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	sampleA = "sample.hlsl_ps_main_AAAAAAAAAAAAAAAAAAAAAAAAAA"
	sampleC = "sample.hlsl_ps_main_CCCCCCCCCCCCCCCCCCCCCCCCCC"
)

func TestShaderDecompile(t *testing.T) {
	root := shaderRoot(t)
	get := func(name, stage string) *shaderOut {
		t.Helper()
		out, err := shaderDecompile(root, name, stage, 0, 0, false)
		if err != nil {
			t.Fatalf("%s %s: %v", name, stage, err)
		}
		return out
	}

	t.Run("a cache file is its HLSL, with the permutations alike", func(t *testing.T) {
		out := get(sampleA, "")
		if out.Shader != sampleA || out.Source != "sample.hlsl" || out.Alike != 1 || !strings.Contains(out.HLSL, "void ps_main(") ||
			out.Length != len(out.HLSL) || out.NextOffset != 0 {
			t.Errorf("%+v", out)
		}
		if out := get(sampleC, ""); out.Alike != 0 {
			t.Errorf("a program of its own: alike %d", out.Alike)
		}
	})
	t.Run("a source with one program for the stage is its HLSL", func(t *testing.T) {
		out := get("sample", "vs")
		if !strings.Contains(out.HLSL, "void vs_main(") || out.Alike != 0 || len(out.Variants) != 0 {
			t.Errorf("%+v", out)
		}
		if out := get("fx/count.hlsl", ""); !strings.Contains(out.HLSL, "void cs_main(") {
			t.Errorf("a source in a directory: %+v", out)
		}
	})
	t.Run("a source with several programs lists them", func(t *testing.T) {
		out := get("sample.hlsl", "ps")
		if out.HLSL != "" || len(out.Variants) != 2 || out.Variants[0].Permutations != 2 || out.Variants[1].Shader != sampleC ||
			!strings.Contains(out.Note, "2 programs") || strings.Contains(out.Note, "stage") {
			t.Errorf("%+v", out)
		}
		if out := get("sample.hlsl", ""); len(out.Variants) != 3 || !strings.Contains(out.Note, "narrow them with stage") {
			t.Errorf("every stage: %+v", out)
		}
	})
	t.Run("a material type answers for its in-game source, with its constants", func(t *testing.T) {
		out := get("mtl_sample", "ps")
		if out.Source != "sample.hlsl" || len(out.Variants) != 2 || strings.Join(out.Params, ",") != "colorTint,hdrScale" {
			t.Errorf("%+v", out)
		}
		if out := get("mtl_sample", "vs"); !strings.Contains(out.HLSL, "void vs_main(") || len(out.Params) != 2 {
			t.Errorf("its vertex shader: %+v", out)
		}
		out = get("mtl_two", "")
		if strings.Join(out.Sources, ",") != "fx_count.hlsl,sample.hlsl" && strings.Join(out.Sources, ",") != "sample.hlsl,fx_count.hlsl" {
			t.Errorf("two sources: %+v", out)
		}
	})
	t.Run("errors", func(t *testing.T) {
		for _, c := range []struct{ name, stage, says string }{
			{"sampl", "", "sources named like it: sample.hlsl"},
			{"nothing_alike", "", `named "nothing_alike"`},
			{"mtl_gone", "", "have no compiled shader in the cache"},
			{"sample.hlsl", "hs", `stage "hs"`},
			{"fx_count.hlsl", "ps", "no ps shader of fx_count.hlsl"},
		} {
			if _, err := shaderDecompile(root, c.name, c.stage, 0, 0, false); err == nil || !strings.Contains(err.Error(), c.says) {
				t.Errorf("%s %s: %v, want %q", c.name, c.stage, err, c.says)
			}
		}
		if _, err := shaderDecompile("", sampleA, "", 0, 0, false); err == nil || !strings.Contains(err.Error(), "no mod-tools path") {
			t.Errorf("no tools path: %v", err)
		}
		if _, err := shaderDecompile(t.TempDir(), sampleA, "", 0, 0, false); err == nil || !strings.Contains(err.Error(), "shader cache") {
			t.Errorf("no cache: %v", err)
		}
	})
}

// Long HLSL comes a page at a time, cut at a line break or, in a line longer
// than a quarter of the page, a space.
func TestShaderDecompilePages(t *testing.T) {
	root := shaderRoot(t)
	whole, err := shaderDecompile(root, sampleC, "", 0, 0, true)
	if err != nil || whole.NextOffset != 0 || len(whole.HLSL) != whole.Length {
		t.Fatalf("all: %v, %+v", err, whole)
	}
	var got strings.Builder
	for offset, pages := 0, 0; ; pages++ {
		out, err := shaderDecompile(root, sampleC, "", offset, 400, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.HLSL) > 400 || pages > 100 {
			t.Fatalf("page of %d characters at %d", len(out.HLSL), offset)
		}
		if out.NextOffset != 0 && !strings.HasSuffix(out.HLSL, "\n") && !strings.HasSuffix(out.HLSL, " ") {
			t.Errorf("a page cut inside a word at %d", out.NextOffset)
		}
		if offset > 0 && out.Alike != 0 {
			t.Error("alike counted again on a later page")
		}
		got.WriteString(out.HLSL)
		if out.NextOffset == 0 {
			break
		}
		offset = out.NextOffset
	}
	if got.String() != whole.HLSL {
		t.Error("the pages don't make the whole HLSL")
	}
	for _, off := range []int{-1, whole.Length} {
		if _, err := shaderDecompile(root, sampleC, "", off, 0, false); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("offset %d: %v", off, err)
		}
	}
}

// A shader the decompiler can't finish still comes back, saying what's missing;
// one it can't read is an error.
func TestShaderDecompileInPartAndUnreadable(t *testing.T) {
	root := shaderRoot(t)
	cache := filepath.Join(root, "share", "assetconvert", "shaders", "pc", "v7")
	b, err := os.ReadFile(filepath.Join(cache, sampleA))
	if err != nil {
		t.Fatal(err)
	}
	stripped := "stripped.hlsl_ps_main_X"
	if err := os.WriteFile(filepath.Join(cache, stripped), bytes.Replace(b, []byte("RDEF"), []byte("XDEF"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := shaderDecompile(root, stripped, "", 0, 0, false)
	if err != nil || !strings.Contains(out.Incomplete, "without reflection") || out.HLSL == "" ||
		strings.HasPrefix(out.Incomplete, "decompiled in part") {
		t.Errorf("%v, %+v", err, out)
	}
	if err := os.WriteFile(filepath.Join(cache, "broken.hlsl_ps_main_Y"), []byte("DXBC"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := shaderDecompile(root, "broken.hlsl_ps_main_Y", "", 0, 0, false); err == nil {
		t.Error("an unreadable shader is no error")
	}
	if _, err := shaderDecompile(root, "broken.hlsl", "", 0, 0, false); err == nil {
		t.Error("an unreadable permutation is no error")
	}
}

// runShader runs `t7kb shader <args>` against root: stdout, stderr, error.
func runShader(root string, args ...string) (string, string, error) {
	cmd := newShaderCmd()
	var out, errs bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errs)
	cmd.SetArgs(append(args, "--tools-path", root))
	err := cmd.Execute()
	return out.String(), errs.String(), err
}

func TestShaderCommand(t *testing.T) {
	root := shaderRoot(t)
	out, _, err := runShader(root, "decompile", sampleA)
	if err != nil || !strings.HasPrefix(out, "// Decompiled by") {
		t.Errorf("HLSL: %v\n%s", err, out)
	}
	out, _, err = runShader(root, "decompile", sampleA, "--json")
	var m map[string]any
	if err != nil || json.Unmarshal([]byte(out), &m) != nil || m["shader"] != sampleA {
		t.Errorf("--json: %v\n%s", err, out)
	}
	out, _, err = runShader(root, "decompile", "sample.hlsl", "--stage", "ps")
	if err != nil || json.Unmarshal([]byte(out), &m) != nil || len(m["variants"].([]any)) != 2 {
		t.Errorf("variants: %v\n%s", err, out)
	}
	if _, _, err := runShader(root, "decompile", "nothing_alike"); err == nil {
		t.Error("an unknown name is no error")
	}
	cache := filepath.Join(root, "share", "assetconvert", "shaders", "pc", "v7")
	b, err := os.ReadFile(filepath.Join(cache, sampleA))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "stripped.hlsl_ps_main_X"), bytes.Replace(b, []byte("RDEF"), []byte("XDEF"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errs, err := runShader(root, "decompile", "stripped.hlsl_ps_main_X")
	if err != nil || !strings.Contains(out, "ps_main") || !strings.Contains(errs, "decompiled in part") {
		t.Errorf("in part: %v\nstdout %q\nstderr %q", err, out, errs)
	}
}

// The MCP tool answers as the operation does, and a failure is a tool error.
func TestShaderToolOverMCP(t *testing.T) {
	root := shaderRoot(t)
	t.Setenv("TA_TOOLS_PATH", root)
	c, err := client.NewInProcessClient(newMCPServer(nil, nil, false))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	text, isErr := call(t, c, "shader_decompile", map[string]any{"name": sampleC, "max_chars": 400})
	m := decode(t, text)
	if isErr || m["shader"] != sampleC || len(m["hlsl"].(string)) > 400 || m["next_offset"] == nil {
		t.Errorf("first page: %s", text)
	}
	text, isErr = call(t, c, "shader_decompile", map[string]any{"name": sampleC, "offset": m["next_offset"], "max_chars": 64000})
	if m := decode(t, text); isErr || m["next_offset"] != nil || !strings.HasSuffix(m["hlsl"].(string), "}\n") {
		t.Errorf("the rest: %s", text)
	}
	text, isErr = call(t, c, "shader_decompile", map[string]any{"name": "sample", "stage": "ps"})
	if m := decode(t, text); isErr || len(m["variants"].([]any)) != 2 {
		t.Errorf("variants: %s", text)
	}
	if text, isErr := call(t, c, "shader_decompile", map[string]any{"name": "nothing_alike"}); !isErr {
		t.Errorf("an unknown name is no tool error: %s", text)
	}
}
