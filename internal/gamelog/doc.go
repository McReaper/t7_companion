// Package gamelog reads the game's console_mp.log, for an agent investigating
// what a run did: which file is the current one, the lines of its last
// launch, what in them is the map's and what is stock noise, and the raw lines
// asked for. Every finding carries its line number in the file, so the agent
// can read around it itself when the sorting gets a line wrong.
//
// # Which file
//
// The game writes console_mp.log only with the logfile dvar set (the build's
// run stage sets logfile 2), in mods/<fs_game>/ when that folder exists, else
// in the game's root, where a usermap's goes although it runs with fs_game too
// (verified on real runs). Stale copies of earlier sessions sit in both
// places, so the current one is the newest by mtime: of the named map's or
// mod's two places when a name is given, else of the root's and every mod's.
// The root's crashes.log (a hard crash) and console.log (the frontend) are
// listed beside it.
//
// # Which lines
//
// A launch rewrites the file under one `logfile opened on <date>` header; with
// log_append it holds earlier launches too, so only the lines after the last
// header are read. One launch can load the map more than once (devmap again,
// map_restart; a stock template logs a second `Game Initialization` after its
// `ModLoad done: <map>`), so the map loads' lines are listed, not cut to.
// While developer is on, the engine prefixes lines with `[<ms>][<TAG>]`; the
// sorting reads them without it (verified on real logs).
//
// # What is the map's
//
// Most of a load's Error lines are stock noise: a stock template map logs about
// 240 failed lookups (`Error: Could not find fx "…"`, materials, weapons a
// usermap doesn't carry, `couldn't exec mod.cfg`, `Waited … for missing
// asset`), verified on a real run. So the sorting gives a script error (the
// S.R.E. block, `******* script error *******` to a line of stars, its lines
// prefixed `Error: `) with its message and frames, a lookup that failed for an
// asset the map's or mod's own zones list, and an Error line of any other kind;
// the rest it counts by asset type. A fatal error's dialog line is in the log
// as `Com_ERROR: ERROR: …`.
//
// # Watching
//
// A Watcher (t7kb watch, which the t7kb plugin's monitor runs for the
// whole of a Claude Code session) says each problem once, as one line, as it
// happens: whatever the game's run, a reload from its console (devmap,
// map_restart) included. It follows every console_mp.log from its end, and a
// log a new launch rewrote (its header changed) from its start, reading only
// complete lines. It says a script error once per launch; a fatal error
// (Com_ERROR); once the map has loaded, the assets its zone lists that the
// game didn't find, else the Error lines of no known kind; a dialog the game
// shows, unless it repeats a fatal error the log said; and a process that
// ended with an error code. Stock noise is never said, and a launch says at
// most maxEvents lines, the last one pointing to game_log: a monitor that
// prints too much is stopped.
package gamelog
