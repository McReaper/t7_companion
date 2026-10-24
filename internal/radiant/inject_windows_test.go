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

// A child process to load a DLL into: the test binary itself, waiting.
func TestMain(m *testing.M) {
	if os.Getenv("T7KB_INJECT_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLocalInjectLoadsADLLIntoAnotherProcess(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^$")
	c.Env = append(os.Environ(), "T7KB_INJECT_CHILD=1")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	pid := c.Process.Pid
	dll := filepath.Join(os.Getenv("SystemRoot"), "System32", "msimg32.dll") // a small DLL a Go program doesn't load
	before, err := Local.Modules(pid)
	if err != nil || len(before) == 0 || !strings.HasSuffix(strings.ToLower(before[0]), ".exe") {
		t.Fatalf("the child's modules, its exe first: %v %v", before, err)
	}
	if slices.ContainsFunc(before, func(m string) bool { return strings.EqualFold(m, dll) }) {
		t.Skip("the child loaded msimg32.dll already")
	}
	if err := Local.Inject(pid, dll); err != nil {
		t.Fatal(err)
	}
	after, err := Local.Modules(pid)
	if err != nil || !slices.ContainsFunc(after, func(m string) bool { return strings.EqualFold(m, dll) }) {
		t.Errorf("after the inject, the child lists %s: %v", dll, err)
	}
	if err := Local.Inject(pid, `C:\nowhere\none.dll`); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("a DLL LoadLibraryW can't load: %v", err)
	}
	if err := Local.Inject(1<<30, dll); err == nil {
		t.Error("no such process")
	}
	if _, err := Local.Control(pid, "plugins list"); err == nil {
		t.Error("a process without MTPlugins has no control window")
	}
	if _, err := Local.Radiants(); err != nil {
		t.Error(err)
	}
}
