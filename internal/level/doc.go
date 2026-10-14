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
//     where its bounding sphere fills the view. Radiant's lens is a pinhole,
//     px = w/2 + f·x/z, and its focal length isn't given: View guesses it, checks
//     that the target's corners landed in the frame and fill it, and if not
//     recovers f from where they landed (FocalFrom) and shoots again; the lens is
//     kept per Radiant, so the next view takes one shot.
//   - 2D: the view shows one axis; a capture of another fails naming the axis
//     shown, and View/Layout/Next turns it XY, XZ, YZ. Zooming moves the
//     view's centre (towards the mouse), so a frame sets the zoom first, from
//     XY 100% in steps of 1.25, then centres: the camera goes to the target,
//     Center 2D On Camera, the camera comes back.
//
// Everything View changes of the user's views (render method, 2D axis, zoom
// and centre, camera) is put back, last change first, even when the shot
// failed; the zoom comes back to the nearest step of 1.25, and what could not
// be put back is noted in the answer.
//
// # Editing
//
// Edit applies a batch of ops (box, entity, delete, set, move, material) in
// order to a copy of the snapshot, unit by unit (a world brush, or an entity
// with its brushes), and sends what changed as the bridge's one edit: one
// undo step, applied whole or not at all. A point entity of the map is
// replaced in place and keeps its GUID; a world brush or an entity that owns
// brushes is deleted and imported anew, under a new GUID, as that is what
// Radiant's undo brings back exactly (a replaced brush entity comes back as
// world brushes). Then the map is read back, and each import found by its
// class and box among the GUIDs the edit's reply lists: Radiant's selection
// order is not the text's. The text is Radiant's own format, without GUIDs
// (the import gives them); the plugin refuses any line Radiant can't read,
// which Radiant itself would take for a fatal error.
package level
