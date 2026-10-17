package cli

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mapToolset edits the map open in the user's Radiant, through the
// T7RadiantBridge plugin: internal/radiant has the connection's rules.
func mapToolset() toolset {
	return toolset{name: "map", tools: []server.ServerTool{radiantOpenTool()}}
}

func radiantOpenTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("radiant_open",
		mcp.WithDescription("Open a map in Radiant, where every map edit happens: never edit a .map file while Radiant may "+
			"hold it, Radiant's next save overwrites it. Uses the Radiant that has the map open already; else the one running "+
			"(pid picks one of several), which loads it in place of its map, refused over unsaved changes unless discard, "+
			"under a dialog or inside a prefab; else starts Radiant on the map and waits for it (up to 3 minutes). Radiant "+
			"needs the T7RadiantBridge plugin (github.com/McReaper/t7_radiant_bridge, loaded by MTPlugins). Answers the "+
			"Radiant's pid, the map's path and how it came to be open (already open, reloaded, launched)."),
		mcp.WithString("map", mcp.Required(), mcp.Description("A map's name (zm_castle: map_source/zm/zm_castle.map) or a .map path, relative to map_source/ unless absolute.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to use when several run (the error lists them).")),
		mcp.WithBoolean("discard", mcp.Description("Drop the Radiant's unsaved changes to load the map (default false: refused).")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			return radiantOpen(ctx, r.GetString("tools_path", ""), r.GetString("map", ""), r.GetInt("pid", 0), r.GetBool("discard", false))
		})}
}
