//go:build !windows

package build

// processWindows: the game and its dialogs run only on Windows.
func processWindows(int) []window { return nil }
