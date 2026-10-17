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
// a payload, a header line and a body; hello first. Its requests run in
// Radiant's main thread one after the other, so replies come in request order;
// a reader goroutine hands each to its caller by id, and events to Events. A
// caller that gives up on a reply (its context ended) leaves the request
// running: closing the connection is what drops the requests still queued.
// While a modal dialog is open, everything but hello, state and menus fails
// with the code busy.
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
// over unsaved changes unless asked, a dialog, or an open prefab. With no
// Radiant running it starts bin/radiant_modtools.exe on the map from bin/, as
// the Launcher does, out of the caller's job so it outlives the MCP server, and
// waits for its bridge and then for the map. The plugin must be installed in
// bin/plugins/T7RadiantBridge; MTPlugins loads it.
package radiant
