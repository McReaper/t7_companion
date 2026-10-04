package cli

import "github.com/spf13/cobra"

func newCreateCmd() *cobra.Command {
	var toolsPath, template, zones string
	var write bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new map or mod from the install's templates (dry run unless --write)",
		Long: "Create a new usermap or mod from rex/templates, as the Launcher's File > New does.\n" +
			"Without --write it only lists the files it would create; it never overwrites one.\n" +
			"Example: t7kb create zm_leviathan --write",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return printIndented(c.OutOrStdout())(createOp(toolsPath, a[0], template, splitZones(zones), write))
		},
	}
	f := cmd.Flags()
	f.StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	f.StringVar(&template, "template", "", `a folder of rex/templates (default: "ZM Mod Level" for zm_, "MP Mod Level" for mp_, "Mod" otherwise)`)
	f.StringVar(&zones, "zones", "", "mods only: zones to keep, among core,mp,cp,zm (default: all)")
	f.BoolVar(&write, "write", false, "create the files (default: dry run)")
	return cmd
}
