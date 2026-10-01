//go:build windows

package cli

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// readLauncherDvars reads the Launcher's saved Dvar Options
// (HKCU\Software\Treyarch\ModLauncher, dvar_<name>), in its order.
func readLauncherDvars() ([]dvar, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Treyarch\ModLauncher`, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("no saved Launcher dvars (HKCU\\Software\\Treyarch\\ModLauncher): open the mod tools Launcher's Dvars dialog once: %w", err)
	}
	defer func() { _ = k.Close() }() // a read-only key
	var out []dvar
	for _, name := range launcherDvars {
		if s, _, err := k.GetStringValue("dvar_" + name); err == nil {
			out = append(out, dvar{name, launcherValue(s)})
		} else if n, _, err := k.GetIntegerValue("dvar_" + name); err == nil {
			out = append(out, dvar{name, strconv.FormatUint(n, 10)})
		}
	}
	return out, nil
}
