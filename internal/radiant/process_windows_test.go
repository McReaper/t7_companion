package radiant

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A child process to look at: the test binary itself, waiting.
func TestMain(m *testing.M) {
	if os.Getenv("T7KB_RADIANT_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Modules lists another process's modules, its exe first, once its loader
// settles: the bridge's loading is read from them.
func TestLocalSeesAnotherProcess(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "T7KB_RADIANT_CHILD=1")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	pid := c.Process.Pid
	// A process just started may list no module yet, or not yet kernel32.
	var mods []string
	var err error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		mods, err = Local.Modules(pid)
		if err == nil && slices.ContainsFunc(mods, func(m string) bool { return strings.EqualFold(filepath.Base(m), "kernel32.dll") }) {
			break
		}
	}
	if err != nil || len(mods) == 0 || !strings.EqualFold(filepath.Base(mods[0]), filepath.Base(os.Args[0])) {
		t.Fatalf("the child's modules, its exe first: %v %v", mods, err)
	}
	if !slices.ContainsFunc(mods, func(m string) bool { return strings.EqualFold(filepath.Base(m), "kernel32.dll") }) {
		t.Errorf("the child lists no kernel32.dll: %v", mods)
	}
	if _, err := Local.Modules(1 << 30); err == nil {
		t.Error("no such process")
	}
	if _, err := Local.Control(pid, "plugins list"); err == nil {
		t.Error("a process without MTPlugins has no control window")
	}
	if _, err := Local.Radiants(); err != nil {
		t.Error(err)
	}
}
