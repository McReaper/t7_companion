package cli

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// shaderToolset decompiles the game's compiled shaders (t7_dxbc), the start of
// a custom shader: internal/shader has the cache's rules.
func shaderToolset() toolset {
	return toolset{name: "shader", tools: []server.ServerTool{shaderDecompileTool()}}
}

func shaderDecompileTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("shader_decompile",
		mcp.WithDescription("Decompile one of Black Ops III's compiled shaders (the mod tools' cache) to HLSL that compiles "+
			"back to the same shader, to start a custom one from a stock one: entry point ps_main/vs_main, material "+
			"parameters as global variables, the reflected names a techset binds textures and constants by. name: a "+
			"cache file (techsetdef_unlit.hlsl_ps_main_CJK6…), a shader source (techsetdef_unlit.hlsl) or a material "+
			"type (emissive_add; its techset gives the source). A source's permutations that compiled to one program "+
			"count as one: with a single program for the stage the answer is its HLSL, else the programs with what "+
			"each reads ($Globals, textures and buffers, inputs) — call again with one's shader as name. Long HLSL "+
			"comes a page at a time (next_offset). incomplete names what couldn't be decompiled."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Cache file, shader source or material type.")),
		mcp.WithString("stage", mcp.Description("ps, vs, gs or cs: the stage to decompile (default every stage the source has).")),
		mcp.WithNumber("offset", mcp.Description("Where to start in the HLSL, from a previous answer's next_offset (default 0).")),
		mcp.WithNumber("max_chars", mcp.Description("Page size in characters (default 16000, at most 64000).")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			return shaderDecompile(r.GetString("tools_path", ""), r.GetString("name", ""), r.GetString("stage", ""),
				r.GetInt("offset", 0), r.GetInt("max_chars", 0), false)
		})}
}
