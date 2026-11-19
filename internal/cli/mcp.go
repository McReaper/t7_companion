package cli

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/journal"
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
}

func toolsets(st *store.Store, emb *embed.Embedder) []toolset {
	return []toolset{
		kbToolset(st, emb),
		buildToolset(),
		gdtToolset(),
		zoneToolset(),
		createToolset(),
		shaderToolset(),
		mapToolset(),
		gameToolset(),
	}
}

// runMCP opens the db, then serves every toolset over stdio. What the tools
// load — the search vectors, the embedding model, the GDT caches — loads on the
// first call that needs it, and is handed back once the server goes idle
// (releaseAfter): a session that never touches BO3 holds none of it. Nothing
// is written to stdout except the MCP protocol stream.
func runMCP() error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	if vectorIndexWanted() {
		st.EnableVectorIndex() // a long-lived server: search memory, not ~445k rows per query
	}
	emb := embed.Lazy()

	useServerGC()
	var idle *idleRelease
	if after := releaseAfter(); after > 0 {
		idle = newIdleRelease(after, func() { releaseLoaded(st, emb) })
		stop := make(chan struct{})
		defer close(stop)
		go idle.run(stop)
	}
	return server.ServeStdio(newMCPServer(st, emb, idle))
}

// releaseLoaded drops what tool calls loaded and hands the memory back to the
// OS, which Go otherwise keeps.
func releaseLoaded(st *store.Store, emb *embed.Embedder) {
	st.ReleaseVectors()
	emb.Release()
	releaseWorkspaces()
	debug.FreeOSMemory()
}

// serverGCPercent is the server's GOGC. Go's default, 100, lets the heap grow to
// twice what is live before collecting, and the vectors alone are ~650 MB: the
// server held ~2.2 GB. The vectors hold no pointers, so the collector barely
// reads them, and collecting at 25% leaves search times unchanged. GOGC set in
// the environment still wins.
const serverGCPercent = 25

func useServerGC() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(serverGCPercent)
	}
}

// mcpInstructions reach the agent in every session where the server is
// registered, so they hold only the rule that always applies: each tool's
// description says what it does, and the t7kb:* skills carry the craft.
const mcpInstructions = "t7kb is a local knowledge base of the Black Ops 3 (BO3) modding community, " +
	"with tools over this machine's Treyarch mod tools. For any BO3 modding question or task " +
	"(GSC/CSC, Radiant, zombies, assets, FX, audio, compile and link errors), search t7kb before " +
	"answering from memory, mid-task too: before asserting how the pipeline behaves, or asking the " +
	"user to confirm it. Read the top hits with get rather than answering from snippets. " +
	"When t7kb:* skills are available (the Claude Code plugin), load the one that owns the task " +
	"first: it holds the method and the traps that fail silently."

// newMCPServer registers every toolset on one server — the one runMCP serves,
// and the one tests drive in-process. idle, when set, counts every call.
func newMCPServer(st *store.Store, emb *embed.Embedder, idle *idleRelease) *server.MCPServer {
	s := server.NewMCPServer("t7kb", Version(), server.WithToolCapabilities(false),
		server.WithInstructions(mcpInstructions),
		server.WithRecovery()) // a panicking handler must not take the whole stdio server down
	dir := journal.Dir()
	for _, ts := range toolsets(st, emb) {
		tools := strictTools(ts.tools)
		if dir != "" { // T7KB_DEBUG: each call recorded, the refused ones too
			tools = journaledTools(dir, tools)
		}
		if idle != nil {
			tools = idle.wrap(tools)
		}
		s.AddTools(tools...)
	}
	return s
}

// vectorIndexWanted: the MCP server keeps the vectors in memory (~650 MB) unless
// T7KB_VECTOR_INDEX is 0, false or off — for a machine short of memory, where
// each search then reads them from the database (~2 s).
func vectorIndexWanted() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("T7KB_VECTOR_INDEX"))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}
