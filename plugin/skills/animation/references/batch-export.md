# Batch and headless export

Referenced from `plugin/skills/animation/SKILL.md` — read that file first. This is the scripted-export detail: driving CoDMayaTools without its GUI, and converting many files at once.

## Scripted export: `ExportXAnim` needs three things the GUI gives it for free

Driving CoDMayaTools from a script skips the export **button**, which is where some of the setup lives. Each omission fails differently:

- **It exports only what is SELECTED.** `GetJointList` walks the whole hierarchy but includes a joint only if `selectedObjects.hasItem(dagPath)` — so selecting just the root writes a valid file with **`NUMPARTS 1`**, no error. Select every joint (`listRelatives(group, ad=True, type="joint")`), and **assert `NUMPARTS`** against that count after writing; nothing else catches it.
- **The progress bar is created by the button, not the window.** `ExportXAnim` does `cmds.progressBar(OBJECT_NAMES['progress'][0], edit=True, …)` and dies with `Object 'CoDMayaToolsProgressbar' not found`. Re-create it as `GeneralWindow_ExportSelected` does: a window named `"w" + <progress name>`, a `columnLayout`, then the `progressBar`.
- **`XAnimExporterInfo` is a SCENE node.** `RefreshXAnimWindow()` creates it (a `renderLayer` holding the notetrack/path attrs). The window's controls survive a scene change; this node does not. In a loop that re-opens a template scene per anim, call `RefreshXAnimWindow()` **after every open** or every export dies on `No object matches name: XAnimExporterInfo.notetracks[1]`.

Set the frame range and FPS by editing the window's fields directly (`<win>_FrameStartField`, `_FrameEndField`, `_FPSField`, `_qualityField`) — `ExportXAnim` reads them, not the playback range.

**`exportxbin.exe` breaks down in bulk.** A folder argument prints `No files processed` despite the tool's own help offering folders, and passing ~90 files in one call **segfaults partway** (48 converted, then a crash — with no non-zero exit to warn you). Convert **one file per invocation** in a loop and count the outputs; that is reliable.
