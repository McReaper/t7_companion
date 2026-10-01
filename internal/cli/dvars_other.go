//go:build !windows

package cli

import "fmt"

// readLauncherDvars: the Launcher and its saved settings exist only on Windows.
func readLauncherDvars() ([]dvar, error) {
	return nil, fmt.Errorf("launcher dvars are read from the Windows registry; pass dvars explicitly")
}
