package radianttest

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// Machine is a computer of fakes, a radiant.Machine.
type Machine struct {
	// OnStart makes the Radiant a Start call starts, given its pid; nil (or a
	// nil Fake) is a Radiant that exits at once.
	OnStart func(pid int, exe, dir string, args []string) *Started

	mu      sync.Mutex
	fakes   map[int]*Started
	started [][]string
	nextPID int
}

// Started is a Radiant running on a Machine: its Fake, served once its bridge
// listens.
type Started struct {
	Fake *Fake
	// ListenAfter is how long after Start its bridge listens; Silent is a
	// Radiant whose bridge never does (MTPlugins didn't load the plugin).
	ListenAfter time.Duration
	Silent      bool
	at          time.Time
}

// NewMachine is a computer running these Radiants, their bridges listening.
func NewMachine(fakes ...*Fake) *Machine {
	m := &Machine{fakes: map[int]*Started{}, nextPID: 9000}
	for _, f := range fakes {
		m.fakes[f.PID] = &Started{Fake: f}
	}
	return m
}

// Starts are the Start calls made: the program, then its arguments.
func (m *Machine) Starts() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]string(nil), m.started...)
}

func (m *Machine) listening(pid int) *Fake {
	s := m.fakes[pid]
	if s == nil || s.Silent || time.Now().Before(s.at.Add(s.ListenAfter)) {
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
