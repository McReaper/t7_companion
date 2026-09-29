// Package cli builds the t7kb command tree (cobra).
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/store"
)

var dbFlag string

// version is set at build time via -ldflags (GoReleaser); "dev" for local builds.
var version = "dev"

// Version returns the build version, used as the MCP server version too.
func Version() string { return version }

// Execute runs the root command, exiting non-zero on error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "t7kb",
		Short: "Hybrid search over a local Black Ops 3 modding knowledge base",
		Long: "t7kb queries a local t7kb.db with hybrid retrieval (BM25 + vector).\n\n" +
			"It is built to be driven by an AI agent over MCP (`t7kb mcp`). Run it with\n" +
			"no arguments for an interactive browse session, or use `search` / `get`.",
		Version:       version,
		Args:          cobra.ArbitraryArgs,
		RunE:          runBrowse,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&dbFlag, "db", "",
		"path to t7kb.db (default: $T7KB_DB, then beside the binary, then ./t7kb.db)")
	root.AddCommand(newSearchCmd(), newGetCmd(), newMCPCmd(), newEmbedCmd(), newUpdateCheckCmd(), newBuildCmd(), newGDTCmd())
	return root
}

// resolveDB picks where t7kb.db should live: --db > $T7KB_DB > beside the
// binary > cwd. It returns the intended path whether or not the file exists yet
// (ensureDB unpacks a sibling t7kb.db.zip there on first run).
func resolveDB() string {
	if dbFlag != "" {
		return dbFlag
	}
	if env := os.Getenv("T7KB_DB"); env != "" {
		return env
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "t7kb.db")
	}
	return "t7kb.db"
}

func openStore() (*store.Store, error) {
	path := resolveDB()
	if err := ensureDB(path); err != nil {
		return nil, err
	}
	return store.Open(path)
}
