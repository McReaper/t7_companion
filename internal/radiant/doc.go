// Package radiant drives a running Radiant through the T7RadiantBridge plugin
// (github.com/McReaper/t7_radiant_bridge, whose PROTOCOL.md this client
// speaks, protocol 1), and starts one on a map. Every map edit goes through
// Radiant, never through the .map file: the user's Radiant keeps the map in
// memory and would overwrite the file on its next save.
//
// # The connection
//
// Each Radiant with the plugin serves the pipe \\.\pipe\t7_radiant_bridge_<pid>.
// A Client is one connection to it: frames of a 4-byte little-endian length and
// a payload, a header line and a body; hello first, which refuses a plugin
// older than MinPlugin, the first with every command the client sends (the
// protocol's number changes only when a command changes meaning; a new command
// raises the plugin's version). Its requests run in Radiant's main thread one
// after the other, so replies come in request order; a reader goroutine hands
// each to its caller by id, and events to Events. A caller that gives up on a
// reply (its context ended) leaves the request running: closing the connection
// is what drops the requests still queued. While a modal dialog is open,
// everything but hello, state and menus fails with the code busy.
//
// # Machines
//
// A Machine is where the Radiants run: Local is this computer (Windows, the
// pipe opened for overlapped I/O so the reader and a writer don't serialise on
// one handle), and radianttest's Machine is a fake for tests, which run on any
// OS. Connect picks a Radiant by pid, or the only one running.
//
// # Opening a map
//
// Open finds the Radiant to use: the one with the map open already, else the
// only one running (or the one named by pid), which reloads the map — never
// over unsaved changes unless asked, a dialog, or an open prefab. A Radiant
// running without the bridge gets it first. With no Radiant running it starts
// bin/radiant_modtools.exe on the map from bin/, as the Launcher does, out of
// the caller's job so it outlives the MCP server, loads the bridge into it,
// and waits for the bridge and then for the map.
//
// # Loading the bridge
//
// t7kb ships the bridge beside its binary, in bridge/ (load.go): the user
// installs nothing else, and t7kb writes nothing into the install unless
// MTPlugins is there. One host only ever runs in a Radiant, or both would hook
// the same functions of it:
//
//   - MTPlugins installed (its d3d9.dll proxy in bin/ with RadiantLoader.ini,
//     not turned off) loads its host into every Radiant: t7kb copies its
//     plugin build into bin/plugins/T7RadiantBridge/ when the install's
//     differs, and has the host load it through its control window
//     (WM_COPYDATA, replies in RadiantPlugins.ctl), as MTPlugins' own tools do.
//   - Otherwise t7kb injects the standalone bridge, which carries its own
//     host: LoadLibraryW run in Radiant by a remote thread, once Radiant takes
//     input (its main window exists). Its file name carries its version, as a
//     running Radiant keeps the one it loaded locked; one already loaded is
//     never loaded twice, so a Radiant still running an older one is closed
//     and opened again.
//
// The light stage's headless Radiant (-ledSilent) gets no bridge.
package radiant
