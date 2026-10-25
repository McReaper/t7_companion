package cli

import (
	"fmt"
	"io"
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
	root.AddCommand(find, newMapViewCmd(), newMapEditCmd(), newMapCommandCmd())
	return root
}

func newMapCommandCmd() *cobra.Command {
	var menu string
	var pid int
	command := &cobra.Command{Use: "command <menu path> [id…]",
		Short: "Run a Radiant menu entry on the entities and brushes named, or list a menu (--menu)",
		RunE: func(c *cobra.Command, a []string) error {
			if c.Flags().Changed("menu") {
				return printIndented(c.OutOrStdout())(mapCommand(c.Context(), "", a, &menu, pid))
			}
			if len(a) == 0 {
				return fmt.Errorf("give a menu path (Selection/CSG/CSG Hollow), or --menu to list a menu's entries (--menu= the menu bar)")
			}
			return printIndented(c.OutOrStdout())(mapCommand(c.Context(), a[0], a[1:], nil, pid))
		}}
	command.Flags().StringVar(&menu, "menu", "", "list this menu's entries instead (Selection/CSG); --menu= (empty) lists the menu bar")
	command.Flags().IntVar(&pid, "pid", 0, "the Radiant to use when several run")
	return command
}

func newMapEditCmd() *cobra.Command {
	var file, label, revsJSON string
	var pid int
	edit := &cobra.Command{Use: "edit --ops <file|->",
		Short: "Apply a batch of ops (a JSON array, as map_edit takes) as one undo step",
		Long: "Apply a batch of ops, a JSON array, as one undo step. It is the MCP tool map_edit: --ops, --revs and\n" +
			"--label are its ops, revs and label, and the tools it names are t7kb's commands (map_find is\n" +
			"t7kb map find). map_edit's description:\n\n" + mapEditHelp,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var raw []byte
			var err error
			if file == "-" {
				raw, err = io.ReadAll(c.InOrStdin())
			} else {
				raw, err = os.ReadFile(file)
			}
			if err != nil {
				return err
			}
			ops, err := decodeOps(raw)
			if err != nil {
				return err
			}
			var revs map[string]string
			if revsJSON != "" {
				if err := decodeStrict([]byte(revsJSON), &revs); err != nil {
					return fmt.Errorf("--revs: an object of id to rev: %w", err)
				}
			}
			return printIndented(c.OutOrStdout())(mapEdit(c.Context(), ops, revs, label, pid))
		}}
	edit.Flags().StringVar(&file, "ops", "-", "the ops' JSON file, - for stdin")
	edit.Flags().StringVar(&label, "label", "", "what the batch does, for Radiant's undo menu")
	edit.Flags().StringVar(&revsJSON, "revs", "", `the rev map_find gave each entity and brush the ops name, as JSON {"id": "rev"}; required for each`)
	edit.Flags().IntVar(&pid, "pid", 0, "the Radiant to edit when several run")
	return edit
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
	fl.StringVar(&a.Hide, "hide", "", "volumes, sky or both comma-separated: hide them for this shot")
	fl.BoolVar(&a.Bake, "bake", false, "3d: bake Radiant's lighting first (its toolbar's bolt, at its L/M/H) and shoot under real lighting")
	fl.IntVar(&a.Size, "size", 0, "the image's longer side in pixels (default 768)")
	fl.BoolVar(&a.NoLabels, "no-labels", false, "leave the ids off the image")
	fl.IntVar(&a.PID, "pid", 0, "the Radiant to see when several run")
	fl.StringVar(&out, "out", "map_view.png", "where to write the image")
	return view
}
