package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newShaderCmd() *cobra.Command {
	var toolsPath string
	root := &cobra.Command{
		Use:   "shader",
		Short: "Decompile the game's compiled shaders to HLSL, the start of a custom shader",
	}
	root.PersistentFlags().StringVar(&toolsPath, "tools-path", "", "BO3 mod-tools root (default: $TA_TOOLS_PATH)")
	var stage string
	var asJSON bool
	decompile := &cobra.Command{Use: "decompile <cache file | shader source | material type>",
		Short: "Print a shader's HLSL, or the programs its source compiled to",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, a []string) error {
			out, err := shaderDecompile(toolsPath, a[0], stage, 0, 0, true)
			if err != nil || asJSON || out.HLSL == "" {
				return printIndented(c.OutOrStdout())(out, err)
			}
			if out.Incomplete != "" {
				fmt.Fprintf(c.ErrOrStderr(), "t7kb: decompiled in part: %s\n", out.Incomplete)
			}
			_, err = fmt.Fprint(c.OutOrStdout(), out.HLSL)
			return err
		}}
	decompile.Flags().StringVar(&stage, "stage", "", "ps, vs, gs or cs: the stage to decompile")
	decompile.Flags().BoolVar(&asJSON, "json", false, "print the answer as JSON, as the MCP tool gives it")
	root.AddCommand(decompile)
	return root
}
