package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is a build t7kb runs, as the process running it writes it at each
// stage: see the package doc's "In the background".
type State struct {
	Tools   string    `json:"tools,omitempty"`
	Target  string    `json:"target"`
	Kind    string    `json:"kind"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
	Planned []string  `json:"planned"`           // the stages it runs, in order
	Running string    `json:"running,omitempty"` // the stage running now
	Done    bool      `json:"done"`
	Ended   time.Time `json:"ended,omitzero"`
	Report  Report    `json:"report"`         // the stages ended so far; the whole build once done
	Lost    bool      `json:"lost,omitempty"` // read only: its process ended before the build did
	Options Options   `json:"options,omitzero"`
}

// The state's folder, how a process is known to still run, and how the
// detached build starts; variables so tests can stand in.
var (
	stateDir = func() (string, error) {
		d, err := os.UserCacheDir()
		return filepath.Join(d, "t7kb", "builds"), err
	}
	processAlive = alive
)

// startGrace is how long a started build has to write its own pid: a state
// still without one after it is lost.
const startGrace = 30 * time.Second

// statePath is the state of a target of the install at tools: one file per
// install and target.
func statePath(tools string, mod bool, name string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	kind := "usermap"
	if mod {
		kind = "mod"
	}
	h := sha256.Sum256([]byte(installKey(tools)))
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%s.json", kind, strings.ToLower(name), hex.EncodeToString(h[:4]))), nil
}

// installKey is an install's root as Windows tells two spellings of it
// apart: by neither case, separator nor a separator at the end.
func installKey(tools string) string {
	return strings.ToLower(strings.TrimRight(strings.ReplaceAll(tools, `\`, "/"), "/"))
}

// readState reads a state; ok is false when there is none.
func readState(path string) (s State, ok bool, err error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, false, fmt.Errorf("%s: %w", path, err)
	}
	s.Lost = !s.Done && (s.PID == 0 && time.Since(s.Started) > startGrace || s.PID != 0 && !processAlive(s.PID))
	return s, true, nil
}

// writeState replaces the state whole, so a reader never sees half of it; a
// reader holding the file open makes the rename fail on Windows, so it tries
// again.
func writeState(path string, s State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	for i := 0; ; i++ {
		err = os.Rename(tmp, path)
		if err == nil || i == 20 {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// claim checks no other build of the target runs, and writes its state as
// starting, by pid: 0 for a build whose process is still to start.
func claim(p *buildPlan, pid int) (string, State, error) {
	path, err := statePath(p.tools, p.o.Mod, p.name)
	if err != nil {
		return "", State{}, err
	}
	old, ok, err := readState(path)
	if err != nil {
		return "", State{}, err
	}
	if ok && !old.Done && !old.Lost {
		return "", State{}, fmt.Errorf("a build of %s runs already (pid %d, started %s, at %s): wait for it, or read it with status",
			p.name, old.PID, old.Started.Local().Format("15:04"), old.Running)
	}
	s := State{Tools: p.tools, Target: p.name, Kind: p.kind(), PID: pid, Started: time.Now(), Planned: p.planned(), Options: *p.o}
	s.Report = Report{Target: p.name, Kind: p.kind(), OK: true}
	return path, s, writeState(path, s)
}

// Start starts the build in a process of its own, which spawn starts with the
// state's path, and returns at once: the state says how it goes. The state
// holds the target, as starting, until the spawned process writes its own pid;
// this one writes it no more, as the build may have ended already.
func Start(o *Options, name string, spawn func(state string) (int, error)) (State, error) {
	p, err := newBuildPlan(o, name, io.Discard)
	if err != nil {
		return State{}, err
	}
	path, s, err := claim(p, 0)
	if err != nil {
		return State{}, err
	}
	if s.PID, err = spawn(path); err != nil {
		_ = os.Remove(path)
		return State{}, err
	}
	return s, nil
}

// RunTracked runs the build here, as Run does, keeping its state for status
// and the watch.
func RunTracked(o *Options, name string, stdout io.Writer) (Report, error) {
	p, err := newBuildPlan(o, name, stdout)
	if err != nil {
		return Report{}, err
	}
	path, s, err := claim(p, os.Getpid())
	if err != nil {
		return Report{}, err
	}
	return p.run(&tracker{path: path, s: s}), nil
}

// Resume runs the build a Start left in its state, in the process Start
// spawned.
func Resume(path string, stdout io.Writer) (Report, error) {
	s, ok, err := readState(path)
	if err != nil || !ok {
		return Report{}, fmt.Errorf("no build state at %s: %v", path, err)
	}
	o := s.Options
	p, err := newBuildPlan(&o, s.Target, stdout)
	if err != nil {
		s.Done, s.Ended = true, time.Now()
		s.Report = Report{Target: s.Target, Kind: s.Kind, FailedStage: "start", Stages: []StageResult{{Name: "start", Errors: []string{err.Error()}}}}
		_ = writeState(path, s)
		return Report{}, err
	}
	s.PID = os.Getpid()
	return p.run(&tracker{path: path, s: s}), nil
}

// Status reads the state of the target's last build t7kb ran.
func Status(o *Options, name string) (State, error) {
	tools := strings.TrimRight(firstNonEmpty(o.ToolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
	if tools == "" {
		return State{}, fmt.Errorf("no mod-tools path: pass --tools-path or set TA_TOOLS_PATH")
	}
	path, err := statePath(tools, o.Mod, name)
	if err != nil {
		return State{}, err
	}
	s, ok, err := readState(path)
	if err == nil && !ok {
		err = fmt.Errorf("t7kb has run no build of %s", name)
	}
	return s, err
}

// States are the states of every build t7kb ran on the install at tools.
func States(tools string) []State {
	dir, err := stateDir()
	if err != nil {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var out []State
	for _, p := range paths {
		if s, ok, err := readState(p); ok && err == nil && installKey(s.Tools) == installKey(tools) {
			out = append(out, s)
		}
	}
	return out
}

// tracker writes a build's state as it goes; a nil one writes nothing.
type tracker struct {
	path string
	s    State
}

func (t *tracker) running(stage string) {
	if t == nil {
		return
	}
	t.s.Running = stage
	_ = writeState(t.path, t.s)
}

func (t *tracker) ended(rep Report, done bool) {
	if t == nil {
		return
	}
	t.s.Report, t.s.Running, t.s.Done = rep, "", done
	if done {
		t.s.Ended = time.Now()
	}
	_ = writeState(t.path, t.s)
}
