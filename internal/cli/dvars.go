package cli

import (
	"fmt"
	"sort"
	"strconv"
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

// dvarPairs turns the MCP dvars object into sorted "name=value" pairs; numbers
// and booleans are accepted as values (true -> 1, false -> 0).
func dvarPairs(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dvars must be an object of name: value")
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	var out []string
	for _, k := range names {
		var s string
		switch x := m[k].(type) {
		case string:
			s = x
		case bool:
			s = map[bool]string{true: "1", false: "0"}[x]
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		default:
			return nil, fmt.Errorf("dvar %q: value must be a string, number or boolean", k)
		}
		out = append(out, k+"="+s)
	}
	return out, nil
}
