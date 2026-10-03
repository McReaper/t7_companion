package cli

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

func newZoneCmd() *cobra.Command {
	var toolsPath string
	root := &cobra.Command{
		Use:   "zone",
		Short: "What a map's last link packed, and why (from the linker's own report)",
	}
	root.PersistentFlags().StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")

	var typ string
	explain := &cobra.Command{Use: "explain <map> <asset>", Short: "Show the chain that pulled an asset into the build",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, a []string) error {
			return printIndented(c.OutOrStdout())(zoneExplain(toolsPath, a[0], a[1], typ))
		}}
	explain.Flags().StringVar(&typ, "type", "", "asset type, when one name is used by several")

	contents := &cobra.Command{Use: "contents <map> [line]", Short: "Show what a zone line pulls in, or every line by weight",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, a []string) error {
			line := ""
			if len(a) == 2 {
				line = a[1]
			}
			return printIndented(c.OutOrStdout())(zoneContents(toolsPath, a[0], line))
		}}
	check := &cobra.Command{Use: "check <map>", Short: "Find your versions of stock assets that the build won't use",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			return printIndented(c.OutOrStdout())(zoneCheck(toolsPath, a[0]))
		}}
	root.AddCommand(explain, contents, check)
	return root
}

// printIndented prints an operation's answer as indented JSON, or returns its error.
func printIndented(out io.Writer) func(v any, err error) error {
	return func(v any, err error) error {
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	}
}
