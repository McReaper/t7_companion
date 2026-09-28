# Workflow extras: target-rig tooling, anim-layer edits, and prop playback

Referenced from `plugin/skills/anim-retarget/SKILL.md`. Three loosely-related pieces of detail that come up less often than the core retarget paths, grouped here rather than as separate files: getting a loose `.xmodel_bin` into Maya as a target rig, the display trap when correcting a baked anim on an anim layer, and playing back a non-character animated prop in-game.

## Getting a loose `.xmodel_bin` into Maya as a target rig — unpack it, don't recompile-and-re-rip

A `.xmodel_bin` is an **LZ4-compressed `.xmodel_export`** (magic `*LZ4*`, `uint32` decompressed size at +5, LZ4 **block** stream at +9), so mesh *and* skeleton are recoverable as text. Three converters can do it — `export2bin.exe` (Treyarch, in `bin\`, EXPORT→BIN only), `exportxbin.exe` (Scobalula, <https://github.com/Scobalula/exportxbin>, both directions), and **`exportx.exe`** ("ExportX", DTZxPorter, <https://dtzxporter.com/tools/exportx>, both directions). ExportX is the one that worked from the command line:

```
exportx.exe -f <path>\model_LOD0.xmodel_bin -m export     # -m bin (default) goes the other way
```

Verify the first lines — `NUMBONES` / `NUMVERTS` / `NUMFACES` — then import via CoDMayaTools. ExportX is a standalone exe; drop it in `bin\` next to the other two. If `exportxbin` reports `Failed to decompress binary file … return: 0`, that's the **tool**, not a bad rip — try its drag-and-drop mode (its documented primary usage) or switch to ExportX rather than re-ripping. Fall back to "compile into a map/mod and re-rip with Greyhound/Saluki as `.cast`" only when you actually want Cast (materials/images alongside the rig).

## Correcting a baked anim on an anim layer — the display gotcha that wastes time

To tweak an artifact non-destructively after bake (an arm/hand offset, a wrist), use an **additive anim layer**. The trap that misleads: a freshly `Create Empty Layer` shows the **dense BASE keys** in the timeline/Graph Editor, so the empty layer looks like it "inherited every frame" — it hasn't.

**The timeline reflects a layer's own keys only once the joint is a MEMBER of that layer.** So: `Layers > Create Empty Layer` → select the joint → **`Add Selected Objects`** → *now* the timeline shows the layer's real (empty) keys, and your corrections land cleanly. Skipping "Add Selected Objects" is what makes the layer look polluted.

Then: make the layer **active** (highlighted), rotate the joint, `S`. A constant misalignment needs just **2 keys** (range start + end) — the additive offset holds across the dense base frames, no per-frame re-keying. (Additive preserves the base *motion* shifted by your offset; to fully replace a limb's motion over a range, use an **Override** layer instead.)

**Prerequisite:** HIK **Source = None** first (see "Bake, then unbind" in SKILL.md). If the retarget is still live and Auto Key is on, every scrub bakes a key onto the active layer and it fills up *for real* — the tell is that deleting all keys leaves the anim still playing (HIK is still driving it).

## Playing an animated prop (not a character) in-game

**An xanim-driven animated prop (ported IGC fxanim: rope, cloth, debris) uses `AnimScripted`, NOT `scene::play` or `SetAnim`.** (If APE shows the model as *Is Siege* — a `*_smod` — none of this applies: siege props take a `sanim` and play client-side through a scene, see the animation skill.) Playing such a model's anim via a scene bundle's `MainAnim` — or via `SetAnim` on its animtree — leaves the mesh **frozen** (the model spawns, no bones move). What works is the cymbal-monkey verb: `model UseAnimTree(#animtree); model AnimScripted("note", origin, angles, %anim);` — it advances the scripted anim frame-by-frame on the model's own skeleton. The anim must still be listed in the animtree you `#using_animtree`.

**Don't attach an IGC fxanim prop to its moving parent — play it at the shared scene origin.** A ripped fxanim (e.g. a rappel rope hanging off a heli) typically has **no root motion** (its `tag_origin`/PART 0 is static every frame) yet its *child bones* carry the full world-space sweep (verify: PART 1's `OFFSET` varies hugely across frames). Since the prop anim and the vehicle anim were authored on the **same IGC origin**, `AnimScripted`-ing the prop at that same origin makes it track the moving vehicle *for free*. `LinkTo`, the scene `AlignTargetTag`, and `scene::play` **on** the vehicle all fight this — each either froze the rope or killed the vehicle's own anim. Attach nothing; co-locate the origins.

## CoDMayaTools export bugs (patch the `.py`)

**`ValueError: No object matches name: XAnimExporterInfo.notetracks[1]`** kills an XAnim export outright, and it has nothing to do with your anim. `cmds.getAttr` **raises** on an element of a multi attribute that was never written, so the source's `cmds.getAttr(...) or ""` never gets the chance to default — any export slot that has never had a notetrack saved blows up. Wrap the read:

```python
def GetNoteList(attr):
    try:
        return cmds.getAttr(attr) or ""
    except Exception:
        return ""
```

and call it from the two export paths (`ExportXAnim`, `ExportXCam`). The ~11 other reads live in the notetrack manager windows, which create the attribute before reading it. Reload the script in Maya afterwards — the in-memory copy is still the broken one.

The rest are **XCam-export code still written for Python 2**; on modern Maya (Py3) each fails with a different traceback. All are one-line fixes in `CoDMayaTools.py`:

- `TypeError: a bytes-like object is required, not 'str'` → `…encode('ascii','ignore').replace('\\','/')` — in Py3 `.encode()` returns bytes; **drop the `.encode(...)`**, keep the `.replace`.
- `TypeError: 'float' object cannot be interpreted as an integer` → `range(0, numframes)` with a float → **`range(0, int(numframes))`**.
- Linker error at build time `JSON: Value is not an int64_t` on the xcam → the export wrote `"framerate": 30.0` / `"numframes": 1096.0` as **floats**; the linker wants ints → cast at the source (`"framerate": int(fps)`, `"numframes": int(fLength)`), or integer-ise those two keys in the `.xcam_export` after export.
