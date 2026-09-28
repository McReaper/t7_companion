package cli

import (
	"fmt"
	"math"
	"strings"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/embed"
)

// newEmbedCmd is a hidden helper to sanity-check the embedder.
func newEmbedCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "embed <text>...",
		Short:  "Debug: embed text and print the vector summary",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			emb, err := embed.New()
			if err != nil {
				return err
			}
			v, err := emb.Embed(strings.Join(args, " "))
			if err != nil {
				return err
			}
			var sumSq float64
			for _, x := range v {
				sumSq += float64(x) * float64(x)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "dim=%d  L2norm=%.4f  head=[%.4f %.4f %.4f %.4f]\n",
				len(v), math.Sqrt(sumSq), v[0], v[1], v[2], v[3])
			return nil
		},
	}
}
