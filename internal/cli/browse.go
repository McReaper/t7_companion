package cli

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

// runBrowse is the interactive default (bare `t7kb`): type a query to see ranked
// hits, type a result number to read its body, repeat. Avoids the
// search-then-copy-doc_id-then-get dance for humans at a terminal.
func runBrowse(cmd *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	emb, err := embed.New()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	sc := bufio.NewScanner(cmd.InOrStdin())
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	fmt.Fprintln(out, "t7kb — type a query; then a result number to open it. Empty line or 'q' quits.")

	var hits []store.Hit
	for {
		fmt.Fprint(out, "\nquery> ")
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line == "q" || line == "quit":
			return nil
		case isResultIndex(line, len(hits)):
			n, _ := strconv.Atoi(line)
			doc, err := st.Get(cmd.Context(), hits[n-1].DocID)
			if err != nil {
				return err
			}
			if doc != nil {
				page, _ := renderDoc(doc, 0, 0) // a document the reader picked: the whole body
				fmt.Fprint(out, page)
			}
		default:
			qvec, err := emb.Embed(line)
			if err != nil {
				return err
			}
			if hits, err = st.SearchHybrid(cmd.Context(), line, qvec, 10); err != nil {
				return err
			}
			if len(hits) == 0 {
				fmt.Fprintln(out, "(no results)")
			} else {
				printHits(out, hits, false)
			}
		}
	}
	return sc.Err()
}

// isResultIndex reports whether s is a 1..n result number.
func isResultIndex(s string, n int) bool {
	i, err := strconv.Atoi(s)
	return err == nil && i >= 1 && i <= n
}
