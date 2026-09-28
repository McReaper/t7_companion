package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/store"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <doc_id>",
		Short: "Print a document's full body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			doc, err := st.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if doc == nil {
				return fmt.Errorf("no such doc_id: %s", args[0])
			}
			printDoc(cmd.OutOrStdout(), doc)
			return nil
		},
	}
}

func printDoc(out io.Writer, d *store.Doc) {
	fmt.Fprintf(out, "# %s\n\n", d.Title)
	fmt.Fprintf(out, "doc_id: %s\nsource: %s  ·  reliability: %.2f\n", d.DocID, d.Source, d.Reliability)
	if d.URL != "" {
		fmt.Fprintf(out, "url: %s\n", d.URL)
	}
	fmt.Fprintf(out, "\n%s\n", d.Body)
}
