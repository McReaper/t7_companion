package cli

import (
	"fmt"
	"os"

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
	root.AddCommand(find, newMapViewCmd())
	return root
}

func newMapViewCmd() *cobra.Command {
	var a mapViewArgs
	var out string
	view := &cobra.Command{Use: "view [id…]",
		Short: "Save an image of a view of the map, with ids on it, and print its projection",
		RunE: func(c *cobra.Command, ids []string) error {
			a.IDs = ids
			res, png, err := mapView(c.Context(), a)
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, png, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(c.ErrOrStderr(), "t7kb: wrote %s\n", out)
			return printIndented(c.OutOrStdout())(res, nil)
		}}
	fl := view.Flags()
	fl.StringVar(&a.View, "view", "", "3d (default), top, front or side")
	fl.StringVar(&a.At, "at", "", "a point x,y,z to frame")
	fl.Float64Var(&a.Radius, "radius", 0, "with --at: how far around it to show (default 256)")
	fl.StringVar(&a.Camera, "camera", "", "3d: shoot from x,y,z,pitch,yaw,roll")
	fl.StringVar(&a.Render, "render", "", "3d: fake, fullbright, wireframe, real, view or density for this shot")
	fl.IntVar(&a.Size, "size", 0, "the image's longer side in pixels (default 768)")
	fl.BoolVar(&a.NoLabels, "no-labels", false, "leave the ids off the image")
	fl.IntVar(&a.PID, "pid", 0, "the Radiant to see when several run")
	fl.StringVar(&out, "out", "map_view.png", "where to write the image")
	return view
}
