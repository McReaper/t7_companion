package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/McReaper/t7_companion/internal/build"
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
			"per-stage JSON report: a failing stage's errors and, for the link, every error and warning the "+
			"linker logged (it exits non-zero on warnings too) and `missing`: what it looked up and found in "+
			"no GDT (it links on without it). Look each message up with search. Requires a "+
			"Windows BO3 mod-tools install (path from $TA_TOOLS_PATH or the tools_path arg). "+
			"NOTE: runs synchronously and can take minutes (link) up to 20-30 min (full compile+light) "+
			"— set a long client timeout. For a script-only change, pass stages=\"link\"."),
		mcp.WithString("name", mcp.Required(),
			mcp.Description("Map or mod name, e.g. \"zm_mymap\".")),
		mcp.WithString("stages",
			mcp.Description("Comma list of stages to run: compile,light,link,run (default \"compile,light,link\"). Use \"link\" alone for a script-only change. run starts the game and watches its first 90 s: a dialog it opens there (a fatal error, or a question like safe mode, which it waits on) fails the stage with its text and the game console's last lines.")),
		mcp.WithBoolean("mod",
			mcp.Description("Target is a mod (mods/<name>) instead of a usermap; skips compile+light (default false).")),
		mcp.WithString("light",
			mcp.Description("Light bake quality: low|medium|high (default \"medium\").")),
		mcp.WithBoolean("onlyents",
			mcp.Description("Fast entity-only compile (-onlyents); invalid after brush edits (default false).")),
		mcp.WithString("language",
			mcp.Description("Linker language (default: the Build Language saved in the mod tools Launcher's options, else \"english\"). The game loads a map's <language>_<map>.ff in its own language and stops on \"Could not find zone\" without it.")),
		mcp.WithBoolean("skip_gdt",
			mcp.Description("Skip the gdtdb /update pass before building (default false).")),
		mcp.WithBoolean("gdt_rebuild",
			mcp.Description("Run gdtdb /rebuild (~1.5 min) instead of /update. /update does pick up GDTs edited outside APE (it reports processed (N GDTs)); use this only if it reports 0 GDTs and the linker then can't find an edited asset (default false).")),
		mcp.WithBoolean("fresh_xpak",
			mcp.Description("link stage: write the .xpak from scratch. The linker only appends to it, so the data every earlier link replaced stays in it and uploads with the zone/ folder the Launcher publishes; zone_contents says how much. Use it for the link you publish (default false).")),
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
		o := &build.Options{
			ToolsPath:  req.GetString("tools_path", ""),
			GamePath:   req.GetString("game_path", ""),
			Mod:        req.GetBool("mod", false),
			Stages:     req.GetString("stages", "compile,light,link"),
			OnlyEnts:   req.GetBool("onlyents", false),
			Light:      req.GetString("light", "medium"),
			Language:   req.GetString("language", ""),
			SkipGDT:    req.GetBool("skip_gdt", false),
			GDTRebuild: req.GetBool("gdt_rebuild", false),
			FreshXpak:  req.GetBool("fresh_xpak", false),

			LauncherDvars: req.GetBool("launcher_dvars", false),
		}
		if o.Dvars, err = dvarPairs(req.GetArguments()["dvars"]); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		rep, err := buildRun(o, name, io.Discard)
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

// dvarPairs turns the MCP dvars object into sorted "name=value" pairs; numbers
// and booleans are accepted as values (true -> 1, false -> 0).
func dvarPairs(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dvars must be an object of name: value")
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	var out []string
	for _, k := range names {
		var s string
		switch x := m[k].(type) {
		case string:
			s = x
		case bool:
			s = map[bool]string{true: "1", false: "0"}[x]
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		default:
			return nil, fmt.Errorf("dvar %q: value must be a string, number or boolean", k)
		}
		out = append(out, k+"="+s)
	}
	return out, nil
}
