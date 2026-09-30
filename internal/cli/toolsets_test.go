package cli

import (
	"strings"
	"testing"
)

// Every MCP tool must be reachable from the CLI too (identical behaviour is the
// rule: both go through the same <feature>_ops.go function), with a unique name
// and a description the agent can route on.
func TestToolsetsHaveUniqueDescribedToolsWithCLICommands(t *testing.T) {
	root := newRootCmd()
	seen := map[string]string{}
	for _, ts := range toolsets(nil, nil) {
		if len(ts.tools) == 0 {
			t.Errorf("toolset %q has no tools", ts.name)
		}
		for _, tool := range ts.tools {
			name := tool.Tool.Name
			if prev, dup := seen[name]; dup {
				t.Errorf("tool %q is registered by both %q and %q", name, prev, ts.name)
			}
			seen[name] = ts.name
			if strings.TrimSpace(tool.Tool.Description) == "" {
				t.Errorf("tool %q has no description", name)
			}
			if tool.Handler == nil {
				t.Errorf("tool %q has no handler", name)
			}
			// gdt_edit -> `t7kb gdt edit`; search -> `t7kb search`
			args := strings.SplitN(name, "_", 2)
			cmd, rest, err := root.Find(args)
			if err != nil || cmd == root || len(rest) != 0 {
				t.Errorf("tool %q has no matching CLI command `t7kb %s`", name, strings.Join(args, " "))
			}
		}
	}
}
