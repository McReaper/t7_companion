package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

func newSearchCmd() *cobra.Command {
	var (
		num     int
		bm25    bool
		scores  bool
		sources []string
	)
	cmd := &cobra.Command{
		Use:   "search <query>...",
		Short: "Hybrid (BM25 + vector) search",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, strings.Join(args, " "), num, bm25, scores, sources)
		},
	}
	cmd.Flags().IntVarP(&num, "num", "n", 10, "number of results")
	cmd.Flags().BoolVar(&bm25, "bm25", false, "keyword-only (skip vector embedding)")
	cmd.Flags().BoolVar(&scores, "scores", false, "show fused RRF score + reliability per hit")
	cmd.Flags().StringSliceVar(&sources, "source", nil, "only these sources: a group ("+strings.Join(store.SourceGroups(), ", ")+") or a source name; comma-separated or repeated")
	return cmd
}

func runSearch(cmd *cobra.Command, query string, num int, bm25, scores bool, sources []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	only, err := st.ResolveSources(cmd.Context(), sources)
	if err != nil {
		return err
	}

	var qvec []float32
	if !bm25 {
		if qvec, err = embedQuery(query); err != nil {
			return err
		}
	}

	hits, err := st.SearchHybrid(cmd.Context(), query, qvec, num, only...)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "no results")
		return nil
	}
	printHits(cmd.OutOrStdout(), hits, scores)
	return nil
}

func embedQuery(query string) ([]float32, error) {
	emb, err := embed.New()
	if err != nil {
		return nil, err
	}
	return emb.Embed(query)
}

func printHits(out io.Writer, hits []store.Hit, showScores bool) {
	for i, h := range hits {
		if showScores {
			fmt.Fprintf(out, "%2d. [rrf %.4f · rel %.2f] %s  (%s)\n", i+1, h.Score, h.Reliability, h.DocID, h.Source)
		} else {
			fmt.Fprintf(out, "%2d. %s  (%s)\n", i+1, h.DocID, h.Source)
		}
		fmt.Fprintf(out, "    %s\n", h.Title)
		if h.Snippet != "" {
			fmt.Fprintf(out, "    %s\n", h.Snippet)
		}
	}
}
