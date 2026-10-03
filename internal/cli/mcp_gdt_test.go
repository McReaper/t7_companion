package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// End-to-end tests of the gdt_* tools through a real MCP client talking to the
// same server `t7kb mcp` builds, in-process, over a tiny fake mod-tools root —
// argument decoding, errors as tool errors, and the output trimming that keeps
// answers small in the agent's context.

func fakeToolsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", t.TempDir()) // the asset index cache
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("deffiles/material.awi", `
	Asset.AddEntry_Combo( "materialType", MaterialTypes );
	Asset.AddEntry_CheckBox( "noCastShadow", false );
	Asset.AddEntry_Float( "gloss", 1, 0, 2 );
	Asset.AddEntry_AssetCombo( "colorMap", "image" );
`)
	write("deffiles/image.awi", `	Asset.AddEntry_Combo( "semantic", "diffuseMap | normalMap" );`)
	var big strings.Builder
	for i := 0; i < 90; i++ { // over the 80-field detail limit
		fmt.Fprintf(&big, "\tAsset.AddEntry_Float( \"field%02d\", 0, 0, 1 );\n", i)
	}
	write("deffiles/weapon.awi", big.String())
	write("share/raw/techsetdefs_stable/lit.techsetdef", "Texture( \"colorMap\" )\n{\n\timage = Image( <colorMap, $white_diffuse> )\n\tsemantic = \"diffuseMap\"\n}\n")
	write("bin/converter_gdt_dirs_0.txt", "source_data\n")
	write("stock.gdtdef", "")

	var g strings.Builder
	g.WriteString("{\r\n\t\"shared_img\" ( \"image.gdf\" )\r\n\t{\r\n\t\t\"semantic\" \"diffuseMap\"\r\n\t}\r\n")
	g.WriteString("\t\"clean\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"noCastShadow\" \"0\"\r\n\t\t\"gloss\" \"1\"\r\n\t\t\"colorMap\" \"shared_img\"\r\n\t}\r\n")
	for i := 0; i < 60; i++ { // 60 users of shared_img (refs cap is 50), 30 of them broken (check cap is 25)
		bad := "0"
		if i < 30 {
			bad = "maybe"
		}
		fmt.Fprintf(&g, "\t\"m%02d\" ( \"material.gdf\" )\r\n\t{\r\n\t\t\"materialType\" \"lit\"\r\n\t\t\"noCastShadow\" \"%s\"\r\n\t\t\"colorMap\" \"shared_img\"\r\n\t}\r\n", i, bad)
	}
	g.WriteString("}\r\n")
	write("source_data/test.gdt", g.String())
	return root
}

// call invokes a tool and returns its text and whether it was a tool error.
func call(t *testing.T, c *client.Client, name string, args map[string]any) (string, bool) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = name, args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: protocol error (tool failures must be tool errors): %v", name, err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	return text, res.IsError
}

func decode(t *testing.T, text string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, text)
	}
	return m
}

func TestGDTToolsOverMCP(t *testing.T) {
	root := fakeToolsRoot(t)
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
	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil || len(tools.Tools) != 12 { // search, get, build, 6 gdt_*, 3 zone_*
		t.Fatalf("tools/list: %d tools, %v", len(tools.Tools), err)
	}

	t.Run("find", func(t *testing.T) {
		out, isErr := call(t, c, "gdt_find", map[string]any{"name": "clean"})
		defs, _ := decode(t, out)["definitions"].([]any)
		if isErr || len(defs) != 1 || !strings.Contains(out, `"file":"source_data/test.gdt"`) {
			t.Fatalf("%s", out)
		}
	})

	t.Run("get hides defaults unless all", func(t *testing.T) {
		out, _ := call(t, c, "gdt_get", map[string]any{"name": "clean"})
		own := decode(t, out)["own_fields"].(map[string]any)
		if _, shown := own["noCastShadow"]; shown || own["colorMap"] != "shared_img" || !strings.Contains(out, "hidden") {
			t.Fatalf("zero/default fields must be hidden, real ones kept: %s", out)
		}
		out, _ = call(t, c, "gdt_get", map[string]any{"name": "clean", "all": true})
		if _, shown := decode(t, out)["own_fields"].(map[string]any)["noCastShadow"]; !shown {
			t.Fatalf("all=true must show every field: %s", out)
		}
	})

	t.Run("schema lists names only for big types", func(t *testing.T) {
		out, _ := call(t, c, "gdt_schema", map[string]any{"type": "weapon"})
		m := decode(t, out)
		if _, detailed := m["entries"]; detailed || m["hint"] == nil {
			t.Fatalf("a 90-field type must come back as names plus a hint: %.300s", out)
		}
		out, _ = call(t, c, "gdt_schema", map[string]any{"type": "weapon", "filter": "field01"})
		if _, detailed := decode(t, out)["entries"]; !detailed {
			t.Fatalf("a filtered schema must be detailed: %s", out)
		}
	})

	t.Run("edit: dry run, write, strict arguments", func(t *testing.T) {
		args := map[string]any{"file": "source_data/new.gdt", "asset": "fresh", "type": "material",
			"set": map[string]any{"materialType": "lit", "noCastShadow": true}}
		out, isErr := call(t, c, "gdt_edit", args)
		if isErr || decode(t, out)["written"] != false {
			t.Fatalf("gdt_edit must be a dry run by default: %s", out)
		}
		if _, err := os.Stat(filepath.Join(root, "source_data", "new.gdt")); err == nil {
			t.Fatal("a dry run wrote the file")
		}
		args["write"] = true
		out, _ = call(t, c, "gdt_edit", args)
		if decode(t, out)["written"] != true {
			t.Fatalf("write=true: %s", out)
		}
		b, _ := os.ReadFile(filepath.Join(root, "source_data", "new.gdt"))
		if !strings.Contains(string(b), "\"noCastShadow\" \"1\"") {
			t.Fatalf("a JSON true must be stored as 1:\n%s", b)
		}
		out, isErr = call(t, c, "gdt_edit", map[string]any{"file": "source_data/new.gdt", "asset": "x", "copyFrom": "clean"})
		if !isErr || !strings.Contains(out, "copyFrom") {
			t.Fatalf("a misspelt argument must be a tool error naming it: %s", out)
		}
	})

	t.Run("check caps detail and groups the rest", func(t *testing.T) {
		out, _ := call(t, c, "gdt_check", map[string]any{"file": "source_data/test.gdt"})
		m := decode(t, out)
		assets, _ := m["assets_with_issues"].([]any)
		kinds, _ := m["issue_kinds"].([]any)
		if len(assets) != 25 || m["truncated"] == nil || len(kinds) == 0 || !strings.Contains(fmt.Sprint(kinds[0]), "30×") {
			t.Fatalf("30 broken assets: detail capped at 25, grouped kinds with counts: %.600s", out)
		}
	})

	t.Run("refs caps at 50", func(t *testing.T) {
		out, _ := call(t, c, "gdt_refs", map[string]any{"name": "shared_img"})
		m := decode(t, out)
		hits, _ := m["referenced_by"].([]any)
		if m["count"] != float64(61) || len(hits) != 50 || m["truncated"] == nil {
			t.Fatalf("61 references: count 61, 50 listed, truncated: count=%v listed=%d", m["count"], len(hits))
		}
	})

	t.Run("failures are tool errors", func(t *testing.T) {
		if out, isErr := call(t, c, "gdt_get", map[string]any{"name": "nope"}); !isErr || !strings.Contains(out, "not found") {
			t.Fatalf("an unknown asset must be a readable tool error: %s", out)
		}
	})
}
