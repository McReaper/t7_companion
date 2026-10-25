// Package level is the map open in the user's Radiant as the map tools see
// it: read through the bridge (internal/radiant), searched, framed and shown.
// The bridge carries text and pixels; everything that shapes them for an agent
// is here.
//
// # Ids
//
// Every entity and brush has a GUID; an agent names them by a short id, the
// GUID's first 8 hex digits, more when another GUID of the map starts the same
// (IDs). Radiant's GUIDs are time-based, so the first group differs between
// any two it made, and an id stays the same from one call to the next. Resolve
// takes any start of a GUID back, and says which ids an ambiguous one names.
//
// # Bounds
//
// A classic brush's box comes from its vertices, where three of its face
// planes meet inside all the others; Radiant writes a face's three points so
// that (p1-p0)×(p2-p0) points into the brush. A patch's box is its control
// points'. These are the geometry's own bounds: Radiant's selection widens a
// box thinner than 0.25 on an axis to 0.25 about its middle, and gives a brush
// whose planes enclose nothing none, as BrushBox does
// (TestBrushBoxesMatchRadiants checks every brush of a map against Radiant).
//
// # Reading the map
//
// Read takes one snapshot per call: state, then the entities' text (the
// bridge's entities, which Radiant writes as it saves, prefabs as misc_prefab
// entities) and the selection. Under a modal dialog the entities can't be read
// and Read says which dialog to close.
//
// # Views
//
// View shoots a view through the bridge's capture and writes ids on it at the
// points the bridge projected, so the image and the map agree without knowing
// Radiant's lens. A target framed by ids spans at least 256 units, so a point
// entity shows with the room around it.
//
//   - 3D: a framed target is shot from the south-west, above (Looking), from
//     where its corners just fill the view (FrameBox). Radiant's lens is a
//     pinhole, px = w/2 + f·x/z, and its focal length isn't given: View guesses
//     it, recovers f from where the target's corners landed (FocalFrom), and
//     shoots again when it is off by more than lensSlack or the corners overflow
//     the frame: a frame from a lens 5% off fills the view 5% off, which the
//     corners filling more than half of it doesn't catch. The lens is kept per
//     Radiant, so the next view takes one shot.
//   - 2D: the view shows one axis; a capture of another fails naming the axis
//     shown, and View/Layout/Next turns it XY, XZ, YZ. Zooming moves the
//     view's centre (towards the mouse), so a frame sets the zoom first, from
//     XY 100% in steps of 1.25, then centres: the camera goes to the target,
//     Center 2D On Camera, the camera comes back.
//
// Everything View changes of the user's views (render method, 2D axis, zoom
// and centre, camera) is put back, last change first, even when the shot
// failed; the zoom comes back to the nearest step of 1.25, and what could not
// be put back is noted in the answer. A bake (ViewOptions.Bake) runs
// Radiant's own, the lighting toolbar's bolt, under real lighting, and waits
// for the console's "All Probes Done": the console is subscribed to before
// the bake starts, as the client drops the events it can't hold. It lights
// Radiant's view only; the game's .led is the build's light stage.
//
// # Editing
//
// Edit applies a batch of ops (brush, patch, raw, entity, delete, set, move,
// material, texture, copy) in order to a copy of the snapshot, unit by unit (a world
// brush, or an entity with its brushes; "@n" names what op n made, every copy
// of a copy op), and sends what changed as the bridge's one edit: one
// undo step, applied whole or not at all. A point entity of the map is
// replaced in place and keeps its GUID; a world brush or an entity that owns
// brushes is deleted and imported anew, under a new GUID, as that is what
// Radiant's undo brings back exactly (a replaced brush entity comes back as
// world brushes). Then the map is read back, and each import found by its
// class and box among the GUIDs the edit's reply lists: Radiant's selection
// order is not the text's. The text is Radiant's own format, without GUIDs
// (the import gives them); the plugin refuses any line Radiant can't read,
// which Radiant itself would take for a fatal error.
//
// # Textures
//
// A face line's texture is "material W H u v rotation 0": Radiant projects it
// along the world axis the face's normal is closest to (z, then x, then y on a
// tie), u = (p·U + u)/W and v = (p·V + v)/H, with U, V = -x, +y on floors and
// ceilings, -y, +z on x walls, -x, +z on y walls, turned by the rotation (at
// 90, U runs where -V ran). So W and H are the world units one repeat spans (a
// negative one mirrors it), the offsets are world units, and faces given the
// same W, H and offsets line up across brushes. A texture op's default tile,
// and a new brush's, is what Tiles gives: map_edit's is the size the
// install's maps use the material at most, else its GDT's tiling at 16
// texels a unit.
//
// # Placing
//
// The agent places each element as it means it; no op lays anything out for
// it. A brush op's faces are the convex hull of its corners: every three
// corners whose plane has all the others on one side make a face, wound into
// the hull, one per plane. A patch op writes its control points as Radiant
// does, row by row (a curve's rows of 16 8, a mesh's of 0 8 on its size
// line), each point's texture coordinates its length along the columns and
// the rows at 16 a unit, as the stock mesh patches carry them. A raw op
// imports map text as written, its GUIDs dropped. All three refuse what
// Radiant's import couldn't read before anything is sent.
//
// # Commands
//
// Command runs a Radiant menu entry (a CSG, a patch tool) on what the ids
// name, selected alone, then selects the user's selection again, whatever
// happened. What it did is read from the map before and after (GUIDs
// created and deleted), and from the undo menu, reported whenever the map
// changed, as two commands alike leave the same label.
//
// # Watching Radiant
//
// A Watcher (t7kb watch, which the t7kb plugin's monitor runs) follows each
// Radiant whose bridge already listens, with a connection of its own
// subscribed to the console and map events; it never loads the bridge, and
// leaves for 30 s a bridge that refused it. What a Radiant has open when the
// watch connects is no news. It says a map Radiant opened, a prefab it
// entered or left, a bake once "All Probes Done" ends it (with what the bake
// warned of), and the Radiant closing; a save says nothing. Radiant writes its
// errors as console lines beginning "ERROR: " (an xmodel or material not in
// the gdtDB, a misc_prefab whose model won't load; a clean map's load writes
// none: verified in a test Radiant): they are said as one line per burst, once
// the console has been quiet for Quiet, each error once. The bridge's events
// are read as they come, as its client drops what arrives on a full channel.
// The map tools' own work comes through too: a map radiant_open opened, the
// errors of an edit naming an asset no GDT defines.
package level
