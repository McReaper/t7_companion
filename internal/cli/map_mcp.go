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
// T7RadiantBridge plugin: internal/radiant has the connection's rules. Its
// tools refuse an argument they don't take: their options share names
// (board, show) that mean different things from tool to tool.
func mapToolset() toolset {
	return toolset{name: "map", tools: strictArgs([]server.ServerTool{radiantOpenTool(), mapFindTool(), mapViewTool(), mapEditTool(), mapCommandTool(), mapCheckTool()})}
}

func mapCheckTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("map_check",
		mcp.WithDescription("Check the textures of the map open in Radiant, its world brushes (all, or those ids names), "+
			"for what draws badly, the worst kinds first: unknown (a material no GDT defines: the missing-texture pattern); "+
			"zfight (two opaque drawn faces on one plane, facing the same way, overlapping: they flicker in game, as a floor "+
			"laid inside the template's ground does; detail is the other face: delete or cut one); seam (faces of one material on one plane, both seen, sharing an edge, whose tiles don't meet; detail is the "+
			"other face: give both the same tile, rotation and origin); stretched (a tile more than half again longer one way "+
			"than the material's; detail is both tiles); hidden (a drawn face against an opaque brush's face, or inside an "+
			"opaque brush; detail says which: texture it caulk, as it can't be seen and still draws). Each finding names the "+
			"brush's id, the face (+x … -z, as map_edit's texture op takes faces) and the material; counts has every kind's "+
			"total. Run it after texturing, and fix with map_edit."),
		mcp.WithString("ids", mcp.Description("The brushes or entities to check, comma- or space-separated (default every world brush).")),
		mcp.WithString("kinds", mcp.Description("Only these kinds, comma-separated: unknown, zfight, seam, stretched, hidden.")),
		mcp.WithNumber("limit", mcp.Description("How many findings to list (default 50, at most 200); counts has them all.")),
		mcp.WithNumber("pid", mcp.Description("The Radiant to use when several run."))),
		Handler: jsonHandler(func(ctx context.Context, r mcp.CallToolRequest) (any, error) {
			return mapCheck(ctx, splitIDs(r.GetString("ids", "")), r.GetString("kinds", ""), r.GetInt("limit", 0), r.GetInt("pid", 0))
		})}
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
			"one key: {\"box\": {\"min\": [x,y,z], \"max\": [x,y,z], \"material\": m, \"contents\"?: flags as Treyarch's brushes carry, \"weaponClip detail\" (only bullets hit it: trims, frames) or \"nonColliding\", "+
			"\"class\"?: c, \"keys\"?: {…}}} a box brush, in worldspawn or as a new brush entity of class (volume_sun, "+
			"trigger_multiple); {\"shape\": {\"outline\": [[x,y]…], \"z\": [z0,z1], \"material\": m, \"contents\"?: as box, \"class\"?: c, \"keys\"?: {…}}} a solid on any "+
			"outline, concave too (cut into convex brushes): an angled wall, a hexagonal floor; "+
			"{\"walls\": {\"lines\": [{\"points\": [[x,y]…], \"closed\"?: true}…], \"doors\"?: [{\"at\": [x,y], \"width\"?: 72, "+
			"\"height\"?: 96, \"seen\"?: 56}…], \"material\": m, \"z\"?: [0,112], \"thickness\"?: 8, \"grid\"?: 8}} walls along "+
			"a plan's lines, centrelines as drawn: corners snap to the grid, a line a few units off an axis runs on it, an end "+
			"near another wall lands on it; corners are mitred, a wall ending on another's side stops on its face, walls "+
			"crossing split, nothing overlaps; each door is cut in the wall nearest at: width clear for the AI, nonColliding "+
			"jambs and clip_player narrowing it to seen for the eye and players only, a lintel, a weaponClip detail trim; "+
			"{\"entity\": {\"class\": c, \"origin\": [x,y,z], \"angles\"?: [p,y,r], \"keys\"?: {…}}} a "+
			"point entity; {\"delete\": [ids]}; {\"set\": {\"id\": id, \"keys\": {k: v, k2: null}}} (null removes); "+
			"{\"move\": {\"ids\": [ids], \"by\": [dx,dy,dz]}}; {\"copy\": {\"ids\": [ids], \"by\": [dx,dy,dz], "+
			"\"count\"?: n}} n copies, each by further than the last (a row of pillars or wall panels from one); "+
			"{\"material\": {\"ids\": [ids], \"material\": m}} swaps the material, each face keeping its tile; "+
			"{\"texture\": {\"ids\": [ids], \"faces\"?: \"top\"|\"bottom\"|\"sides\"|\"+x,-y\"…, \"material\"?: m, "+
			"\"tile\"?: [u,v], \"scale\"?: s, \"origin\"?: [x,y,z], \"fit\"?: [nu,nv], \"rotate\"?: deg, \"flip\"?: "+
			"\"u\"|\"v\"|\"uv\"}} lays a material on faces (all by default; \"hidden\": the faces other brushes hide, by face, caulk by "+
			"default, the way to caulk what nobody sees): tile is the world units one repeat spans "+
			"(default the material's own, as the install's maps use it), tiles start on origin (default 0 0 0, so faces "+
			"given the same tile line up across brushes), fit makes nu repeats span each face (0 sizes that axis as the "+
			"other) starting at its edge. New boxes get their material's own tile too. {\"layout\": {\"rooms\": [{\"name\": n, "+
			"\"min\": [x,y], \"max\": [x,y], \"material\"?: m, \"floor\"?: m, \"ceiling\"?: m, \"outdoor\"?: true, \"zone\"?: z}], "+
			"\"openings\"?: [{\"room\": n, \"to\": n2 | \"side\": \"-y\", \"at\"?: u, \"width\"?: 72, \"height\"?: 96, "+
			"\"sill\"?: 0}], \"material\": m, \"floor\"?: m, \"ceiling\"?: m, \"exterior\"?: m, \"z\"?: 0, \"height\"?: 112, "+
			"\"wall\"?: 8, \"slab\"?: 16, \"probes\"?: true, \"spawners\"?: 4}} builds a storey from its plan: rooms are rectangles meeting along "+
			"their edges, from wall centre to wall centre (a room 256 wide is 248 inside); a wall stands on every edge, one where two "+
			"rooms meet, each side of it in the material of the room it looks into (exterior outside, and in an outdoor room); each room gets a floor slab "+
			"under z and a ceiling height above (outdoor: none) and a reflection_probe (probes false: none); an opening is a door to "+
			"the room it opens on, or on a side of its room, 72 wide (two abreast), a window with a sill (56 by 64 then), at the world x or y u of its middle "+
			"(default the middle of the shared wall or side), the walls built around it; the faces the pieces hide of each other are "+
			"caulk, and the answer's part names each piece (\"hall floor\", \"hall|kitchen wall\"); rooms naming a zone get it, its "+
			"links through their doors. {\"zone\": {\"name\": z, \"areas\": [{\"min\": [x,y,z], \"max\": [x,y,z]} | {\"outline\": [[x,y]…], \"z\"?: 0, "+
			"\"height\"?: 128} | {\"floors\": [ids], \"height\"?: 128}], \"spawners\"?: 4, \"pathnode\"?: true, \"links\"?: [{\"zone\": z2, "+
			"\"flag\"?: \"always_on\", \"one_way\"?: false}]}} makes a zombies zone: an info_volume (player_volume) over each area, "+
			"any outline (cut into convex brushes) or the floor brushes' own; riser spawners spread on the floor found under it, clear "+
			"of walls; a node_pathnode for the navmesh to grow from; a zone the map has keeps its spawners' name. The answer's "+
			"script lists the add_adjacent_zone calls the map's script lacks (map_edit writes no script), its notes overlapping "+
			"zones and rooms in none. {\"probes\": {\"at\"?: [x,y,z], \"replace\"?: false, \"global\"?: false, \"height\"?: 384, "+
			"\"keys\"?: {…}}} probes the space players reach, any map: from at (default the info_player_start) it walks the "+
			"floors a player gets to (steps of 18 up, any drop off a ledge, a body's room; stairs, upper storeys and prefabs' "+
			"brushes included, clips and nonColliding brushes passed through), on a map with zones only what lies in them, and boxes them, the largest first, floor to ceiling, sides on the walls' "+
			"faces, none overlapping: a lintel or a storey parts boxes, a sealed space gets none (it would bake out of range), "+
			"a box over open sky stops height up; each a reflection_probe at Treyarch's defaults (4x, AO 8, blends 3), blends "+
			"longer over floor no box holds; replace deletes the probes standing in the boxes (kept by default, noted); global also moves "+
			"the global probe into the largest box (an info_null the volume_sun targets). Ids are map_find's, or @n: what op "+
			"n of this batch made (every copy of a copy op), so a box built, textured and copied is one undo step. A point "+
			"entity keeps its id; a world brush or an entity that owns brushes is rebuilt and gets a new one (was gives "+
			"the old). The answer reads the map back: each op's id, class, origin or bounds, the tile each material got "+
			"and where it came from, the faces each texture op textured. Edits go through Radiant, never the .map file, "+
			"and the user's selection is kept."),
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
			"Top shows the layout best (outlines, entity names). Pick the render by the question: fullbright for "+
			"textures, fake for shapes, bake for light (sun leaks, dark rooms): it bakes Radiant's lighting (its toolbar's "+
			"bolt, its view only; the game's is build's light stage) and shoots under real lighting, black on an unbaked "+
			"map; warned lists what the bake's console warned of (a probe out of range, a shadow dropped). A 2D answer gives its centre and scale (image pixels per unit). "+
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
