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

func newMapCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "map",
		Short: "Find, see and edit the map open in Radiant",
	}
	var a mapFindArgs
	find := &cobra.Command{Use: "find [id…]",
		Short: "List the entities that match, with their ids and bounds",
		RunE: func(c *cobra.Command, ids []string) error {
			a.IDs = ids
			return printIndented(c.OutOrStdout())(mapFind(c.Context(), a))
		}}
	fl := find.Flags()
	fl.StringVar(&a.Class, "class", "", "classname, * matching any run of characters")
	fl.StringVar(&a.Key, "key", "", "a key the entity has, or key=value")
	fl.StringVar(&a.Near, "near", "", "a point x,y,z: entities within --radius, nearest first")
	fl.Float64Var(&a.Radius, "radius", 0, "with --near: the distance in units (default 256)")
	fl.BoolVar(&a.Selected, "selected", false, "only entities with a brush selected in Radiant")
	fl.BoolVar(&a.Brushes, "brushes", false, "list each entity's brushes and patches")
	fl.StringVar(&a.Keys, "keys", "", "which keys to show: all, or a comma list (default the naming keys)")
	fl.IntVar(&a.Limit, "limit", 0, "how many entities at most (default 50)")
	fl.IntVar(&a.PID, "pid", 0, "the Radiant to read when several run")
	root.AddCommand(find)
	return root
}
