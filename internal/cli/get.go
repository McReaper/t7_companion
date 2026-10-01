package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var offset, maxChars int
	var find string
	var all bool
	cmd := &cobra.Command{
		Use:   "get <doc_id>",
		Short: "Print a document (long ones a page at a time, as the MCP get tool serves them)",
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
			size := pageSize(maxChars)
			if all {
				size = 0
			}
			page, err := docPage(doc, offset, find, size)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), page)
			return err
		},
	}
	cmd.Flags().IntVar(&offset, "offset", 0, "where to start in the body (from a previous page's truncation note)")
	cmd.Flags().IntVar(&maxChars, "max-chars", defaultPageChars, fmt.Sprintf("page size in characters (at most %d)", maxPageChars))
	cmd.Flags().StringVar(&find, "find", "", "start the page at this word or phrase (case-insensitive; with --offset, the next occurrence after it)")
	cmd.Flags().BoolVar(&all, "all", false, "print the whole body, however long")
	return cmd
}
