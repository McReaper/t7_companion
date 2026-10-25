package build

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// stateFixture puts the build states in a folder of the test's, says which
// pids run, and lays out an install whose stages the fake pipeline runs.
func stateFixture(t *testing.T, running map[int]bool) (root string, f *fakePipeline) {
	t.Helper()
	t.Setenv("TA_TOOLS_PATH", "")
	t.Setenv("TA_GAME_PATH", "")
	dir := t.TempDir()
	prevDir, prevAlive := stateDir, processAlive
	t.Cleanup(func() { stateDir, processAlive = prevDir, prevAlive })
	stateDir = func() (string, error) { return dir, nil }
	processAlive = func(pid int) bool { return running[pid] || pid == os.Getpid() }
	root = fakeTools(t)
	f = &fakePipeline{root: root}
	f.install(t)
	return root, f
}

func TestStartClaimsTheTargetAndSpawnsItsBuild(t *testing.T) {
	root, _ := stateFixture(t, map[int]bool{4242: true})
	o := &Options{ToolsPath: root, Stages: "compile,link", Light: "medium"}
	var spawned string
	s, err := Start(o, "zm_test", func(state string) (int, error) { spawned = state; return 4242, nil })
	if err != nil {
		t.Fatal(err)
	}
	if s.PID != 4242 || !reflect.DeepEqual(s.Planned, []string{"gdt", "compile", "link"}) || s.Kind != "usermap" {
		t.Errorf("the started build: %+v", s)
	}
	onDisk, ok, err := readState(spawned)
	if err != nil || !ok || onDisk.PID != 0 || onDisk.Lost || onDisk.Options.Stages != "compile,link" || onDisk.Done {
		t.Errorf("the state holds the target as starting until the build writes its pid, with the options it runs: %+v %v %v", onDisk, ok, err)
	}
	if _, err := Start(o, "zm_test", func(string) (int, error) { t.Error("a second build spawned"); return 0, nil }); err == nil || !strings.Contains(err.Error(), "runs already") {
		t.Errorf("one build of a target at a time: %v", err)
	}
	if _, err := RunTracked(o, "zm_test", io.Discard); err == nil || !strings.Contains(err.Error(), "runs already") {
		t.Errorf("a waited build waits its turn too: %v", err)
	}
	if _, err := Start(&Options{ToolsPath: root, Stages: "bake"}, "zm_test", nil); err == nil {
		t.Error("a bad stage is refused before anything starts")
	}
	if _, err := Start(&Options{ToolsPath: root, Stages: "link", Mod: true}, "zm_test", func(string) (int, error) { return 0, os.ErrPermission }); err == nil {
		t.Error("a spawn that fails is the call's error")
	}
	if _, err := Status(&Options{ToolsPath: root, Mod: true}, "zm_test"); err == nil {
		t.Error("the failed spawn leaves no state")
	}
}

func TestResumeRunsTheBuildAndKeepsItsState(t *testing.T) {
	root, f := stateFixture(t, nil)
	o := &Options{ToolsPath: root, Stages: "compile,link", Light: "medium"}
	var path string
	if _, err := Start(o, "zm_test", func(state string) (int, error) { path = state; return 99, nil }); err != nil {
		t.Fatal(err)
	}
	var mid []string // the stage the state said was running as each stage ran
	prev := stageRunner
	stageRunner = func(label, dir, exe string, timeout time.Duration, v bool, out io.Writer, args ...string) StageResult {
		s, _, _ := readState(path)
		mid = append(mid, s.Running)
		return prev(label, dir, exe, timeout, v, out, args...)
	}
	f.fail = "link"
	rep, err := Resume(path, io.Discard)
	if err != nil || rep.OK || rep.FailedStage != "link" {
		t.Fatalf("the resumed build's report: %+v %v", rep, err)
	}
	if !reflect.DeepEqual(mid, []string{"gdt", "compile", "link"}) {
		t.Errorf("the state names each stage while it runs: %q", mid)
	}
	s, err := Status(o, "zm_test")
	if err != nil || !s.Done || s.Lost || s.PID != os.Getpid() || s.Running != "" || s.Report.FailedStage != "link" || len(s.Report.Stages) != 3 || s.Ended.IsZero() {
		t.Errorf("status reads the ended build: %+v %v", s, err)
	}
	if got := States(root); len(got) != 1 || States(t.TempDir()) != nil {
		t.Errorf("an install's states, and only its: %d", len(got))
	}
	if _, err := Resume(filepath.Join(t.TempDir(), "none.json"), io.Discard); err == nil {
		t.Error("no state to resume")
	}
}

func TestResumeOfABuildThatCantStartEndsIt(t *testing.T) {
	root, _ := stateFixture(t, nil)
	o := &Options{ToolsPath: root, Stages: "link", Light: "medium"}
	var path string
	if _, err := Start(o, "zm_test", func(state string) (int, error) { path = state; return 99, nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "bin", "linker_modtools.exe")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resume(path, io.Discard); err == nil {
		t.Fatal("the mod tools went")
	}
	s, _, _ := readState(path)
	if !s.Done || s.Report.FailedStage != "start" || len(s.Report.Stages) != 1 || !strings.Contains(s.Report.Stages[0].Errors[0], "mod tools not found") {
		t.Errorf("the state says why it never started: %+v", s)
	}
}

func TestRunTrackedAndALostBuild(t *testing.T) {
	root, _ := stateFixture(t, nil)
	o := &Options{ToolsPath: root, Stages: "link", Light: "medium", SkipGDT: true}
	rep, err := RunTracked(o, "zm_test", io.Discard)
	if err != nil || !rep.OK {
		t.Fatalf("%+v %v", rep, err)
	}
	if s, err := Status(o, "zm_test"); err != nil || !s.Done || !reflect.DeepEqual(s.Planned, []string{"link"}) {
		t.Errorf("a waited build keeps its state too: %+v %v", s, err)
	}
	// A build whose process died at the light stage: lost, and its target free.
	path, _ := statePath(root, false, "zm_lost")
	if err := writeState(path, State{Tools: root, Target: "zm_lost", Kind: "usermap", PID: 31337, Running: "light"}); err != nil {
		t.Fatal(err)
	}
	if s, err := Status(&Options{ToolsPath: root}, "zm_lost"); err != nil || !s.Lost {
		t.Errorf("a build whose process ended is lost: %+v %v", s, err)
	}
	if _, err := RunTracked(&Options{ToolsPath: root, Stages: "link", SkipGDT: true, Light: "medium"}, "zm_lost", io.Discard); err != nil {
		t.Errorf("a lost build holds its target no more: %v", err)
	}
	// A build whose process never wrote its pid.
	path, _ = statePath(root, false, "zm_unborn")
	if err := writeState(path, State{Tools: root, Target: "zm_unborn", Kind: "usermap", Started: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if s, err := Status(&Options{ToolsPath: root}, "zm_unborn"); err != nil || !s.Lost {
		t.Errorf("a build that never started is lost after its grace: %+v %v", s, err)
	}
}

func TestStatusErrors(t *testing.T) {
	root, _ := stateFixture(t, nil)
	if _, err := Status(&Options{}, "zm_test"); err == nil || !strings.Contains(err.Error(), "no mod-tools path") {
		t.Errorf("no tools path: %v", err)
	}
	if _, err := Status(&Options{ToolsPath: root}, "zm_never"); err == nil || !strings.Contains(err.Error(), "no build of zm_never") {
		t.Errorf("no build yet: %v", err)
	}
	path, _ := statePath(root, false, "zm_bad")
	writeFile(t, path, "{not json")
	if _, err := Status(&Options{ToolsPath: root}, "zm_bad"); err == nil {
		t.Error("a state that isn't JSON")
	}
	stateDir = func() (string, error) { return "", os.ErrNotExist }
	if _, err := Status(&Options{ToolsPath: root}, "zm_test"); err == nil {
		t.Error("no cache folder")
	}
	if States(root) != nil {
		t.Error("no states without a cache folder")
	}
}

func TestStatePathIsPerInstallAndTarget(t *testing.T) {
	stateFixture(t, nil)
	a, _ := statePath(`C:\bo3`, false, "zm_x")
	b, _ := statePath(`C:\BO3\`, false, "ZM_X")
	c, _ := statePath(`D:\bo3`, false, "zm_x")
	d, _ := statePath(`C:\bo3`, true, "zm_x")
	if a != b || a == c || a == d || !strings.HasPrefix(filepath.Base(d), "mod-zm_x-") {
		t.Errorf("one file per install and target, whatever the case: %s %s %s %s", a, b, c, d)
	}
}

func TestAliveTellsARunningProcessFromAnEndedOne(t *testing.T) {
	c := exec.Command(os.Args[0], "-test.run=^$") // this test binary, running no test
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	if !alive(os.Getpid()) || alive(0) || alive(c.Process.Pid) {
		t.Errorf("this process runs, pid 0 is none, an ended one runs no more")
	}
}
