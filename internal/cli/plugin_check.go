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

// t7kbInstalls returns every install of the t7kb plugin, one per scope.
// Without Claude Code (or with an output it can't read) it returns nothing:
// the plugin check is a bonus on top of the binary check, never a failure.
func t7kbInstalls() []pluginInstall {
	raw, err := listPlugins()
	if err != nil {
		return nil
	}
	var all []pluginInstall
	if json.Unmarshal(raw, &all) != nil {
		return nil
	}
	var mine []pluginInstall
	for _, p := range all {
		if strings.HasPrefix(p.ID, "t7kb@") {
			mine = append(mine, p)
		}
	}
	return mine
}

// keptInstalls splits installs into the one(s) to keep and the extra scopes
// to remove: a user-scope install covers every project, so beside one, a
// project or local install only doubles what has to be kept up to date.
func keptInstalls(installs []pluginInstall) (keep, extra []pluginInstall) {
	if !slices.ContainsFunc(installs, func(p pluginInstall) bool { return p.Scope == "user" }) {
		return installs, nil
	}
	for _, p := range installs {
		if p.Scope == "user" {
			keep = append(keep, p)
		} else {
			extra = append(extra, p)
		}
	}
	return keep, extra
}

// stale returns the installs whose version isn't latest.
func stale(installs []pluginInstall, latest string) []pluginInstall {
	var out []pluginInstall
	for _, p := range installs {
		if strings.TrimPrefix(p.Version, "v") != latest {
			out = append(out, p)
		}
	}
	return out
}

// runFrom is the hint for a command that acts on a project's own settings.
func runFrom(p pluginInstall) string {
	if p.ProjectPath == "" {
		return ""
	}
	return "   (run from " + p.ProjectPath + ")"
}

// reportExtraScopes prints the commands that remove the project/local
// installs a user-scope install already covers.
func reportExtraScopes(out io.Writer, extra []pluginInstall) {
	if len(extra) == 0 {
		return
	}
	fmt.Fprintln(out, "\nThe t7kb plugin is also installed per project; your user-scope install already covers")
	fmt.Fprintln(out, "every project, so keep that one and remove the others:")
	for _, p := range extra {
		fmt.Fprintf(out, "  claude plugin uninstall %s --scope %s%s\n", p.ID, p.Scope, runFrom(p))
	}
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
		fmt.Fprintf(out, "  claude plugin update %s --scope %s%s\n", p.ID, p.Scope, runFrom(p))
	}
	fmt.Fprintln(out, "then restart Claude Code. To stop this recurring, enable auto-update for the marketplace in /plugin.")
}
