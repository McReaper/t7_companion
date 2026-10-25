package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mapToolset edits the map open in the user's Radiant, through the
// T7RadiantBridge plugin: internal/radiant has the connection's rules. Its
// tools refuse an argument they don't take, so another tool's option or a
// misspelt one is an error, not ignored.
func mapToolset() toolset {
	return toolset{name: "map", tools: strictArgs([]server.ServerTool{radiantOpenTool(), mapFindTool(), mapViewTool(), mapEditTool(), mapCommandTool()})}
}

func mapCommandTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_command",
		mcp.WithDescription("Run one of Radiant's own menu entries on entities and brushes of the open map: they are "+
			"selected alone for it, then the user's selection comes back. For what map_edit has no op for: CSG "+
			"(Selection/CSG/CSG Hollow, CSG Subtract, CSG Merge), patches, textures, prefabs. The answer reads the map "+
			"back: the undo entry the command made (one Ctrl+Z), what it created (id, class or material, bounds), what it "+
			"deleted, what it left selected. A command that opens a dialog waits on the user. menu instead of command "+
			"lists a menu's entries (\"Selection/CSG\"; \"\" the menu bar), \" >\" marking a submenu. A label that holds a / "+
			"is written as it reads (Selection/Select By/Select by Key/Value), except in a Radiant MTPlugins hosts the bridge in."),
		mcp.WithString("command", mcp.Description("The menu path, labels from the menu bar down joined by /: Selection/CSG/CSG Hollow.")),
		mcp.WithString("ids", mcp.Description("What to select for it, comma- or space-separated (default nothing selected).")),
		mcp.WithString("menu", mcp.Description("List this menu's entries instead of running a command: its labels from the menu bar down joined by / (Selection/CSG); \"\" the menu bar.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to use when several run."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			var menu *string
			if m, ok := r.GetArguments()["menu"].(string); ok {
				menu = &m
			}
			return mapCommand(ctx, r.GetString("command", ""), splitIDs(r.GetString("ids", "")), menu, r.GetInt("pid", 0))
		})}
}

// mapEditHelp is map_edit's description, the ops' one statement: t7kb map
// edit's help is it too.
const mapEditHelp = "Edit the map open in Radiant: a batch of ops, applied in order, lands as ONE undo step " +
	"(the answer's undo names it; one Ctrl+Z in Radiant takes it all back) or not at all. The agent places each " +
	"element as it means it; nothing is laid out for it. Ops, each an object with one key:\n" +
	"- {\"brush\": {\"points\": [[x,y,z]…], \"material\": m, \"contents\"?: flags as Treyarch's brushes carry, " +
	"\"weaponClip detail\" (only bullets hit it: trims, frames) or \"nonColliding\", \"class\"?: c, \"keys\"?: {…}}} " +
	"a convex brush through its corners (its faces are their hull: a box is its 8 corners, a wedge its 6, a column of " +
	"n sides its 2n; 4 to 64 points that enclose a volume), each face in the material at its own tile, in worldspawn " +
	"or as a new brush entity of class (info_volume, trigger_multiple, script_brushmodel)\n" +
	"- {\"patch\": {\"rows\": [[[x,y,z]…]…], \"material\": m, \"mesh\"?: false, \"contents\"?, \"class\"?, " +
	"\"keys\"?}} a patch through its control points, rows of as many each: a curve (Radiant's Bézier: 3, 5, 7… " +
	"each way) or a mesh (terrain: 2 or more), drawn on the side (next row's point - point) × (next point of the row - point) " +
	"points to (Radiant's own cylinder faces out so), its texture laid along the rows and columns by their length " +
	"(map_command runs Radiant's patch tools on it after)\n" +
	"- {\"raw\": text} map text as Radiant writes it, " +
	"entities with their brushes and patches (a worldspawn's brushes go to the world), its guid lines dropped " +
	"(the import gives new ones), every brush 4 faces or more or one patch\n" +
	"- {\"entity\": {\"class\": c, \"origin\": [x,y,z], \"angles\"?: [p,y,r], \"keys\"?: {…}}} a point entity\n" +
	"- {\"delete\": [ids]}\n" +
	"- {\"set\": {\"id\": id, \"keys\": {k: v, k2: null}}} (null removes)\n" +
	"- {\"move\": {\"ids\": [ids], \"by\": [dx,dy,dz]}}\n" +
	"- {\"copy\": {\"ids\": [ids], \"by\": [dx,dy,dz], \"count\"?: n}} n copies, each by further than the last\n" +
	"- {\"material\": {\"ids\": [ids], \"material\": m}} swaps the material, each face keeping its tile\n" +
	"- {\"texture\": {\"ids\": [ids], \"faces\"?: \"top\"|\"bottom\"|\"sides\"|\"+x,-y\"…, \"material\"?: m, " +
	"\"tile\"?: [u,v], \"scale\"?: s, \"origin\"?: [x,y,z], \"rotate\"?: deg, \"flip\"?: \"u\"|\"v\"|\"uv\"}} " +
	"lays a material on faces (all by default): tile is the world units one repeat spans (default the material's own, " +
	"as the install's maps use it), tiles start on origin (default 0 0 0, so faces given the same tile line up across " +
	"brushes)\n" +
	"Ids are map_find's, or @n: what op " +
	"n of this batch made (every copy of a copy op), so a brush placed, textured and copied is one undo step. A point " +
	"entity keeps its id; a world brush or an entity that owns brushes is rebuilt and gets a new one (was gives " +
	"the old). Every entity or brush of the map an op names needs its own rev from map_find, in revs: a brush's is the " +
	"one its brush_list row gives, not its entity's, so a world brush needs no rev of the worldspawn. The user works in the " +
	"same Radiant, and a batch naming something they changed or deleted since it was read is refused whole, nothing sent. " +
	"The answer reads the map back: each op's id and rev, class, origin or bounds, the tile each material got " +
	"and where it came from, the faces each texture op textured. Edits go through Radiant, never the .map file, " +
	"and the user's selection is kept."

func mapEditTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_edit",
		mcp.WithDescription(mapEditHelp),
		mcp.WithArray("ops", mcp.Required(), mcp.Description("The ops, in order."), mcp.Items(map[string]any{"type": "object"})),
		mcp.WithObject("revs", mcp.Description("The rev map_find (or a previous map_edit) gave each entity and brush of the map "+
			"the ops name, by id: {id: rev}, a brush's own, not its entity's. Required for every such id (@n, what this batch makes, needs none): the user "+
			"edits the same map, and a batch naming something they changed since it was read, or deleted, is refused whole.")),
		mcp.WithString("label", mcp.Description("What the batch does: Radiant's undo menu shows \"t7kb: <label>\" (default: the ops counted).")),
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
			var revs map[string]string
			if v, ok := r.GetArguments()["revs"]; ok {
				if err := decodeArg(v, &revs); err != nil {
					return nil, fmt.Errorf("revs: an object of id to rev: %w", err)
				}
			}
			return mapEdit(ctx, ops, revs, r.GetString("label", ""), r.GetInt("pid", 0))
		})}
}

func mapViewTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_view",
		mcp.WithDescription("See the map open in Radiant: an image of its 3D view or one of its 2D views (top, front, side), "+
			"as Radiant draws it, with the short ids of what is in it written where their origin or centre is (labels "+
			"gives the same as id: [x, y] in the image). Frame entities or brushes by ids, or a point by at (and radius); "+
			"shoot the 3D view from camera; or take the view as it is. A framed 3D shot looks down from the south-west. "+
			"Top shows the layout best (outlines, entity names). Pick the render by the question: fullbright for "+
			"textures, fake for shapes, bake for light (sun leaks, dark rooms): it bakes Radiant's lighting (its toolbar's "+
			"bolt, its view only; the game's is build's light stage) and shoots under real lighting, black on an unbaked "+
			"map; warned lists what the bake's console warned of (a probe out of range, a shadow dropped); once a bake ran, "+
			"closing Radiant asks whether to export the lighting, a dialog for the user to answer. A 2D answer gives its centre and scale (image pixels per unit). "+
			"The user's camera, render method, 2D axis, zoom and centre are put back (notes says what could not be); "+
			"Radiant must not be minimised. size is the image's longer side (default 768: about 400 tokens)."),
		mcp.WithString("view", mcp.Description("3d (default), top, front or side.")),
		mcp.WithString("ids", mcp.Description("Entities or brushes to frame, by id, comma- or space-separated.")),
		mcp.WithString("at", mcp.Description("A point x y z to frame, with radius around it.")),
		mcp.WithNumber("radius", mcp.Description("With at: how far around the point to show (default 256).")),
		mcp.WithString("camera", mcp.Description("3d: shoot from x y z pitch yaw roll (degrees; positive pitch looks up, yaw 90 faces +y).")),
		mcp.WithString("render", mcp.Description("3d: the render method for this shot: fake, fullbright, wireframe, real, view or density.")),
		mcp.WithString("hide", mcp.Description("volumes, sky or both comma-separated: hide for this shot the map's volumes and triggers (their tool textures cover a view from inside them too) or its sky brushes (a skybox covers a view from outside it); shown again after.")),
		mcp.WithBoolean("bake", mcp.Description("3d: bake Radiant's lighting first, at its toolbar's L/M/H, and shoot under real lighting (default false); after an edit, bake again to see its light.")),
		mcp.WithNumber("size", mcp.Description("The image's longer side in pixels (default 768, at most 1568).")),
		mcp.WithBoolean("no_labels", mcp.Description("Leave the ids off the image (default false).")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to see when several run."))),
		Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, png, err := mapView(ctx, mapViewArgs{View: r.GetString("view", ""), IDs: splitIDs(r.GetString("ids", "")),
				At: r.GetString("at", ""), Radius: r.GetFloat("radius", 0), Camera: r.GetString("camera", ""),
				Render: r.GetString("render", ""), Hide: r.GetString("hide", ""), Bake: r.GetBool("bake", false), Size: r.GetInt("size", 0), NoLabels: r.GetBool("no_labels", false), PID: r.GetInt("pid", 0)})
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
			"(the map tools' name for it: a GUID's first 8 hex digits) and its rev (what map_edit's revs want back), class, origin, and for brush entities their bounds "+
			"(min x y z, max x y z) and brush count; keys are the naming ones (targetname, target, script_noteworthy, "+
			"script_string, script_int, name, model) unless keys asks for others. brushes lists each one's brushes and "+
			"patches (id, rev, material, bounds, contents, curve/mesh); naming a brush's id lists just it, a patch with its "+
			"control points as rows, as map_edit's patch op takes them. near goes by bounds: a point entity by its own, an "+
			"entity that owns brushes (the worldspawn too) by its brushes', coming with just those within radius, nearest "+
			"first, and sorted by the nearest; its bounds and brush count stay the whole entity's. "+
			"No face is shown: Radiant holds the geometry."),
		mcp.WithString("ids", mcp.Description("Entity or brush ids (or GUIDs, or any start of one), comma- or space-separated.")),
		mcp.WithString("class", mcp.Description("Classname; * matches any run of characters (light*, script_*).")),
		mcp.WithString("key", mcp.Description("An entity key the entity has, or key=value (* in the value), e.g. targetname=door_1.")),
		mcp.WithString("near", mcp.Description("A point, x y z: what comes within radius of it by bounds, nearest first: point entities by theirs, brush entities and the worldspawn by their brushes', which they list.")),
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
			"under a dialog or inside a prefab; else starts Radiant on the map and waits for it (up to 3 minutes). It loads "+
			"its bridge into a Radiant that runs without it, through MTPlugins when that is installed. Answers the "+
			"Radiant's pid, the map's path and how it came to be open (already open, reloaded, launched)."),
		mcp.WithString("map", mcp.Required(), mcp.Description("A map's name (zm_castle: map_source/zm/zm_castle.map) or a .map path, relative to map_source/ unless absolute.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to use when several run (the error lists them).")),
		mcp.WithBoolean("discard", mcp.Description("Drop the Radiant's unsaved changes to load the map (default false: refused).")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH)."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			return radiantOpen(ctx, r.GetString("tools_path", ""), r.GetString("map", ""), r.GetInt("pid", 0), r.GetBool("discard", false))
		})}
}
