package cli

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// zoneToolset reads the report the linker writes on every link — what a map's
// fastfile holds, how big each asset is, and which zone line pulled it in.
func zoneToolset() toolset {
	tp := mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))
	name := mcp.WithString("name", mcp.Required(), mcp.Description("Map or mod name, e.g. \"zm_mymap\"."))
	return toolset{name: "zone", tools: []server.ServerTool{
		zoneExplainTool(name, tp), zoneContentsTool(name, tp), zoneCheckTool(name, tp)}}
}

func zoneExplainTool(name, tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("zone_explain",
		mcp.WithDescription("Why an asset is in a map's build, from the report the linker wrote at the last link: the chain "+
			"of parents that pulled it in (image <- material <- fx <- weapon <- .zpkg <- .zone), and its size. Says when the "+
			"last link didn't pack it, and when sources changed since the link (then relink before trusting it)."),
		name,
		mcp.WithString("asset", mcp.Required(), mcp.Description("Asset name, e.g. \"t6_skybox\" or \"zombie/fx_blood\".")),
		mcp.WithString("type", mcp.Description("Asset type, when one name is used by several (image, material, xmodel, fx…).")), tp),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			return zoneExplain(r.GetString("tools_path", ""), r.GetString("name", ""), r.GetString("asset", ""), r.GetString("type", ""))
		})}
}

func zoneContentsTool(name, tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("zone_contents",
		mcp.WithDescription("What a zone line pulls into a map's build, from the last link's report: asset count and bytes "+
			"(resident and streamed), counts per type, and the largest assets. Without a line: every zone line by weight, "+
			"to see what makes a fastfile big, and what publishing uploads (the zone/ folder), with the .xpak data earlier links "+
			"left in it. images: where the upload can shrink without cutting content: the images by who defines them (your "+
			"GDTs, stock, none), your GDTs and largest images by what one mipBase step down would save."),
		name,
		mcp.WithString("line", mcp.Description("A zone line (\"weapon,t8_knife_zm\"), an asset name, or an included .zpkg name.")),
		mcp.WithBoolean("images", mcp.Description("Weigh the images instead (of the whole build, or of the line) and what mipBase would save.")), tp),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			if r.GetBool("images", false) {
				return zoneImages(r.GetString("tools_path", ""), r.GetString("name", ""), r.GetString("line", ""))
			}
			return zoneContents(r.GetString("tools_path", ""), r.GetString("name", ""), r.GetString("line", ""))
		})}
}

func zoneCheckTool(name, tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("zone_check",
		mcp.WithDescription("Find your versions of stock assets that the build won't use: a stock script or asset you "+
			"copied and zoned, or defined in your own GDT, that a stock assetlist inherited through the map's >class still "+
			"lists, so the linker packs only a reference to the shipped one. Gives the assetlist line to comment out. "+
			"Works before the first link; after a link it also checks every GDT asset the build drew on."),
		name, tp),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			return zoneCheck(r.GetString("tools_path", ""), r.GetString("name", ""))
		})}
}
