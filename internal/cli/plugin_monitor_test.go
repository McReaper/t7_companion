package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// monitorRun is the plugin monitor's script started the way Claude Code
// starts it: under a shell of its own, killed to stop the monitor. The shell
// waits on the script and writes its exit status; the script's output goes to
// files, so nothing it leaves running holds a pipe the test waits on.
type monitorRun struct {
	t          *testing.T
	sh         string
	out, errs  string
	exit       string
	pid        string
	shell      *exec.Cmd
	shellEnded bool
}

// monitorScript is the plugin's watch.sh, or a skip without sh to run it.
func monitorScript(t *testing.T) (sh, script string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on the PATH")
	}
	script, err = filepath.Abs("../../plugin/monitors/watch.sh")
	if err != nil {
		t.Fatal(err)
	}
	return sh, script
}

// startMonitor runs the script in dir with env added, under a shell that
// lives until stopShell.
func startMonitor(t *testing.T, sh, script, dir string, env ...string) *monitorRun {
	t.Helper()
	tmp := t.TempDir()
	r := &monitorRun{t: t, sh: sh, out: filepath.Join(tmp, "out"), errs: filepath.Join(tmp, "err"), exit: filepath.Join(tmp, "exit")}
	pidFile := filepath.Join(tmp, "pid")
	r.shell = exec.Command(sh, "-c", `"$0" "$1" >"$2" 2>"$3" & echo $! >"$4"; wait $!; echo $? >"$5"`, sh, script, r.out, r.errs, pidFile, r.exit)
	r.shell.Dir = dir
	r.shell.Env = append(os.Environ(), env...)
	if err := r.shell.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.stopShell)
	r.waitFor("the script to start", func() bool {
		b, _ := os.ReadFile(pidFile)
		r.pid = strings.TrimSpace(string(b))
		return r.pid != ""
	})
	return r
}

// stopShell kills the shell the script runs under, as stopping the monitor
// does.
func (r *monitorRun) stopShell() {
	if r.shellEnded {
		return
	}
	r.shellEnded = true
	_ = r.shell.Process.Kill()
	_ = r.shell.Wait()
}

// running says whether the script still runs: its shell wrote no exit status,
// and, once that shell is killed, sh still knows its pid.
func (r *monitorRun) running() bool {
	if _, err := os.Stat(r.exit); err == nil {
		return false
	}
	return r.alive(r.pid)
}

// alive says whether a process sh knows by pid still runs.
func (r *monitorRun) alive(pid string) bool {
	return exec.Command(r.sh, "-c", `kill -0 "$0" 2>/dev/null`, pid).Run() == nil
}

func (r *monitorRun) stdout() string { return readTrimmed(r.t, r.out) }
func (r *monitorRun) stderr() string { return readTrimmed(r.t, r.errs) }

// waitFor waits up to 20 s, well past the script's 5 s poll, for cond.
func (r *monitorRun) waitFor(what string, cond func() bool) {
	r.t.Helper()
	for end := time.Now().Add(20 * time.Second); !cond(); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(end) {
			r.t.Fatalf("timed out waiting for %s (stdout %q, stderr %q)", what, r.stdout(), r.stderr())
		}
	}
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// writeStub writes an executable sh script standing in for t7kb.
func writeStub(t *testing.T, dir, body string) string {
	t.Helper()
	stub := filepath.Join(dir, "t7kb")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return stub
}

// The plugin's monitor starts with every Claude Code session, and the mod
// tools set TA_TOOLS_PATH and TA_GAME_PATH for the whole machine, so its script
// runs `t7kb watch` only for a session working inside one of those roots,
// compared as Windows compares paths. Elsewhere it doesn't exit, which would
// wake the agent: it says why on stderr, stays, and ends with its shell.
func TestMonitorWatchesOnlyInsideTheGameRoot(t *testing.T) {
	sh, script := monitorScript(t)
	base := t.TempDir()
	root := filepath.Join(base, "Black Ops III")
	mapDir := filepath.Join(root, "usermaps", "zm_x")
	sibling := root + " backup"
	elsewhere := filepath.Join(base, "project")
	for _, d := range []string{mapDir, sibling, elsewhere} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stub := writeStub(t, base, "echo \"t7kb $*\"\n")

	for _, c := range []struct {
		name, dir, game, tools string
		watch                  bool
		why                    string
	}{
		{"in the root", root, "", root, true, ""},
		{"in a map's folder", mapDir, "", root, true, ""},
		{"in the root spelled with another case and a trailing backslash", mapDir, "", strings.ToUpper(root) + `\`, true, ""},
		{"in the game root alone", mapDir, root, "", true, ""},
		{"in the game root with the tools elsewhere", mapDir, root, elsewhere, true, ""},
		{"in a sibling sharing the root's prefix", sibling, root, root, false, "opened outside the BO3 folder, " + root},
		{"in another folder", elsewhere, root, root, false, "opened outside the BO3 folder, " + root},
		{"with no root set", root, "", "", false, "TA_TOOLS_PATH, and it is unset"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := startMonitor(t, sh, script, c.dir, "TA_GAME_PATH="+c.game, "TA_TOOLS_PATH="+c.tools, "T7KB_BIN="+stub)
			if c.watch {
				r.waitFor("the watch to end", func() bool { return !r.running() })
				if got := r.stdout(); got != "t7kb watch" {
					t.Errorf("stdout %q, want the watch run", got)
				}
				return
			}
			r.waitFor("the reason on stderr", func() bool { return strings.Contains(r.stderr(), "t7kb watch is off") })
			if !strings.Contains(r.stderr(), c.why) {
				t.Errorf("stderr %q doesn't say %q", r.stderr(), c.why)
			}
			if got := r.stdout(); got != "" {
				t.Errorf("stdout %q, want nothing: each line wakes the agent", got)
			}
			if !r.running() {
				t.Fatal("the script exited: its exit wakes the agent")
			}
			r.stopShell()
			r.waitFor("the script to end with its shell", func() bool { return !r.running() })
		})
	}
}

// Stopping the monitor kills only the shell Claude Code started, so the
// script stops the watch itself once that shell is gone.
func TestMonitorStopsTheWatchWithItsShell(t *testing.T) {
	sh, script := monitorScript(t)
	root := t.TempDir()
	pidFile := filepath.Join(root, "stub.pid")
	stub := writeStub(t, root, "echo $$ >\""+filepath.ToSlash(pidFile)+"\"\necho \"t7kb $*\"\nwhile :; do sleep 1; done\n")

	r := startMonitor(t, sh, script, root, "TA_GAME_PATH=", "TA_TOOLS_PATH="+root, "T7KB_BIN="+stub)
	var watch string
	r.waitFor("the watch to start", func() bool {
		watch = readTrimmed(t, pidFile)
		return watch != "" && r.stdout() == "t7kb watch"
	})
	if !r.alive(watch) || !r.running() {
		t.Fatal("the watch or the script ended before the monitor stopped")
	}
	r.stopShell()
	r.waitFor("the watch to end with the monitor's shell", func() bool { return !r.alive(watch) && !r.running() })
}

// Inside the root, what keeps the watch from running reaches the agent on
// stdout: t7kb not installed, or the watch's own error.
func TestMonitorSaysWhyTheWatchCantStart(t *testing.T) {
	sh, script := monitorScript(t)
	root := t.TempDir()
	failing := writeStub(t, root, "echo \"error: no game path\" >&2\nexit 1\n")

	for _, c := range []struct{ name, bin, want string }{
		{"without t7kb", "", "t7kb isn't installed"},
		{"when the watch fails", failing, "error: no game path"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(sh, script)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "TA_GAME_PATH=", "TA_TOOLS_PATH="+root, "T7KB_BIN="+c.bin, "LOCALAPPDATA="+t.TempDir())
			out, err := cmd.Output()
			if !strings.Contains(string(out), c.want) {
				t.Errorf("stdout %q doesn't say %q", out, c.want)
			}
			if err == nil {
				t.Error("exited 0, want the failure's status")
			}
		})
	}
}
