package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// bo3AppID is Black Ops III's Steam app id.
const bo3AppID = "311210"

// How the run stage watches the game it starts, as variables so tests can
// wait less. It counts as launched once it stays up gameStartup: without Steam
// attached it shows "you must launch Steam" and exits at once. Meanwhile it
// looks every gamePoll for a dialog of the game's: the game shows a fatal
// error in one and asks its questions in one (safe mode after a crash), and
// waits on it either way. What the game does next is the watcher's
// (gamelog.Watcher, t7kb watch).
var (
	gameStartup = 8 * time.Second
	gamePoll    = time.Second
)

// window is a visible top-level window of the game.
type window struct {
	Class, Title string
	Controls     []control // its child controls that have text, in Windows' order
}

// control is a window's child control: a message box's message is a Static
// control's text, its answers Buttons'.
type control struct{ Class, Text string }

// gameWindows lists a process's windows; a variable so tests can open one.
var gameWindows = processWindows

// The window classes the run stage reads: what every message box is, and the
// console the game opens beside a fatal error, holding its log.
const (
	dialogClass  = "#32770"
	consoleClass = "CoD Black Ops III WinConsole"
)

// consoleLines is how many of the console's last lines a dialog's report
// carries: the context of an error the dialog states in one line.
const consoleLines = 5

// runGame starts the game and watches its start: the stage fails if it exits or
// opens a dialog before gameStartup passes. The game keeps running either way.
func runGame(game string, args []string) StageResult {
	start := time.Now()
	exe := filepath.Join(game, "BlackOps3.exe")
	c := exec.Command(exe, args...)
	c.Dir = game
	c.Env = gameEnv(os.Environ())
	if err := c.Start(); err != nil {
		return StageResult{Name: "run", OK: false, Errors: []string{"could not launch game: " + err.Error()}}
	}
	exited := make(chan error, 1)
	go func() { exited <- c.Wait() }()
	sr := watchGame(c.Process.Pid, exited, start)
	sr.Name, sr.Seconds = "run", round1(time.Since(start).Seconds())
	return sr
}

// watchGame waits on the started game until it opens a dialog, exits, or
// gameStartup passes.
func watchGame(pid int, exited <-chan error, start time.Time) StageResult {
	tick := time.NewTicker(gamePoll)
	defer tick.Stop()
	deadline := time.After(gameStartup)
	for {
		select {
		case err := <-exited:
			return gameExit(err)
		case <-deadline:
			return StageResult{OK: true, Note: fmt.Sprintf("launched BlackOps3.exe: it stayed up its first %s", gameStartup)}
		case <-tick.C:
			if errs := dialogErrors(gameWindows(pid)); errs != nil {
				return StageResult{Errors: errs}
			}
		}
	}
}

// gameExit says why the game ended at once: Steam refused it.
func gameExit(err error) StageResult {
	how := ""
	if err != nil {
		how = " (" + err.Error() + ")"
	}
	return StageResult{Errors: []string{"the game exited right after starting" + how + " — is Steam running and signed in? A game it shows \"you must launch Steam\" in closes at once"}}
}

// dialogErrors is what the game's dialogs say, nil when it shows none: each
// one's title, message and buttons, then the last lines of the console the
// game opens beside a fatal error.
func dialogErrors(ws []window) []string {
	out := dialogLines(ws)
	if out == nil {
		return nil
	}
	var console []string
	for _, w := range ws {
		if w.Class == consoleClass {
			for _, c := range w.Controls {
				console = append(console, strings.Split(c.Text, "\n")...)
			}
		}
	}
	return append(out, consoleTail(console, out)...)
}

// dialogLines are the game's dialogs, one line each.
func dialogLines(ws []window) []string {
	var out []string
	for _, w := range ws {
		if w.Class == dialogClass {
			out = append(out, dialogLine(w))
		}
	}
	return out
}

// dialogLine is a dialog in one line: `the game opened a dialog, "Erreur
// fatale": ERROR: Could not find zone 'fr_zm_x' [OK]`.
func dialogLine(w window) string {
	var text, buttons []string
	for _, c := range w.Controls {
		t := strings.Join(strings.Fields(c.Text), " ")
		if c.Class == "Button" {
			buttons = append(buttons, "["+strings.ReplaceAll(t, "&", "")+"]") // "&Oui": & marks the key that presses it
		} else if t != "" {
			text = append(text, t)
		}
	}
	return strings.TrimSpace(fmt.Sprintf("the game opened a dialog, %q: %s %s", w.Title, strings.Join(text, " "), strings.Join(buttons, " ")))
}

// consoleTail is the console's last consoleLines non-empty lines, as
// "console: …", leaving out the ones a dialog already says.
func consoleTail(lines, said []string) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < consoleLines; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" && !saidIn(l, said) {
			out = append(out, "console: "+l)
		}
	}
	slices.Reverse(out)
	return out
}

func saidIn(l string, said []string) bool {
	for _, s := range said {
		if strings.Contains(s, l) {
			return true
		}
	}
	return false
}

// gameArgs are the Launcher's own run arguments (captured from the stock
// modlauncher.exe starting the game): fs_game is the map's name for a usermap
// too — without it the game never mounts usermaps/<map>/ and +devmap leaves it
// on the main menu. A mod is mounted with fs_game and loaded from the menu, so it
// gets no +devmap.
func gameArgs(name string, isMod bool) []string {
	args := []string{"+set", "fs_game", name}
	if !isMod {
		args = append(args, "+devmap", name)
	}
	return args
}

// gameEnv is the environment the game starts with: Steam starts the mod tools
// Launcher with SteamAppId/SteamGameId set and the game inherits them; started
// from anywhere else, BlackOps3.exe can't attach to the running Steam client and
// refuses with "you must launch Steam to play the game" (verified on a real
// install: the same command runs once SteamAppId=311210 is set).
func gameEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "SteamAppId") && !strings.EqualFold(k, "SteamGameId") {
			out = append(out, kv)
		}
	}
	return append(out, "SteamAppId="+bo3AppID, "SteamGameId="+bo3AppID)
}
