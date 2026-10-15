//go:build !windows

package build

import "fmt"

// readLauncherDvars: the Launcher and its saved settings exist only on Windows.
func readLauncherDvars() ([]dvar, error) {
	return nil, fmt.Errorf("launcher dvars are read from the Windows registry; pass dvars explicitly")
}

// readLauncherLanguage: no Launcher, no saved Build Language.
func readLauncherLanguage() string { return "" }
