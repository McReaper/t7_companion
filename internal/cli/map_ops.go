package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant"
)

// radiantMachine is where the map tools find Radiant: this computer, or a
// test's fakes.
var radiantMachine radiant.Machine = radiant.Local

// openTimeout bounds radiant_open: a cold Radiant takes tens of seconds to
// start, then loads the map.
const openTimeout = 3 * time.Minute

// radiantOpenOut is a radiant_open answer.
type radiantOpenOut struct {
	PID    int    `json:"pid"`
	Map    string `json:"map"`
	Opened string `json:"opened"` // already open, reloaded or launched
	Plugin string `json:"plugin"` // the bridge plugin's version
}

// radiantOpen makes a Radiant show the map (internal/radiant has the rules).
func radiantOpen(ctx context.Context, toolsPath, mapArg string, pid int, discard bool) (*radiantOpenOut, error) {
	root := toolsRoot(toolsPath)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	mapPath, err := mapFile(root, mapArg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	o, err := radiant.Open(ctx, radiantMachine, radiant.OpenOptions{Root: root, Map: mapPath, PID: pid, Discard: discard, Bridge: bridgeDir()})
	if err != nil {
		return nil, err
	}
	return &radiantOpenOut{PID: o.PID, Map: o.Map, Opened: o.How, Plugin: o.Plugin}, nil
}

// mapFile is the .map a map argument names: a path ending in .map, relative
// to map_source/ unless absolute, or a map's name, whose .map is
// map_source/<its first two letters>/<name>.map, where the Launcher keeps it.
func mapFile(root, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	var p string
	switch {
	case arg == "":
		return "", fmt.Errorf("no map: pass a map's name (zm_castle) or its .map path")
	case strings.EqualFold(filepath.Ext(arg), ".map"):
		p = arg
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, "map_source", p)
		}
	case len(arg) < 3 || strings.ContainsAny(arg, `/\`):
		return "", fmt.Errorf("%q is no map name (zm_castle) nor a .map path", arg)
	default:
		p = filepath.Join(root, "map_source", strings.ToLower(arg[:2]), arg+".map")
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no map at %s (create makes a new one)", p)
	}
	return p, nil
}

// bridgeDir is where the bridge DLLs t7kb ships are: bridge/ beside its
// binary, or $T7KB_BRIDGE_DIR (a build of the bridge, for its development).
func bridgeDir() string {
	if d := os.Getenv("T7KB_BRIDGE_DIR"); d != "" {
		return d
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "bridge")
}
