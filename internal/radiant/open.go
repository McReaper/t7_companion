package radiant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OpenOptions say which map to open, and in which Radiant.
type OpenOptions struct {
	Root    string // the mod-tools root, where bin/radiant_modtools.exe is
	Map     string // the .map file
	PID     int    // the Radiant to use when several run the bridge, 0 for the only one
	Discard bool   // reload over unsaved changes
}

// How a map came to be open.
const (
	AlreadyOpen = "already open"
	Reloaded    = "reloaded"
	Launched    = "launched"
)

// Opened is the Radiant that has the map open, and how it came to.
type Opened struct {
	PID    int
	Map    string // as Radiant holds it
	How    string // AlreadyOpen, Reloaded or Launched
	Plugin string // the bridge plugin's version
}

// pollEvery is how often Open looks for a bridge or a loaded map.
var pollEvery = 250 * time.Millisecond

// Open makes a Radiant show the map, see the package doc. It waits as long as
// ctx allows: starting Radiant takes tens of seconds.
func Open(ctx context.Context, m Machine, o OpenOptions) (Opened, error) {
	if _, err := os.Stat(o.Map); err != nil {
		return Opened{}, fmt.Errorf("no map at %s", o.Map)
	}
	insts, err := List(ctx, m)
	if err != nil {
		return Opened{}, err
	}
	for _, in := range insts {
		if SamePath(in.State.Map, o.Map) && (o.PID == 0 || o.PID == in.PID) {
			return opened(ctx, m, in.PID, AlreadyOpen)
		}
	}
	target, err := pick(insts, o.PID)
	if err != nil {
		return Opened{}, err
	}
	if target == nil {
		return launch(ctx, m, o)
	}
	return reload(ctx, m, *target, o)
}

// pick is the Radiant to reload the map in, nil when none runs.
func pick(insts []Instance, pid int) (*Instance, error) {
	if pid != 0 {
		for i := range insts {
			if insts[i].PID == pid {
				return &insts[i], nil
			}
		}
		return nil, fmt.Errorf("no Radiant with pid %d runs the bridge (%s)", pid, describe(insts))
	}
	switch len(insts) {
	case 0:
		return nil, nil
	case 1:
		return &insts[0], nil
	}
	return nil, fmt.Errorf("%d Radiants run the bridge (%s): pass the pid of the one to open the map in", len(insts), describe(insts))
}

// reload opens the map in a running Radiant, unless that would lose its work
// or it can't.
func reload(ctx context.Context, m Machine, in Instance, o OpenOptions) (Opened, error) {
	st := in.State
	switch {
	case st.Dialog != "":
		return Opened{}, fmt.Errorf("the Radiant of pid %d has the dialog %q open: close it first", in.PID, st.Dialog)
	case st.PrefabDepth > 0:
		return Opened{}, fmt.Errorf("the Radiant of pid %d is editing inside a prefab: leave it first", in.PID)
	case st.Modified && !o.Discard:
		return Opened{}, fmt.Errorf("the Radiant of pid %d has unsaved changes to %s: save them, or pass discard to drop them", in.PID, st.Map)
	}
	c, err := Connect(ctx, m, in.PID)
	if err != nil {
		return Opened{}, err
	}
	defer c.Close()
	if _, err := c.Call(ctx, "reload "+o.Map, nil); err != nil {
		return Opened{}, err
	}
	if err := waitMap(ctx, c, o.Map); err != nil {
		return Opened{}, err
	}
	return opened(ctx, m, in.PID, Reloaded)
}

// launch starts Radiant on the map and waits for its bridge, then the map.
func launch(ctx context.Context, m Machine, o OpenOptions) (Opened, error) {
	bin := filepath.Join(o.Root, "bin")
	exe := filepath.Join(bin, "radiant_modtools.exe")
	if _, err := os.Stat(exe); err != nil {
		return Opened{}, fmt.Errorf("no Radiant at %s: is %s a BO3 mod-tools root?", exe, o.Root)
	}
	plugin := filepath.Join(bin, "plugins", "T7RadiantBridge", "T7RadiantBridge.dll")
	if _, err := os.Stat(plugin); err != nil {
		return Opened{}, fmt.Errorf("the T7RadiantBridge plugin is not installed (%s): t7kb drives Radiant through it; "+
			"install MTPlugins, then unzip T7RadiantBridge.zip from %s into bin/plugins/", plugin, Releases)
	}
	pid, err := m.Start(exe, bin, o.Map)
	if err != nil {
		return Opened{}, fmt.Errorf("could not start Radiant: %w", err)
	}
	c, err := waitBridge(ctx, m, pid, filepath.Join(bin, "plugins", "RadiantPlugins.log"))
	if err != nil {
		return Opened{}, err
	}
	defer c.Close()
	if err := waitMap(ctx, c, o.Map); err != nil {
		return Opened{}, err
	}
	return opened(ctx, m, pid, Launched)
}

// waitBridge waits for a starting Radiant's bridge to answer.
func waitBridge(ctx context.Context, m Machine, pid int, log string) (*Client, error) {
	for {
		if !m.Running(pid) {
			return nil, fmt.Errorf("the Radiant started (pid %d) exited before its bridge answered: see %s", pid, log)
		}
		if rw, err := m.Dial(ctx, pid); err == nil {
			return NewClient(ctx, rw)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the Radiant started (pid %d) runs, but its bridge never answered: does MTPlugins load it? see %s", pid, log)
		case <-time.After(pollEvery):
		}
	}
}

// waitMap waits until Radiant has the map open, with no dialog left over the load.
func waitMap(ctx context.Context, c *Client, mapPath string) error {
	var last State
	for {
		st, err := c.State(ctx)
		switch {
		case err == nil:
			if SamePath(st.Map, mapPath) && st.Dialog == "" {
				return nil
			}
			last = st
		case !errors.Is(err, context.DeadlineExceeded):
			return err
		}
		select {
		case <-ctx.Done():
			if last.Dialog != "" {
				return fmt.Errorf("the dialog %q is open over the load of %s: answer it in Radiant", last.Dialog, mapPath)
			}
			return fmt.Errorf("the map Radiant has open is still %q, not %s", last.Map, mapPath)
		case <-time.After(pollEvery):
		}
	}
}

func opened(ctx context.Context, m Machine, pid int, how string) (Opened, error) {
	c, err := Connect(ctx, m, pid)
	if err != nil {
		return Opened{}, err
	}
	defer c.Close()
	st, err := c.State(ctx)
	if err != nil {
		return Opened{}, err
	}
	return Opened{PID: pid, Map: st.Map, How: how, Plugin: c.Hello.Plugin}, nil
}
