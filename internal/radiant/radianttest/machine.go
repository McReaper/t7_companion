package radianttest

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Machine is a computer of fakes, a radiant.Machine.
type Machine struct {
	// OnStart makes the Radiant a Start call starts, given its pid; nil (or a
	// nil Fake) is a Radiant that exits at once.
	OnStart func(pid int, exe, dir string, args []string) *Started

	mu       sync.Mutex
	fakes    map[int]*Started
	started  [][]string
	injected [][]string // pid, dll
	controls [][]string // pid, command
	nextPID  int
}

// Started is a Radiant running on a Machine: its Fake, served once its bridge
// listens.
type Started struct {
	Fake *Fake
	// ListenAfter is how long after Start its bridge listens; Silent is a
	// Radiant whose bridge never does (MTPlugins didn't load the plugin).
	ListenAfter time.Duration
	Silent      bool
	// Modules are the modules it reports, its exe first. Unloaded is a
	// Radiant whose bridge listens only once something loads it: an Inject, or
	// MTPlugins' host told to enable the plugin (Control).
	Modules  []string
	Unloaded bool
	// ControlAfter is how long after Start MTPlugins' host opens its control
	// window: it is loaded (listed in Modules) well before.
	ControlAfter time.Duration
	at           time.Time
	loaded       bool
}

// NewMachine is a computer running these Radiants, their bridges listening.
func NewMachine(fakes ...*Fake) *Machine {
	m := &Machine{fakes: map[int]*Started{}, nextPID: 9000}
	for _, f := range fakes {
		m.fakes[f.PID] = &Started{Fake: f}
	}
	return m
}

// Run adds a Radiant that runs already, as a Start would have made it.
func (m *Machine) Run(s *Started) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.at = time.Now()
	m.fakes[s.Fake.PID] = s
}

// Exit ends the Radiant of that pid: its bridge's connections close, and it
// runs no more.
func (m *Machine) Exit(pid int) {
	m.mu.Lock()
	s := m.fakes[pid]
	delete(m.fakes, pid)
	m.mu.Unlock()
	if s != nil {
		s.Fake.quit()
	}
}

// Starts are the Start calls made: the program, then its arguments.
func (m *Machine) Starts() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]string(nil), m.started...)
}

func (m *Machine) listening(pid int) *Fake {
	s := m.fakes[pid]
	if s == nil || s.Silent || (s.Unloaded && !s.loaded) || time.Now().Before(s.at.Add(s.ListenAfter)) {
		return nil
	}
	return s.Fake
}

// Bridges are the pids whose bridge listens.
func (m *Machine) Bridges() ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pids []int
	for pid := range m.fakes {
		if m.listening(pid) != nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// Dial connects to a listening bridge.
func (m *Machine) Dial(_ context.Context, pid int) (io.ReadWriteCloser, error) {
	m.mu.Lock()
	f := m.listening(pid)
	m.mu.Unlock()
	if f == nil {
		return nil, fmt.Errorf("no pipe t7_radiant_bridge_%d", pid)
	}
	return f.Dial(), nil
}

// Start records the call and starts what OnStart makes.
func (m *Machine) Start(exe, dir string, args ...string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextPID++
	pid := m.nextPID
	m.started = append(m.started, append([]string{exe, dir}, args...))
	if m.OnStart != nil {
		if s := m.OnStart(pid, exe, dir, args); s != nil && s.Fake != nil {
			s.Fake.PID, s.at = pid, time.Now()
			m.fakes[pid] = s
		}
	}
	return pid, nil
}

// Running reports whether the pid is one of the machine's Radiants.
func (m *Machine) Running(pid int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fakes[pid] != nil
}

// Radiants are the pids of every Radiant on the machine, listening or not.
func (m *Machine) Radiants() ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pids []int
	for pid := range m.fakes {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids, nil
}

// Modules are the modules the Radiant reports.
func (m *Machine) Modules(pid int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.fakes[pid]
	if s == nil {
		return nil, fmt.Errorf("no process %d", pid)
	}
	return append([]string(nil), s.Modules...), nil
}

// Ready is at once for a running Radiant.
func (m *Machine) Ready(_ context.Context, pid int) error {
	if !m.Running(pid) {
		return fmt.Errorf("no process %d", pid)
	}
	return nil
}

// Inject records the call; the Radiant's bridge then listens, and it lists the DLL.
func (m *Machine) Inject(pid int, dll string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.injected = append(m.injected, []string{strconv.Itoa(pid), dll})
	s := m.fakes[pid]
	if s == nil {
		return fmt.Errorf("no process %d", pid)
	}
	s.loaded, s.Modules = true, append(s.Modules, dll)
	return nil
}

// Injected are the Inject calls made: the pid, then the DLL.
func (m *Machine) Injected() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]string(nil), m.injected...)
}

// Control answers as MTPlugins' host does, in a Radiant that lists
// RadiantPlugins.dll once its control window is open (ControlAfter): "plugins
// enable <name>" loads the plugin, so the bridge listens, "plugins disable
// <name>" unloads it.
func (m *Machine) Control(pid int, cmd string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.fakes[pid]
	if s == nil || !slices.ContainsFunc(s.Modules, func(p string) bool { return strings.EqualFold(filepath.Base(p), "RadiantPlugins.dll") }) ||
		time.Now().Before(s.at.Add(s.ControlAfter)) {
		return "", fmt.Errorf("MTPlugins' control window isn't in that Radiant")
	}
	m.controls = append(m.controls, []string{strconv.Itoa(pid), cmd})
	switch {
	case cmd == "plugins list":
		return "no plugins", nil
	case strings.HasPrefix(cmd, "plugins enable "):
		s.loaded = true
		return strings.TrimPrefix(cmd, "plugins enable ") + " Loaded", nil
	case strings.HasPrefix(cmd, "plugins disable "):
		s.loaded = false
		return strings.TrimPrefix(cmd, "plugins disable ") + " Disabled", nil
	case cmd == "plugins rescan":
		return "rescanned", nil
	}
	return "unknown command", nil
}

// Controls are the commands MTPlugins' host received: the pid, then the command.
func (m *Machine) Controls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]string(nil), m.controls...)
}
