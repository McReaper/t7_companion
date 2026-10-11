package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/McReaper/t7_companion/internal/gdt"
)

var (
	wsMu    sync.Mutex
	wsCache = map[string]*gdt.Workspace{}
)

// toolsRoot is the mod-tools root a tool works on: its tools_path argument,
// else $TA_TOOLS_PATH.
func toolsRoot(toolsPath string) string {
	if toolsPath == "" {
		toolsPath = os.Getenv("TA_TOOLS_PATH")
	}
	return strings.TrimRight(toolsPath, `\/`)
}

// workspace opens (and caches, for the long-lived MCP server) the mod-tools root.
func workspace(toolsPath string) (*gdt.Workspace, error) {
	root := toolsRoot(toolsPath)
	if root == "" {
		return nil, fmt.Errorf("no mod-tools path: pass tools_path / --tools-path or set TA_TOOLS_PATH")
	}
	root = filepath.Clean(root)
	key := strings.ToLower(root) // C:\x and c:/x are one install
	wsMu.Lock()
	defer wsMu.Unlock()
	if w, ok := wsCache[key]; ok {
		return w, nil
	}
	if _, err := os.Stat(root + "/deffiles"); err != nil {
		return nil, fmt.Errorf("%s has no deffiles/ — not a BO3 mod-tools root", root)
	}
	w, err := gdt.Open(root)
	if err != nil {
		return nil, err
	}
	wsCache[key] = w
	return w, nil
}

// ---- operations shared by CLI and MCP
