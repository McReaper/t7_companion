package cli

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mapToolset edits the map open in the user's Radiant, through the
// T7RadiantBridge plugin: internal/radiant has the connection's rules.
func mapToolset() toolset {
	return toolset{name: "map", tools: []server.ServerTool{radiantOpenTool(), mapFindTool()}}
}

func mapFindTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_find",
		mcp.WithDescription("Find entities in the map open in Radiant (the one running, or pid), as it is now, unsaved edits "+
			"included. Every filter given must hold: ids, class, key, near, selected. Each entity comes with its short id "+
			"(the map tools' name for it: a GUID's first 8 hex digits), class, origin, and for brush entities their bounds "+
			"(min x y z, max x y z) and brush count; keys are the naming ones (targetname, target, script_noteworthy, "+
			"script_string, script_int, name, model) unless keys asks for others. brushes lists each one's brushes and "+
			"patches (id, material, bounds, contents, curve/mesh); naming a brush's id lists just it. near sorts by distance. "+
			"No face is shown: Radiant holds the geometry."),
		mcp.WithString("ids", mcp.Description("Entity or brush ids (or GUIDs, or any start of one), comma- or space-separated.")),
		mcp.WithString("class", mcp.Description("Classname; * matches any run of characters (light*, script_*).")),
		mcp.WithString("key", mcp.Description("An entity key the entity has, or key=value (* in the value), e.g. targetname=door_1.")),
		mcp.WithString("near", mcp.Description("A point, x y z: entities whose bounds come within radius of it, nearest first.")),
		mcp.WithNumber("radius", mcp.Description("With near: the distance in units (default 256).")),
		mcp.WithBoolean("selected", mcp.Description("Only entities with a brush selected in Radiant (the user's selection).")),
		mcp.WithBoolean("brushes", mcp.Description("List each entity's brushes and patches (default false).")),
		mcp.WithString("keys", mcp.Description("Which keys to show: all, or a comma list (default the naming keys).")),
		mcp.WithNumber("limit", mcp.Description("How many entities at most (default 50); brush lists stop at 200 rows.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to read when several run."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			return mapFind(ctx, mapFindArgs{IDs: splitIDs(r.GetString("ids", "")), Class: r.GetString("class", ""),
				Key: r.GetString("key", ""), Near: r.GetString("near", ""), Radius: r.GetFloat("radius", 0),
				Selected: r.GetBool("selected", false), Brushes: r.GetBool("brushes", false), Keys: r.GetString("keys", ""),
				Limit: r.GetInt("limit", 0), PID: r.GetInt("pid", 0)})
		})}
}

// splitIDs reads ids separated by commas or spaces.
func splitIDs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
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
