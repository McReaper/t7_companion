package build

import (
	"fmt"
	"strings"
)

// The run stage can start the game with dvars, as the Launcher's Dvar Options
// do: `+set <dvar> <value>` for each, before fs_game and devmap (captured from
// the stock modlauncher.exe). The two that are console commands rather than
// dvars are passed as `+<name> <value>`.

// dvarCommands are run as `+<name> <value>`, not `+set <name> <value>`.
var dvarCommands = map[string]bool{"connect": true, "set_gametype": true}

type dvar struct{ name, value string }

// parseDvars reads "name=value" pairs (CLI --dvar).
func parseDvars(pairs []string) ([]dvar, error) {
	var out []dvar
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t\"") {
			return nil, fmt.Errorf("--dvar wants name=value, got %q", p)
		}
		out = append(out, dvar{k, strings.TrimSpace(v)})
	}
	return out, nil
}

// mergeDvars overrides base with extra (same name, case-insensitively, keeps its
// place; a new one goes last).
func mergeDvars(base, extra []dvar) []dvar {
	out := append([]dvar(nil), base...)
	for _, e := range extra {
		replaced := false
		for i := range out {
			if strings.EqualFold(out[i].name, e.name) {
				out[i], replaced = e, true
			}
		}
		if !replaced {
			out = append(out, e)
		}
	}
	return out
}

// dvarArgs renders the dvars as game arguments; an empty value is left out, as
// the Launcher leaves out an empty connect or password.
func dvarArgs(dvars []dvar) []string {
	var args []string
	for _, d := range dvars {
		if d.value == "" {
			continue
		}
		if dvarCommands[d.name] {
			args = append(args, "+"+d.name, d.value)
		} else {
			args = append(args, "+set", d.name, d.value)
		}
	}
	return args
}

// launcherValue turns a stored Launcher setting into the value it passes:
// checkboxes are stored as "true"/"false" and passed as 1/0.
func launcherValue(v string) string {
	switch strings.ToLower(v) {
	case "true":
		return "1"
	case "false":
		return "0"
	}
	return v
}
