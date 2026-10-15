// Package build drives the Black Ops 3 mod-tools build pipeline headlessly —
// gdtdb, cod2map64, radiant_modtools' light bake, linker_modtools, then
// optionally the game — so an agent builds a map or mod without the GUI
// Launcher and gets back a compact per-stage Report instead of the hundreds of
// lines each tool prints. It backs the build MCP tool and `t7kb build`
// (internal/cli/build_mcp.go, build_cmd.go), which must behave identically. It
// is a thin orchestrator: build logic belongs to the mod tools, and this
// package only runs them the way the Launcher does. Run is one of t7kb's three
// side-effecting entry points, with gdt_edit and create: it executes the
// Windows mod tools, and writes what they write.
//
// # Running the tools
//
// Each tool runs with its own directory as working directory, like the
// Launcher: cod2map can't load default_navmesh_settings.json from anywhere
// else and then silently skips the navmesh, and gdtdb ties the asset paths it
// records to its cwd, so a different one makes a Launcher-built gdt.db flag
// every asset as a phantom duplicate. Arguments go through exec, never a
// shell: git-bash/MSYS mangles /update and +medium and misreports native exit
// codes. cod2map exits 0 even when it wrote nothing, so the compile stage
// trusts the .d3dbsp's mtime. The light bake (radiant_modtools -ledSilent)
// detaches and gives no usable exit code: the stage polls for its .led.
//
// # The link's verdict
//
// The linker exits non-zero on warnings too, so its exit code is not the
// verdict: every errorlog the link wrote is (zone_source/*/assetinfo, one per
// language pass; linklog.go). An error (^1) fails the stage; warnings (^3) are
// listed, and a link with only warnings built its fastfile. After a link that
// built, the stage lists what the linker looked up and found in no GDT, read
// from the zone's .deps (zone.MissingLookups): it links on without it and logs
// nothing. A mod links each of core_mod, mp_mod, cp_mod and zm_mod present, in
// that order, as the Launcher does. With FreshXpak the link sets the target's
// .xpak files aside so the linker writes them from scratch (it only ever
// appends), then keeps the new ones, or puts the old ones back after a failed
// link (xpak.go). Its language, unless asked for, is the Launcher's Build
// Language (the registry, dvars_windows.go), else english: the game loads a
// map's <language>_<map>.ff in its own language and stops on a fatal error
// without it, so a link in another language than the Launcher's builds a map
// the user's game can't open.
//
// # The run stage
//
// It starts BlackOps3.exe with the Launcher's own arguments (gameArgs) and
// Steam's app id in its environment (gameEnv), each saying why, and counts the
// game launched once it stays up 8 s: a game that can't attach to Steam exits
// at once. Its dvars are Options.Dvars over, with LauncherDvars, the ones
// saved in the Launcher's Dvars dialog (the Windows registry,
// dvars_windows.go). It then watches the game's first 90 s (game.go): the game
// shows its fatal errors, and asks its questions (safe mode after a crash), in
// a message box, and waits on it, so a dialog of its process fails the stage
// with what it says, and the last lines of the console the game opens beside a
// fatal error; so does the game closing. The dialog is read with WM_GETTEXT
// (window_windows.go), as GetWindowText reads no other process's controls. The
// game keeps running whatever the stage says.
package build
