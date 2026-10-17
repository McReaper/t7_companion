package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mapToolset edits the map open in the user's Radiant, through the
// T7RadiantBridge plugin: internal/radiant has the connection's rules.
func mapToolset() toolset {
	return toolset{name: "map", tools: []server.ServerTool{radiantOpenTool(), mapFindTool(), mapViewTool(), mapEditTool(), mapCommandTool()}}
}

func mapCommandTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_command",
		mcp.WithDescription("Run one of Radiant's own menu entries on entities and brushes of the open map: they are "+
			"selected alone for it, then the user's selection comes back. For what map_edit has no op for: CSG "+
			"(Selection/CSG/CSG Hollow, CSG Subtract, CSG Merge), patches, textures, prefabs. The answer reads the map "+
			"back: the undo entry the command made (one Ctrl+Z), what it created (id, class or material, bounds), what it "+
			"deleted, what it left selected. A command that opens a dialog waits on the user. menu instead of command "+
			"lists a menu's entries (\"Selection/CSG\"; \"\" the menu bar), \" >\" marking a submenu."),
		mcp.WithString("command", mcp.Description("The menu path, labels from the menu bar: Selection/CSG/CSG Hollow.")),
		mcp.WithString("ids", mcp.Description("What to select for it, comma- or space-separated (default nothing selected).")),
		mcp.WithString("menu", mcp.Description("List this menu's entries instead of running a command.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to use when several run."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			return mapCommand(ctx, r.GetString("command", ""), splitIDs(r.GetString("ids", "")), r.GetString("menu", ""), r.GetInt("pid", 0))
		})}
}

func mapEditTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_edit",
		mcp.WithDescription("Edit the map open in Radiant: a batch of ops, applied in order, lands as ONE undo step "+
			"(the answer's undo names it; one Ctrl+Z in Radiant takes it all back) or not at all. Ops, each an object with "+
			"one key: {\"box\": {\"min\": [x,y,z], \"max\": [x,y,z], \"material\": m, \"contents\"?: \"detail\", "+
			"\"class\"?: c, \"keys\"?: {…}}} a box brush, in worldspawn or as a new brush entity of class (volume_sun, "+
			"trigger_multiple); {\"entity\": {\"class\": c, \"origin\": [x,y,z], \"angles\"?: [p,y,r], \"keys\"?: {…}}} a "+
			"point entity; {\"delete\": [ids]}; {\"set\": {\"id\": id, \"keys\": {k: v, k2: null}}} (null removes); "+
			"{\"move\": {\"ids\": [ids], \"by\": [dx,dy,dz]}}; {\"material\": {\"ids\": [ids], \"material\": m}}. "+
			"Ids are map_find's. A point entity keeps its id; a world brush or an entity that owns brushes is rebuilt and "+
			"gets a new one (was gives the old). The answer reads the map back: each op's id, class, origin or bounds. "+
			"Edits go through Radiant, never the .map file, and the user's selection is kept."),
		mcp.WithArray("ops", mcp.Required(), mcp.Description("The ops, in order."), mcp.Items(map[string]any{"type": "object"})),
		mcp.WithString("label", mcp.Description("What the batch does, for Radiant's undo menu (default: the ops counted).")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to edit when several run."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			raw, err := json.Marshal(r.GetArguments()["ops"])
			if err != nil {
				return nil, err
			}
			ops, err := decodeOps(raw)
			if err != nil {
				return nil, err
			}
			return mapEdit(ctx, ops, r.GetString("label", ""), r.GetInt("pid", 0))
		})}
}

func mapViewTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_view",
		mcp.WithDescription("See the map open in Radiant: an image of its 3D view or one of its 2D views (top, front, side), "+
			"as Radiant draws it, with the short ids of what is in it written where their origin or centre is (labels "+
			"gives the same as id: [x, y] in the image). Frame entities or brushes by ids, or a point by at (and radius); "+
			"shoot the 3D view from camera; or take the view as it is. A framed 3D shot looks down from the south-west. "+
			"Top shows the layout best (outlines, entity names); an unbaked map is black under real lighting, so pass "+
			"render fake to see a blockout's shapes. A 2D answer gives its centre and scale (image pixels per unit). "+
			"The user's camera, render method, 2D axis, zoom and centre are put back (notes says what could not be); "+
			"Radiant must not be minimised. size is the image's longer side (default 768: about 400 tokens)."),
		mcp.WithString("view", mcp.Description("3d (default), top, front or side.")),
		mcp.WithString("ids", mcp.Description("Entities or brushes to frame, by id, comma- or space-separated.")),
		mcp.WithString("at", mcp.Description("A point x y z to frame, with radius around it.")),
		mcp.WithNumber("radius", mcp.Description("With at: how far around the point to show (default 256).")),
		mcp.WithString("camera", mcp.Description("3d: shoot from x y z pitch yaw roll (degrees; positive pitch looks up, yaw 90 faces +y).")),
		mcp.WithString("render", mcp.Description("3d: the render method for this shot: fake, fullbright, wireframe, real, view or density.")),
		mcp.WithNumber("size", mcp.Description("The image's longer side in pixels (default 768, at most 1568).")),
		mcp.WithBoolean("no_labels", mcp.Description("Leave the ids off the image (default false).")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to see when several run."))),
		Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, png, err := mapView(ctx, mapViewArgs{View: r.GetString("view", ""), IDs: splitIDs(r.GetString("ids", "")),
				At: r.GetString("at", ""), Radius: r.GetFloat("radius", 0), Camera: r.GetString("camera", ""),
				Render: r.GetString("render", ""), Size: r.GetInt("size", 0), NoLabels: r.GetBool("no_labels", false), PID: r.GetInt("pid", 0)})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			res := jsonResult(out)
			res.Content = append(res.Content, mcp.NewImageContent(base64.StdEncoding.EncodeToString(png), "image/png"))
			return res, nil
		}}
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
