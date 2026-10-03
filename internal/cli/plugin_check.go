package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// pluginInstall is one entry of `claude plugin list --json`: a plugin can be
// installed once per scope (user, project, local), each at its own version.
type pluginInstall struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath"`
}

// listPlugins runs Claude Code's public plugin listing; a var so tests can fake it.
var listPlugins = func() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "claude", "plugin", "list", "--json").Output()
}

// stalePlugins returns the t7kb plugin installs whose version isn't latest.
// Without Claude Code (or with an output it can't read) it returns nothing:
// the plugin check is a bonus on top of the binary check, never a failure.
func stalePlugins(latest string) []pluginInstall {
	raw, err := listPlugins()
	if err != nil {
		return nil
	}
	var all []pluginInstall
	if json.Unmarshal(raw, &all) != nil {
		return nil
	}
	var stale []pluginInstall
	for _, p := range all {
		if strings.HasPrefix(p.ID, "t7kb@") && strings.TrimPrefix(p.Version, "v") != latest {
			stale = append(stale, p)
		}
	}
	return stale
}

// reportStalePlugins prints, for each outdated install, the commands that
// update it. A third-party marketplace doesn't auto-update by default, so an
// install stays on its version until someone runs these.
func reportStalePlugins(out io.Writer, stale []pluginInstall, tag string) {
	if len(stale) == 0 {
		return
	}
	fmt.Fprintf(out, "\nThe t7kb Claude Code plugin (its skills) is behind %s:\n", tag)
	var marketplaces []string
	for _, p := range stale {
		where := p.Scope + " scope"
		if p.ProjectPath != "" {
			where += ", project " + p.ProjectPath
		}
		fmt.Fprintf(out, "  %s %s (%s)\n", p.ID, p.Version, where)
		if m := strings.SplitN(p.ID, "@", 2)[1]; !slices.Contains(marketplaces, m) {
			marketplaces = append(marketplaces, m)
		}
	}
	fmt.Fprintln(out, "Update it with:")
	for _, m := range marketplaces {
		fmt.Fprintf(out, "  claude plugin marketplace update %s\n", m)
	}
	for _, p := range stale {
		cmd := fmt.Sprintf("claude plugin update %s --scope %s", p.ID, p.Scope)
		if p.ProjectPath != "" {
			cmd += "   (run from " + p.ProjectPath + ")"
		}
		fmt.Fprintf(out, "  %s\n", cmd)
	}
	fmt.Fprintln(out, "then restart Claude Code. To stop this recurring, enable auto-update for the marketplace in /plugin.")
}
