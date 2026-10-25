package radiant

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BundledBridge is the bridge release t7kb ships beside its binary: the
// release packaging fetches it.
const BundledBridge = "1.2.2"

const pluginName = "T7RadiantBridge"

// StandaloneDLL is the bridge with its own host, which t7kb injects into a
// Radiant MTPlugins doesn't load into. Its name carries its version: a running
// Radiant locks the one it loaded, and an update installs the next beside it.
func StandaloneDLL(dir string) string {
	return filepath.Join(dir, pluginName+"-"+BundledBridge+".dll")
}

// PluginDLL is the bridge as an MTPlugins plugin, which t7kb copies into the
// install's bin/plugins/ for a Radiant MTPlugins loads into.
func PluginDLL(dir string) string {
	return filepath.Join(dir, "mtplugins", pluginName, pluginName+".dll")
}

// mtplugins reports whether MTPlugins loads its host into this install's
// Radiants: its d3d9.dll proxy in bin/ with its RadiantLoader.ini, not turned off.
func mtplugins(bin string) bool {
	if _, err := os.Stat(filepath.Join(bin, "d3d9.dll")); err != nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(bin, "RadiantLoader.ini"))
	if err != nil {
		return false // another program's d3d9.dll
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.EqualFold(strings.TrimSpace(k), "enabled") {
			return strings.TrimSpace(v) != "0"
		}
	}
	return true
}

// module is the loaded module of that base name, case-insensitively, or "".
func module(mods []string, match func(base string) bool) string {
	for _, p := range mods {
		if match(strings.ToLower(filepath.Base(p))) {
			return p
		}
	}
	return ""
}

func named(name string) func(string) bool {
	return func(base string) bool { return base == strings.ToLower(name) }
}

// standaloneLoaded matches the standalone bridge of any version.
func standaloneLoaded(base string) bool {
	return strings.HasPrefix(base, strings.ToLower(pluginName)+"-") && strings.HasSuffix(base, ".dll")
}

// loadBridge makes the bridge run in the Radiant of that pid and connects to
// it: through MTPlugins when it loads into the install's Radiants, else by
// injecting the standalone bridge. One host only ever runs in a Radiant.
func loadBridge(ctx context.Context, m Machine, pid int, o OpenOptions) (*Client, error) {
	bin := filepath.Join(o.Root, "bin")
	if err := m.Ready(ctx, pid); err != nil {
		if !m.Running(pid) {
			return nil, fmt.Errorf("the Radiant of pid %d exited before its bridge answered", pid)
		}
		return nil, err
	}
	mods, err := m.Modules(pid)
	if err != nil {
		return nil, err
	}
	if module(mods, named("RadiantPlugins.dll")) != "" || mtplugins(bin) {
		return loadPlugin(ctx, m, pid, bin, o.Bridge)
	}
	if p := module(mods, standaloneLoaded); p != "" {
		return nil, fmt.Errorf("the Radiant of pid %d runs the bridge %s, which doesn't answer: close Radiant and open the map again", pid, filepath.Base(p))
	}
	dll := StandaloneDLL(o.Bridge)
	if _, err := os.Stat(dll); err != nil {
		return nil, fmt.Errorf("t7kb's Radiant bridge isn't beside it (%s): reinstall t7kb", dll)
	}
	if err := m.Inject(pid, dll); err != nil {
		return nil, fmt.Errorf("could not load the bridge into Radiant (pid %d): %w", pid, err)
	}
	return waitBridge(ctx, m, pid, "T7RadiantBridge.log beside "+dll)
}

// loadPlugin has MTPlugins' host load t7kb's plugin build: copied into
// bin/plugins/ when the install's differs, then enabled.
func loadPlugin(ctx context.Context, m Machine, pid int, bin, dir string) (*Client, error) {
	mods, err := waitHost(ctx, m, pid, bin)
	if err != nil {
		return nil, err
	}
	src, dst := PluginDLL(dir), filepath.Join(bin, "plugins", pluginName, pluginName+".dll")
	ours, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("t7kb's Radiant bridge isn't beside it (%s): reinstall t7kb", src)
	}
	if theirs, err := os.ReadFile(dst); err != nil || !bytes.Equal(ours, theirs) {
		if module(mods, named(pluginName+".dll")) != "" {
			if _, err := m.Control(pid, "plugins disable "+pluginName); err != nil {
				return nil, err
			}
		}
		if err := writeFile(dst, ours); err != nil {
			return nil, err
		}
		if _, err := m.Control(pid, "plugins rescan"); err != nil {
			return nil, err
		}
	}
	if _, err := m.Control(pid, "plugins enable "+pluginName); err != nil {
		return nil, err
	}
	return waitBridge(ctx, m, pid, filepath.Join(bin, "plugins", "RadiantPlugins.log"))
}

// waitHost waits for MTPlugins' host to take commands in the Radiant, and
// returns the Radiant's modules then. Its proxy loads it as Radiant starts, but
// it opens its control window seconds later, at the end of its own start, in
// which it loads the plugins it finds: once it answers, whether it loaded the
// install's plugin is settled.
func waitHost(ctx context.Context, m Machine, pid int, bin string) ([]string, error) {
	for {
		_, err := m.Control(pid, "plugins list")
		if err == nil {
			return m.Modules(pid)
		}
		if !m.Running(pid) {
			return nil, fmt.Errorf("the Radiant of pid %d exited before MTPlugins' host answered: see %s", pid, filepath.Join(bin, "RadiantLoader.log"))
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("MTPlugins is installed, but its host never answered in Radiant (pid %d): %v; see %s", pid, err, filepath.Join(bin, "RadiantLoader.log"))
		case <-time.After(pollEvery):
		}
	}
}

func writeFile(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}
