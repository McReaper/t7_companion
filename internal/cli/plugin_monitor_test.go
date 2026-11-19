package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The plugin's monitor starts with every Claude Code session, and the mod
// tools set TA_TOOLS_PATH and TA_GAME_PATH for the whole machine, so its script
// runs `t7kb watch` only for a session working inside one of those roots,
// compared as Windows compares paths.
func TestMonitorWatchesOnlyInsideTheGameRoot(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on the PATH")
	}
	script, err := filepath.Abs("../../plugin/monitors/watch.sh")
	if err != nil {
		t.Fatal(err)
	}
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
	stub := filepath.Join(base, "t7kb")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho \"t7kb $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, dir, game, tools string
		watch                  bool
	}{
		{"in the root", root, "", root, true},
		{"in a map's folder", mapDir, "", root, true},
		{"in the root spelled with another case and a trailing backslash", mapDir, "", strings.ToUpper(root) + `\`, true},
		{"in the game root alone", mapDir, root, "", true},
		{"in the game root with the tools elsewhere", mapDir, root, elsewhere, true},
		{"in a sibling sharing the root's prefix", sibling, root, root, false},
		{"in another folder", elsewhere, root, root, false},
		{"with no root set", root, "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(sh, script)
			cmd.Dir = c.dir
			cmd.Env = append(os.Environ(), "TA_GAME_PATH="+c.game, "TA_TOOLS_PATH="+c.tools, "T7KB_BIN="+stub)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("watch.sh: %v", err)
			}
			if got := strings.TrimSpace(string(out)) == "t7kb watch"; got != c.watch {
				t.Errorf("ran t7kb watch = %v, want %v (output %q)", got, c.watch, out)
			}
		})
	}
}
