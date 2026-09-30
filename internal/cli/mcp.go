package cli

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP server over stdio (primary surface)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return runMCP()
		},
	}
}

// toolset is one feature's MCP tools. To add a feature: put its logic in its own
// internal/<pkg>, the operation shared by MCP and CLI in <feature>_ops.go (so the
// two can't drift), its tool definitions in <feature>_mcp.go returning a toolset,
// its cobra commands in <feature>_cmd.go — and list it in toolsets below.
type toolset struct {
	name  string
	tools []server.ServerTool
	warm  func() // optional: background warm-up when the server starts
}

func toolsets(st *store.Store, emb *embed.Embedder) []toolset {
	return []toolset{
		kbToolset(st, emb),
		buildToolset(),
		gdtToolset(),
	}
}

// runMCP opens the db and loads the embedder once, then serves every toolset over
// stdio. Nothing is written to stdout except the MCP protocol stream.
func runMCP() error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	emb, err := embed.New()
	if err != nil {
		return err
	}

	return server.ServeStdio(newMCPServer(st, emb, true))
}

// newMCPServer registers every toolset on one server — the one runMCP serves,
// and the one tests drive in-process.
func newMCPServer(st *store.Store, emb *embed.Embedder, warm bool) *server.MCPServer {
	s := server.NewMCPServer("t7kb", Version(), server.WithToolCapabilities(false),
		server.WithRecovery()) // a panicking handler must not take the whole stdio server down
	for _, ts := range toolsets(st, emb) {
		s.AddTools(ts.tools...)
		if warm && ts.warm != nil {
			ts.warm()
		}
	}
	return s
}
