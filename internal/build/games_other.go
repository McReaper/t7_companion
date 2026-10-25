//go:build !windows

package build

import (
	"os/exec"
	"syscall"
)

// Games are the running game's processes: the game runs on Windows only.
type Games struct{}

// NewGames follows the game's processes.
func NewGames() *Games { return &Games{} }

// Running are the pids of the game's processes: none here.
func (g *Games) Running() []int { return nil }

// Dialogs are the dialogs the process shows: none here.
func (g *Games) Dialogs(int) []string { return nil }

// Exit is the exit code of an ended process.
func (g *Games) Exit(int) (uint32, bool) { return 0, true }

// Runs reports whether a process of the executable runs: the mod tools run on
// Windows only.
func Runs(string) bool { return false }

// alive reports whether the process pid still runs.
func alive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// Detach makes a command run on its own, in a session of its own, so it
// outlives the starting process.
func Detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
