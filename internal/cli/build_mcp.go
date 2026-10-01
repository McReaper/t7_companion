package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// buildToolset drives the mod-tools build pipeline (the one tool that runs the
// Windows binaries).
func buildToolset() toolset {
	return toolset{name: "build", tools: []server.ServerTool{{Tool: buildToolDef(), Handler: buildToolHandler()}}}
}

func buildToolDef() mcp.Tool {
	return mcp.NewTool("build",
		mcp.WithDescription("Headlessly compile/light/link a Black Ops 3 map or mod by driving the "+
			"mod-tools pipeline (gdtdb, cod2map64, radiant light, linker), returning a compact "+
			"per-stage JSON report with the first actionable error of any failing stage. Requires a "+
			"Windows BO3 mod-tools install (path from $TA_TOOLS_PATH or the tools_path arg). "+
			"NOTE: runs synchronously and can take minutes (link) up to 20-30 min (full compile+light) "+
			"— set a long client timeout. For a script-only change, pass stages=\"link\"."),
		mcp.WithString("name", mcp.Required(),
			mcp.Description("Map or mod name, e.g. \"zm_mymap\".")),
		mcp.WithString("stages",
			mcp.Description("Comma list of stages to run: compile,light,link,run (default \"compile,light,link\"). Use \"link\" alone for a script-only change.")),
		mcp.WithBoolean("mod",
			mcp.Description("Target is a mod (mods/<name>) instead of a usermap; skips compile+light (default false).")),
		mcp.WithString("light",
			mcp.Description("Light bake quality: low|medium|high (default \"medium\").")),
		mcp.WithBoolean("onlyents",
			mcp.Description("Fast entity-only compile (-onlyents); invalid after brush edits (default false).")),
		mcp.WithString("language",
			mcp.Description("Linker language (default \"english\").")),
		mcp.WithBoolean("skip_gdt",
			mcp.Description("Skip the gdtdb /update pass before building (default false).")),
		mcp.WithBoolean("gdt_rebuild",
			mcp.Description("Run gdtdb /rebuild (~1.5 min) instead of /update. /update does pick up GDTs edited outside APE (it reports processed (N GDTs)); use this only if it reports 0 GDTs and the linker then can't find an edited asset (default false).")),
		mcp.WithObject("dvars",
			mcp.Description("run stage: dvars to start the game with, as {\"developer\": \"2\", \"logfile\": \"2\"} — logfile 2 writes console_mp.log, the way to read a script error. Overrides launcher_dvars.")),
		mcp.WithBoolean("launcher_dvars",
			mcp.Description("run stage: start the game with the dvars saved in the mod tools Launcher's Dvars dialog, as the Launcher does (default false).")),
		mcp.WithString("tools_path",
			mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH).")),
		mcp.WithString("game_path",
			mcp.Description("BO3 game root (default $TA_GAME_PATH, then tools_path).")),
	)
}

func buildToolHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		o := &buildOpts{
			toolsPath:  req.GetString("tools_path", ""),
			gamePath:   req.GetString("game_path", ""),
			isMod:      req.GetBool("mod", false),
			stages:     req.GetString("stages", "compile,light,link"),
			onlyEnts:   req.GetBool("onlyents", false),
			light:      req.GetString("light", "medium"),
			language:   req.GetString("language", "english"),
			skipGDT:    req.GetBool("skip_gdt", false),
			gdtRebuild: req.GetBool("gdt_rebuild", false),

			launcherDvars: req.GetBool("launcher_dvars", false),
		}
		if o.dvars, err = dvarPairs(req.GetArguments()["dvars"]); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		rep, err := runBuildReport(o, name, io.Discard)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("marshal build report", err), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}
