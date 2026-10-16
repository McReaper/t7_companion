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
// Edit applies a batch of ops (box, entity, delete, set, move, material,
// texture, copy) in order to a copy of the snapshot, unit by unit (a world
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
// same W, H and offsets line up across brushes. Radiant's own Fit Texture on a
// test box at 0, 90 and 30 degrees pins these (TestAFitLands…, TestAt30…); at
// 30 it sizes v along the face's bounding box's diagonal, so a fit here, over
// the face's corners, is the true one. A texture op's default tile, and a new
// box's, is what Tiles gives: map_edit's is the size the install's maps use the
// material at most, else its GDT's tiling at 16 texels a unit.
//
// # Checks
//
// CheckTextures reads the world brushes' faces for what draws badly. A drawn
// face is hidden when the opaque brushes' faces on its plane, facing it,
// cover it whole, one alone or several together (what is left of it once
// each is clipped away is nothing, slivers under eps aside), or when it lies
// inside an opaque brush; on the stock prefabs' geometry 86% of the faces it
// calls hidden are caulk or another tool material, against 56% of all faces
// (TestHiddenFacesAreTheOnesTreyarchCaulked).
// Two opaque drawn faces on one plane, facing the same way, overlapping by
// an area and drawing otherwise (another material, or tiles that don't
// meet) fight: the engine flickers between them (community, t7kb video
// 0.30), as a floor laid inside the template's ground does; the stock
// prefabs hold 570 such pairs among 117,778 faces, left in by hand. A seam
// is two faces of one material on one plane, both seen, sharing an
// edge or overlapping, whose tiles differ (size, rotation, mirror, or offset
// modulo a tile): faces meeting at a corner alone show none. Coplanar faces
// are found by their rounded plane, so the checks scale with a map.
//
// # Layouts
//
// A layout op builds a storey from a plan of rooms, rectangles meeting along
// their edges. Every line a room's edge lies on is cut where an edge on it
// starts or ends, so each stretch of wall has one room, or the outside, on
// either side, and a wall across it meets it at each end (a room's corner is
// on both its edges). A stretch is a box centred on its line, through the
// floor and ceiling slabs; the walls along x run on half a wall past the end
// of their line's run and take the corners, the walls along y stop half a
// wall short of every end, against them: no two pieces overlap, no corner is
// left open. Slabs fill each room between its walls. An opening sits in the
// stretch that holds it between those end walls, the stretch cut in strips
// around it, a sill under it (a door's threshold, of its room's floor) and a
// lintel over it. A stretch's sides take the material of the room each looks
// into; the faces the pieces hide of each other (selfHidden, the checks' own
// test) are caulk.
//
// # Boards
//
// ShowMaterials shows materials as Radiant draws them: a 256-unit slab of
// each, at its tile, in rows 512 units above the map, framed from straight
// above, fullbright. ShowModels places a misc_model of each, four to a row
// 128 units apart, framed from the front under fake lighting. Both hide the
// volumes and the sky's brushes for the shot; then the edit that built them is undone
// when the undo menu's last entry is still it, else the slabs are deleted
// (one more undo entry). Radiant's redo holds the board. Sheet shows them
// as Radiant's Textures pane does instead, without Radiant: their color
// images, read from the install's files (Thumb), four across, numbered.
//
// # Commands
//
// Command runs a Radiant menu entry (a CSG, a patch tool) on what the ids
// name, selected alone, then selects the user's selection again, whatever
// happened. What it did is read from the map before and after (GUIDs
// created and deleted), and from the undo menu, reported whenever the map
// changed, as two commands alike leave the same label.
package level
