package cli

import (
	"github.com/spf13/cobra"
)

func newGameCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "game",
		Short: "What the game printed and showed while it ran",
	}
	var a gameLogArgs
	log := &cobra.Command{Use: "log [map-or-mod]", Short: "Sort the game's console_mp.log, grep or tail it",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				a.Name = args[0]
			}
			return printIndented(c.OutOrStdout())(gameLog(a))
		}}
	f := log.Flags()
	f.StringVar(&a.ToolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH; $TA_GAME_PATH wins for the game root)")
	f.StringVar(&a.Path, "path", "", "a log file to read instead of finding the current one")
	f.StringVar(&a.Grep, "grep", "", "the last launch's lines matching this regular expression (case-insensitive), with context")
	f.IntVar(&a.Context, "context", 0, "lines before and after each grep match (default 2, at most 10)")
	f.IntVar(&a.Max, "max", 0, "grep matches given at most (default 50, at most 200)")
	f.IntVar(&a.Tail, "tail", 0, "the last launch's last lines (at most 500)")
	f.IntVar(&a.After, "after", 0, "read only the lines after this line of the file: what is new since an earlier read")
	root.AddCommand(log)
	return root
}
