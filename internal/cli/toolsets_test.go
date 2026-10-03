package cli

import (
	"os"
	"regexp"
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

// Skills name MCP tools as t7kb:<tool>, and check_skills.py accepts only the
// names in its TOOL_NAMES: it must list every registered tool, or a skill that
// names a new tool fails validation.
func TestSkillValidatorKnowsEveryTool(t *testing.T) {
	src, err := os.ReadFile("../../plugin/skills/contribute/scripts/check_skills.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^TOOL_NAMES = \{([^}]*)\}`).FindSubmatch(src)
	if m == nil {
		t.Fatal("TOOL_NAMES not found in check_skills.py")
	}
	known := map[string]bool{}
	for _, q := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(m[1], -1) {
		known[string(q[1])] = true
	}
	for _, ts := range toolsets(nil, nil) {
		for _, tool := range ts.tools {
			if !known[tool.Tool.Name] {
				t.Errorf("check_skills.py's TOOL_NAMES lacks %q", tool.Tool.Name)
			}
		}
	}
}
