package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hookRun runs the plugin's SessionStart hook against a fake t7kb whose
// --version says binary and whose update-check --quiet prints latest, with
// env added. It returns what the hook printed and whether it asked for the
// latest release.
func hookRun(t *testing.T, plugin, binary, latest string, env ...string) (out string, asked bool) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on the PATH")
	}
	script, err := filepath.Abs("../../plugin/hooks/binary-version.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "{\n  \"name\": \"t7kb\",\n  \"version\": \"" + plugin + "\"\n}\n" // one key a line, as the real one
	if err := os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "asked")
	fake := filepath.Join(dir, "t7kb")
	body := "#!/bin/sh\ncase \"$1\" in\n" +
		"--version) echo 't7kb version " + binary + "' ;;\n" +
		"update-check) [ \"$2\" = --quiet ] && touch '" + filepath.ToSlash(marker) + "' && printf '%s' '" + latest + "' ;;\n" +
		"esac\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, script)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, append([]string{"CLAUDE_PLUGIN_ROOT=" + dir, "T7KB_BIN=" + filepath.ToSlash(fake)}, env...)...)
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	_, statErr := os.Stat(marker)
	return string(b), statErr == nil
}

// hookMessage decodes what the hook printed, which Claude Code reads as JSON.
func hookMessage(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		SystemMessage string `json:"systemMessage"`
		Hook          struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("the hook printed no valid JSON: %v\n%s", err, out)
	}
	if v.SystemMessage == "" || v.SystemMessage != v.Hook.AdditionalContext {
		t.Fatalf("the user and the agent must get the same message:\n%s", out)
	}
	return v.SystemMessage
}

// The hook says a newer release is out once the binary and the plugin agree,
// and asks nothing when they don't (that message already sends the user to
// update), for a dev build, or when the user turned non-essential traffic off.
func TestSessionStartHook(t *testing.T) {
	out, asked := hookRun(t, "4.1.0", "v4.1.0", "v4.2.0\n")
	if msg := hookMessage(t, out); !asked || !strings.Contains(msg, "t7kb: v4.2.0 is out (this install is 4.1.0)") || !strings.Contains(msg, "/t7kb:setup-or-update") {
		t.Errorf("a newer release must be announced: %q", msg)
	}

	if out, asked := hookRun(t, "4.1.0", "v4.1.0", ""); out != "" || !asked {
		t.Errorf("up to date: the hook asks and says nothing, got %q (asked %v)", out, asked)
	}

	out, asked = hookRun(t, "4.2.0", "v4.1.0", "v4.2.0")
	if msg := hookMessage(t, out); asked || !strings.Contains(msg, "binary is 4.1.0 but the t7kb plugin (its skills) is 4.2.0") {
		t.Errorf("a version mismatch is said alone, without asking GitHub: %q (asked %v)", msg, asked)
	}

	if out, asked := hookRun(t, "4.1.0", "dev", "v4.2.0"); out != "" || asked {
		t.Errorf("a dev build: got %q (asked %v), want nothing", out, asked)
	}

	if out, asked := hookRun(t, "4.1.0", "v4.1.0", "v4.2.0", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"); out != "" || asked {
		t.Errorf("with non-essential traffic off: got %q (asked %v), want nothing", out, asked)
	}

	out, _ = hookRun(t, "4.1.0", "v4.1.0", `v4.2.0"\`)
	if msg := hookMessage(t, out); !strings.Contains(msg, "t7kb: v4.2.0 is out (") {
		t.Errorf("a tag with JSON's special characters must not break the message: %q", msg)
	}
}
