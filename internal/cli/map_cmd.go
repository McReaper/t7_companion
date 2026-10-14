package cli

import (
	"github.com/spf13/cobra"
)

func newRadiantCmd() *cobra.Command {
	var toolsPath string
	root := &cobra.Command{
		Use:   "radiant",
		Short: "Drive Radiant through the T7RadiantBridge plugin",
	}
	root.PersistentFlags().StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	var pid int
	var discard bool
	open := &cobra.Command{Use: "open <map name | .map path>",
		Short: "Open a map in the running Radiant, or start Radiant on it",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return printIndented(c.OutOrStdout())(radiantOpen(c.Context(), toolsPath, a[0], pid, discard))
		}}
	open.Flags().IntVar(&pid, "pid", 0, "the Radiant to use when several run")
	open.Flags().BoolVar(&discard, "discard", false, "drop the Radiant's unsaved changes to load the map")
	root.AddCommand(open)
	return root
}
