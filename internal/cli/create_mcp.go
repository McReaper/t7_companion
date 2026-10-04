package cli

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// createToolset creates a new map or mod from the install's templates, as the
// Launcher's File > New does. Side-effecting (with write), dry run by default.
func createToolset() toolset {
	return toolset{name: "create", tools: []server.ServerTool{{Tool: mcp.NewTool("create",
		mcp.WithDescription("Create a new usermap or mod from the mod tools' own templates (rex/templates), as the Mod Tools "+
			"Launcher's File > New does: the map's .map in map_source/, its scripts, zone and sound config under usermaps/<name>, "+
			"or a mod's zone files under mods/<name>. A dry run unless write is true: it lists the files and any that already "+
			"exist, and a write never overwrites one. A map's name must start with zm_ or mp_, matching the template."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Map or mod name: letters, digits and _, e.g. \"zm_leviathan\" (lower-cased).")),
		mcp.WithString("template", mcp.Description("A folder of rex/templates, e.g. \"ZM Mod Level\" (default for zm_), \"ZM Basic Level\", "+
			"\"ZM Advanced Level\", \"MP Mod Level\" (default for mp_), \"Mod\" (default otherwise). The answer lists this install's.")),
		mcp.WithString("zones", mcp.Description("Mods only: which zones to keep, comma-separated among core, mp, cp, zm (default: all).")),
		mcp.WithBoolean("write", mcp.Description("Create the files (default false: dry run).")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			return createOp(r.GetString("tools_path", ""), r.GetString("name", ""), r.GetString("template", ""),
				splitZones(r.GetString("zones", "")), r.GetBool("write", false))
		})}}}
}

// splitZones reads "core,zm" into its items.
func splitZones(s string) []string {
	var out []string
	for _, z := range strings.Split(s, ",") {
		if z = strings.ToLower(strings.TrimSpace(z)); z != "" {
			out = append(out, z)
		}
	}
	return out
}
