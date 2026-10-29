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

// PluginDLL is the bridge as an MTPlugins plugin, which t7kb copies into the
// install's bin/plugins/ for MTPlugins' host to load.
func PluginDLL(dir string) string {
	return filepath.Join(dir, "mtplugins", pluginName, pluginName+".dll")
}

// MTPluginsURL is where MTPlugins' installer is.
const MTPluginsURL = "https://github.com/Aidan601/MTPlugins/releases/latest"

// mtpluginsMissing says why MTPlugins doesn't load its host into this
// install's Radiants, nil when it does: its d3d9.dll proxy in bin/ with its
// RadiantLoader.ini, not turned off. The map tools need it: only its host
// loads the bridge, t7kb loads nothing into Radiant itself.
func mtpluginsMissing(bin string) error {
	if _, err := os.Stat(filepath.Join(bin, "d3d9.dll")); err != nil {
		return fmt.Errorf("the map tools need MTPlugins, which loads t7kb's Radiant bridge into Radiant, and it isn't installed: "+
			"install it from %s (the t7kb:setup-or-update skill offers it)", MTPluginsURL)
	}
	b, err := os.ReadFile(filepath.Join(bin, "RadiantLoader.ini"))
	if err != nil {
		return fmt.Errorf("the map tools need MTPlugins, and %s is another program's (no RadiantLoader.ini beside it): install MTPlugins from %s",
			filepath.Join(bin, "d3d9.dll"), MTPluginsURL)
	}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.EqualFold(strings.TrimSpace(k), "enabled") && strings.TrimSpace(v) == "0" {
			return fmt.Errorf("the map tools need MTPlugins, which is turned off (enabled=0 in %s): turn it back on",
				filepath.Join(bin, "RadiantLoader.ini"))
		}
	}
	return nil
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

// standaloneLoaded matches the standalone bridge earlier t7kb injected, of any
// version: its own host would clash with MTPlugins'.
func standaloneLoaded(base string) bool {
	return strings.HasPrefix(base, strings.ToLower(pluginName)+"-") && strings.HasSuffix(base, ".dll")
}

// loadBridge makes the bridge run in the Radiant of that pid and connects to
// it, through MTPlugins' host. One host only ever runs in a Radiant.
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
	if module(mods, named("RadiantPlugins.dll")) != "" { // MTPlugins' host runs: an old bridge steps aside for it
		return loadPlugin(ctx, m, pid, bin, o.Bridge)
	}
	if p := module(mods, standaloneLoaded); p != "" {
		return nil, fmt.Errorf("the Radiant of pid %d runs the bridge %s, which doesn't answer: close Radiant and open the map again", pid, filepath.Base(p))
	}
	if err := mtpluginsMissing(bin); err != nil {
		return nil, fmt.Errorf("%w; then close the Radiant of pid %d and open the map again: MTPlugins loads as Radiant starts", err, pid)
	}
	return loadPlugin(ctx, m, pid, bin, o.Bridge) // its host loads as Radiant starts, seconds before it answers
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
