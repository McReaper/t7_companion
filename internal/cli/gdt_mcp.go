package cli

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/McReaper/t7_companion/internal/gdt"
)

// gdtToolset is GDT authoring: find/get/schema/edit/check/refs over the mod-tools
// root's GDTs. Its index is warmed at server start, like gdtdb does for APE.
func gdtToolset() toolset {
	return toolset{name: "gdt", tools: gdtToolDefs(), warm: func() {
		if w, err := workspace(""); err == nil {
			w.Warm()
			w.WarmModels()
		}
	}}
}

func gdtToolDefs() []server.ServerTool {
	tp := mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))
	return []server.ServerTool{gdtFindTool(tp), gdtGetTool(tp), gdtSchemaTool(tp), gdtEditTool(tp), gdtCheckTool(tp), gdtRefsTool(tp)}
}

func gdtFindTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_find",
		mcp.WithDescription("Find every GDT that defines an asset (file, line, type, parent, whether the GDT is stock). "+
			"More than one definition is a `Duplicate asset` link error."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Asset name, e.g. \"mtl_my_wall\".")), tp),
		Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
			return gdtFind(w, r.GetString("name", ""))
		})}
}

func gdtGetTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_get",
		mcp.WithDescription("Read an asset from its GDT: own fields, fields inherited through a derived asset's parent chain, "+
			"and validation issues against the deffile schema (and, for a material, its techset)."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Asset name.")),
		mcp.WithString("file", mcp.Description("Which GDT, when the asset is defined in several (relative to the root).")),
		mcp.WithString("filter", mcp.Description("Only return fields whose key contains this text (e.g. \"lod\", \"map\").")),
		mcp.WithBoolean("all", mcp.Description("Also show empty and zero fields (hidden by default to keep the answer small).")), tp),
		Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
			return gdtGet(w, r.GetString("name", ""), r.GetString("file", ""), r.GetString("filter", ""), r.GetBool("all", false))
		})}
}

func gdtSchemaTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_schema",
		mcp.WithDescription("The fields APE declares for an asset type, read from deffiles/<type>.awi: kind, default, min/max, "+
			"combo options, target asset type. For type \"material\" with material_type, also the techsetdef's texture slots "+
			"(GDT field + required image semantic), parameters, category and HLSL sources. No type lists all asset types."),
		mcp.WithString("type", mcp.Description("Asset type (the GDT's \"<type>.gdf\"), e.g. material, xmodel, image, xanim.")),
		mcp.WithString("material_type", mcp.Description("For materials: the materialType (techset) to resolve, e.g. lit, lit_emissive.")),
		mcp.WithString("filter", mcp.Description("Only entries whose name or title contains this text.")), tp),
		Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
			return gdtSchema(w, r.GetString("type", ""), r.GetString("material_type", ""), r.GetString("filter", ""))
		})}
}

func gdtEditTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_edit",
		mcp.WithDescription("Create or update assets in a GDT, validated against the deffile schema and (for materials) the "+
			"techset: numeric ranges, combo values, checkboxes, color/vector shapes, unknown keys, materialType/materialCategory "+
			"agreement, texture fields the techset doesn't read, image semantic mismatches, missing source files, a second asset of the same type and name. "+
			"One asset via asset/type/parent/copy_from/image/set/unset, or several at once via assets (applied in order, so a "+
			"later item can derive from or reference an earlier one; written all-or-nothing). image creates an image asset from "+
			"a texture, with the semantic taken from the techset slot it fills and the other settings from a stock image of that "+
			"semantic. DRY RUN BY DEFAULT — returns the changes and issues; pass write=true to save (atomic, with a .bak, other "+
			"assets left byte-identical; refused if the file changed on disk meanwhile). Only GDTs under gdtdb's directories. Refuses stock Treyarch GDTs: copy_from the stock asset into your own GDT instead — parent only works within one GDT."),
		mcp.WithString("file", mcp.Required(), mcp.Description("GDT path relative to the root, e.g. \"source_data/my_map.gdt\" (created if missing).")),
		mcp.WithString("asset", mcp.Description("Asset name (single-asset form).")),
		mcp.WithString("type", mcp.Description("Create a full asset of this type (material, xmodel, image, …).")),
		mcp.WithString("parent", mcp.Description("Create a derived asset of this parent (only overridden fields are stored). The parent must be in the same GDT — gdtdb rejects one from another file.")),
		mcp.WithString("copy_from", mcp.Description("Create by copying this asset's type and resolved fields (an xmodel's LOD paths are cleared).")),
		mcp.WithObject("set", mcp.Description("Fields to set, as {\"key\": \"value\"} with real values — paths use single backslashes; escaping is handled.")),
		mcp.WithArray("unset", mcp.Description("Field keys to remove."), mcp.WithStringItems()),
		mcp.WithObject("image", mcp.Description("Create an image asset from a texture: {\"texture\": \"texture_assets/my/wall_n.tif\" (relative to the root), "+
			"\"material_type\": \"lit\", \"field\": \"normalMap\"} takes the semantic from that techset slot, or give \"semantic\" directly. "+
			"The texture must exist, with power-of-two dimensions.")),
		mcp.WithArray("assets", mcp.Description("Batch form: a list of {asset, type?, parent?, copy_from?, image?, set?, unset?} objects, "+
			"same meaning as the single-asset parameters (which are then ignored)."), mcp.Items(map[string]any{"type": "object"})),
		mcp.WithBoolean("write", mcp.Description("Actually save (default false = dry run).")), tp),
		Handler: gdtHandler(gdtEditCall)}
}

// gdtEditCall is gdt_edit: one asset, or a batch via assets.
func gdtEditCall(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
	var args gdtEditArgs
	if err := decodeArg(r.GetArguments(), &args); err != nil {
		return nil, err
	}
	if len(args.Assets) > 0 {
		items := make([]gdtEditItem, len(args.Assets))
		for i, raw := range args.Assets {
			if err := decodeStrict(raw, &items[i]); err != nil {
				return nil, fmt.Errorf("assets[%d]: %w", i, err)
			}
		}
		return gdtEditBatch(w, args.File, items, !args.Write)
	}
	if args.Asset == "" {
		return nil, fmt.Errorf("pass asset (one asset) or assets (a batch)")
	}
	req, err := args.request(args.File)
	if err != nil {
		return nil, err
	}
	req.DryRun = !args.Write
	return gdtEdit(w, req)
}

func gdtCheckTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_check",
		mcp.WithDescription("Diagnose a whole GDT (or one asset in it) the way the linker would, before you build: schema values, "+
			"material/techset rules, typed references (the referenced asset exists and is the right type, e.g. a material's "+
			"colorMap names an image), source files on disk (xmodel/xanim exports, image textures), missing parents, and names "+
			"defined in more than one GDT. Returns only the assets that have issues."),
		mcp.WithString("file", mcp.Required(), mcp.Description("GDT path relative to the root.")),
		mcp.WithString("asset", mcp.Description("Only check this asset.")), tp),
		Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
			return gdtCheck(w, r.GetString("file", ""), r.GetString("asset", ""))
		})}
}

func gdtRefsTool(tp mcp.ToolOption) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("gdt_refs",
		mcp.WithDescription("Find every asset, in every GDT, that references an asset by name — through a field (a material's "+
			"image, an entry of an xmodel's skinOverride, …), as a derived asset's parent, or — for a material — inside an "+
			"xmodel's LOD or collision file. Check this before renaming or deleting an asset."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Asset name.")), tp),
		Handler: gdtHandler(func(w *gdt.Workspace, r mcp.CallToolRequest) (any, error) {
			return gdtRefs(w, r.GetString("name", ""))
		})}
}

// gdtHandler opens the workspace the call names (or $TA_TOOLS_PATH) and runs fn.
func gdtHandler(fn func(*gdt.Workspace, mcp.CallToolRequest) (any, error)) server.ToolHandlerFunc {
	return jsonHandler(func(_ context.Context, req mcp.CallToolRequest) (any, error) {
		w, err := workspace(req.GetString("tools_path", ""))
		if err != nil {
			return nil, err
		}
		return fn(w, req)
	})
}

// ---- CLI
