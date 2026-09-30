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

// workspace opens (and caches, for the long-lived MCP server) the mod-tools root.
func workspace(toolsPath string) (*gdt.Workspace, error) {
	root := strings.TrimRight(firstNonEmpty(toolsPath, os.Getenv("TA_TOOLS_PATH")), `\/`)
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
