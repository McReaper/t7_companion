package cli

import (
	"io"
	"os"
	"os/exec"

	"github.com/McReaper/t7_companion/internal/build"
)

// The build's entry points; tests swap them to see what each surface asks for.
var (
	buildRun    = build.RunTracked
	buildStart  = func(o *build.Options, name string) (build.State, error) { return build.Start(o, name, spawnBuild) }
	buildStatus = build.Status
	buildResume = build.Resume
)

// buildStarted is build's answer when it starts the build in the background.
type buildStarted struct {
	Started string   `json:"started"` // the target and its kind
	PID     int      `json:"pid"`
	Stages  []string `json:"stages"`
	Next    string   `json:"next"`
}

// buildStartedNext says where the build's verdict comes from.
const buildStartedNext = "the build runs on its own: the t7kb plugin's monitor says each link and compile verdict and when the build ends; " +
	"build with status reads how far it got, and its report once done"

// buildOp is build's one operation, for both surfaces: by default it starts
// the build in the background and answers at once; with wait it runs it here
// and answers its report; with status it reads the state of the target's last
// build.
func buildOp(o *build.Options, name string, wait, status bool, stdout io.Writer) (any, error) {
	switch {
	case status:
		s, err := buildStatus(o, name)
		s.Tools, s.Options = "", build.Options{}
		return s, err
	case wait:
		return buildRun(o, name, stdout)
	}
	s, err := buildStart(o, name)
	if err != nil {
		return nil, err
	}
	return buildStarted{Started: s.Target + " (" + s.Kind + ")", PID: s.PID, Stages: s.Planned, Next: buildStartedNext}, nil
}

// spawnBuild starts this binary on the build a state holds, detached, so it
// outlives the MCP server or the terminal that started it.
func spawnBuild(state string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	c := exec.Command(exe, "build", "--resume", state)
	build.Detach(c)
	if err := c.Start(); err != nil {
		return 0, err
	}
	pid := c.Process.Pid
	_ = c.Process.Release()
	return pid, nil
}
