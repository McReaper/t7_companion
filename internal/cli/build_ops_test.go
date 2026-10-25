package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/build"
)

// standInBuild replaces the build's entry points: each records what it was
// asked, and answers as a build of zm_x would.
func standInBuild(t *testing.T) *[]string {
	t.Helper()
	oldRun, oldStart, oldStatus, oldResume := buildRun, buildStart, buildStatus, buildResume
	t.Cleanup(func() { buildRun, buildStart, buildStatus, buildResume = oldRun, oldStart, oldStatus, oldResume })
	var calls []string
	buildRun = func(_ *build.Options, name string, _ io.Writer) (build.Report, error) {
		calls = append(calls, "wait "+name)
		return build.Report{Target: name, Kind: "usermap", OK: true, Stages: []build.StageResult{{Name: "link", OK: true}}}, nil
	}
	buildStart = func(_ *build.Options, name string) (build.State, error) {
		calls = append(calls, "start "+name)
		return build.State{Target: name, Kind: "usermap", PID: 42, Planned: []string{"gdt", "link"}}, nil
	}
	buildStatus = func(_ *build.Options, name string) (build.State, error) {
		calls = append(calls, "status "+name)
		return build.State{Tools: "C:/bo3", Target: name, Kind: "usermap", PID: 42, Started: time.Unix(0, 0), Running: "link",
			Options: build.Options{Stages: "link"}}, nil
	}
	buildResume = func(path string, _ io.Writer) (build.Report, error) {
		calls = append(calls, "resume "+path)
		return build.Report{}, nil
	}
	return &calls
}

func TestBuildStartsWaitsOrReadsItsStatusOnBothSurfaces(t *testing.T) {
	calls := standInBuild(t)
	run := func(args ...string) string {
		t.Helper()
		root := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(append([]string{"build"}, args...))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}
	if out := run("zm_x"); !strings.Contains(out, `"started": "zm_x (usermap)"`) || !strings.Contains(out, `"pid": 42`) || !strings.Contains(out, "monitor") {
		t.Errorf("the default starts the build and says where its verdict comes from: %s", out)
	}
	if out := run("zm_x", "--wait"); !strings.Contains(out, "build OK") {
		t.Errorf("--wait prints the report: %s", out)
	}
	out := run("zm_x", "--status")
	if !strings.Contains(out, `"running": "link"`) || strings.Contains(out, "C:/bo3") || strings.Contains(out, "options") {
		t.Errorf("--status reads the state, without its install or options: %s", out)
	}
	run("--resume", "state.json")
	for _, args := range []map[string]any{{"name": "zm_x"}, {"name": "zm_x", "wait": true}, {"name": "zm_x", "status": true}} {
		res := callBuildTool(t, args)
		var v map[string]any
		if res.IsError || json.Unmarshal([]byte(resultText(res)), &v) != nil {
			t.Errorf("%v: %s", args, resultText(res))
		}
	}
	want := "start zm_x|wait zm_x|status zm_x|resume state.json|start zm_x|wait zm_x|status zm_x"
	if got := strings.Join(*calls, "|"); got != want {
		t.Errorf("each surface asks for the same:\n got %s\nwant %s", got, want)
	}
	root := newRootCmd()
	root.SetOut(io.Discard)
	root.SetArgs([]string{"build"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "needs the map's or mod's name") {
		t.Errorf("no name: %v", err)
	}
}

func TestBuildStartErrorsAreTheToolsErrors(t *testing.T) {
	standInBuild(t)
	buildStart = func(*build.Options, string) (build.State, error) {
		return build.State{}, errString("a build of zm_x runs already")
	}
	if res := callBuildTool(t, map[string]any{"name": "zm_x"}); !res.IsError || !strings.Contains(resultText(res), "runs already") {
		t.Errorf("a refused start is a tool error: %s", resultText(res))
	}
}

// spawnedChild marks a process a test spawned as t7kb: TestMain ends it at
// once, as the test binary would otherwise run every test again, spawning in
// turn.
const spawnedChild = "T7KB_TEST_SPAWNED_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(spawnedChild) == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The detached build is this binary again, on the state: here the test
// binary, which TestMain ends at once.
func TestSpawnBuildStartsADetachedProcess(t *testing.T) {
	t.Setenv(spawnedChild, "1")
	pid, err := spawnBuild("no-state.json")
	if err != nil || pid <= 0 {
		t.Errorf("spawned %d: %v", pid, err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
