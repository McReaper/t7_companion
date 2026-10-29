package radiant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Machine is where Radiants run: the bridges listening there, a connection to
// one, and starting a new Radiant. Local is this computer.
type Machine interface {
	// Bridges are the pids of the Radiants whose bridge is listening.
	Bridges() ([]int, error)
	// Dial connects to the bridge of the Radiant with that pid.
	Dial(ctx context.Context, pid int) (io.ReadWriteCloser, error)
	// Start starts a program in dir and returns its pid.
	Start(exe, dir string, args ...string) (int, error)
	// Running reports whether the process with that pid still runs.
	Running(pid int) bool
	// Radiants are the pids of every Radiant running, its bridge listening or not.
	Radiants() ([]int, error)
	// Modules are the paths of the modules the process has loaded, its exe first.
	Modules(pid int) ([]string, error)
	// Ready waits until the process takes input.
	Ready(ctx context.Context, pid int) error
	// Control sends a command to MTPlugins' host in the process and returns its reply.
	Control(pid int, cmd string) (string, error)
	// WakeDisplay turns the screen on if it slept, for the display's idle
	// time: Windows composes no window while the screen is off, and a capture
	// then comes back blank.
	WakeDisplay()
}

// ErrNoRadiant is Connect's error when no Radiant runs the bridge.
var ErrNoRadiant = errors.New("no Radiant has t7kb's bridge loaded: radiant_open loads it into the Radiant running, or starts one on the map")

// Instance is one Radiant with the bridge, as List found it.
type Instance struct {
	PID   int
	State State
}

// Connect connects to the bridge of the Radiant with that pid, or with pid 0
// to the only one running.
func Connect(ctx context.Context, m Machine, pid int) (*Client, error) {
	if pid == 0 {
		pids, err := m.Bridges()
		if err != nil {
			return nil, err
		}
		switch len(pids) {
		case 0:
			return nil, ErrNoRadiant
		case 1:
			pid = pids[0]
		default:
			insts, err := List(ctx, m)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%d Radiants run the bridge (%s): pass the pid of one", len(insts), describe(insts))
		}
	}
	rw, err := m.Dial(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("no Radiant bridge with pid %d: %w", pid, err)
	}
	return NewClient(ctx, rw)
}

// List connects to every Radiant running the bridge and reads its state, in
// pid order. One that closes in between is left out.
func List(ctx context.Context, m Machine) ([]Instance, error) {
	pids, err := m.Bridges()
	if err != nil {
		return nil, err
	}
	sort.Ints(pids)
	var out []Instance
	for _, pid := range pids {
		c, err := Connect(ctx, m, pid)
		if err != nil {
			continue
		}
		st, err := c.State(ctx)
		c.Close()
		if err == nil {
			out = append(out, Instance{pid, st})
		}
	}
	return out, nil
}

// describe names each Radiant by its pid and map, for an error.
func describe(insts []Instance) string {
	var parts []string
	for _, in := range insts {
		m := in.State.Map
		if m == "" {
			m = "no map"
		}
		parts = append(parts, fmt.Sprintf("pid %d: %s", in.PID, m))
	}
	return strings.Join(parts, "; ")
}
