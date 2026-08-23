# The XCam path

Referenced from `plugin/skills/anim-retarget/SKILL.md`'s "Moving the first-person CAMERA" section — read that section first to confirm XCam is the right mechanism for your clip (a cinematic on a `Player`-object scene). This file covers building and playing the XCam itself: the Maya camera setup, the xcam asset, and the FOV/DOF export-field conversion.

An **XCam** is a dedicated camera animation asset, played per-client with **`PlayMainCamXCam` (CSC)**. Because it's CSC/per-client, it also satisfies "every player sees the cinematic" in co-op (each client plays it on its own camera). `PlayMainCamXCam` is how the shipped campaign/MP cinematics drive their cameras.

## Making the XCam in Maya

Work in the scene where the retargeted rig's `tag_camera` animates.

1. **Create a camera** (`Create → Cameras → Camera`).
2. **Snap it to `tag_camera`** (position + rotation): select `tag_camera` then the camera, `parentConstraint tag_camera camera1;` then delete that constraint (leaves the camera at the tag). Do **not** use `-mo` here (you want an exact snap, not the current offset).
3. **Fix the axis offset** — a Maya camera looks down its **−Z**, a CoD `tag_camera`'s forward is **+X**, so the raw-inherited orientation looks sideways/into the body. Apply a relative object-space rotation (`rotate -r -os -fo 90 0 -90 camera1;` is the usual starting point — verify by looking through the camera, adjust by 90° steps until forward is correct).
4. **Re-constrain WITH `-mo`** (`parentConstraint -mo tag_camera camera1;`) — now the corrected aim is locked and it follows the anim. (This is the one place `-mo` is right on Path A.)
5. **Bake** the camera, then **Call of Duty Tools → Export XCam** (frame range = full anim). The sub-camera name in the export is your Maya camera's name (e.g. `camera1`) — you need it to play the XCam.

## The xcam asset + playing it

- **Asset** (`xcam.gdf` in your GDT, or APE): `filename` → the `.xcam_export`; **`use_firstperson_player` = 1** → this is what makes the **first-person body/arms render** during the XCam (the missing-hands fix lives here, not in the scene); `parent_scene` → the scene bundle that animates the body; `disableNearDof` = 1 to kill close-range blur. Zone it `xcam,<name>`.
- **Body vs camera are separate**: the XCam is only the camera (+ FP body visibility). The body animation still comes from playing the retargeted anim on a model (a `Prop`-type scene object is simplest — it plays the xanim directly, no `all_player.atr` needed).
- **Play** (CSC): `PlayMainCamXCam(localClientNum, "<xcam>", lerp, "<subcam>", "", origin, angles)` — `<subcam>` is the Maya camera name; `origin`/`angles` are the world placement (the scene's align point). `StopMainCamXCam(localClientNum)` ends it. Bridge from server logic with a **clientfield** (GSC sets it → a CSC callback calls `PlayMainCamXCam`). Note the CSC side **cannot read a server-side struct**, so pass the base origin/angles as constants that match the scene's spawn point.

## Camera settings ↔ export values (FOV / DOF), and the conversion

The export's per-frame `fov`/`fdist`/`fstop` come from the Maya camera (CoDMayaTools `ExportXCam`), and the FOV conversion is non-obvious:

| Export field | CoDMayaTools formula | Maya attribute (`cameraShape`) |
|---|---|---|
| `fov` | `verticalFieldOfView(deg) × 1.5714` | Focal Length (+ film back) — **the VERTICAL FOV, not Maya's displayed horizontal "Angle of View"** |
| `fdist` | `focusDistance × CM_TO_INCH` (~0.3937) | Depth of Field → Focus Distance |
| `fstop` | `fStop` (direct) | Depth of Field → F Stop |

Consequences seen in practice: a default camera exports `fov ≈ 59.5` (vertical ~37.9° × 1.5714), which won't match the horizontal Angle of View Maya shows. And **over-strong DOF** is usually a tiny **Focus Distance** — e.g. a ~5 cm focus distance exports `fdist ≈ 1.97`, focusing ~2 units away and blurring everything past it; set Focus Distance high (≈ 2000 → `fdist ≈ 800`) so the scene is sharp, or raise F Stop. For fast iteration these values are **constant per-frame in the `.xcam_export`** and can be edited there directly instead of re-exporting.

> **FOV is driven by `flen`, NOT `fov` — the `fov` field is a decoy.** Each camera block in the `.xcam_export` has `"aperture": "FOCAL_LENGTH"`; with that mode the game **derives the runtime FOV from `flen` (focal length), and ignores the `fov` field entirely**. Editing `"fov"` (any value, 40→160) changes nothing in-game — verified — and neither does `cg_fov` (a played main-cam xcam ignores it too). To widen/narrow the cinematic FOV, edit **`flen`**: **lower `flen` = wider FOV** (e.g. `flen 10` is very wide; ~10–14 for a ~120°-ish feel; shipped CAC inspect cams sit near `flen 27` = tight). Also set **`aspectratio` to `1.7786`** (16:9) — CoDMayaTools may export `1.5`, which skews the framing. The clean source-side fix is the Maya camera's **Focal Length** attribute (it writes `flen`); editing `flen` (all per-frame occurrences + the camera-def) directly in the export is the fast iteration path.
