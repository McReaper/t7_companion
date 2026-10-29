package cli

import (
	"os"
	"runtime/debug"
	"strings"
	"sync"

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
	warm  func() // optional: warm-up the server runs in the background at start, returning once done
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

// runMCP opens the db and loads the embedder once, then serves every toolset over
// stdio. Nothing is written to stdout except the MCP protocol stream.
func runMCP() error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	if vectorIndexWanted() {
		st.EnableVectorIndex() // a long-lived server: search memory, not ~445k rows per query
	}

	emb, err := embed.New()
	if err != nil {
		return err
	}

	useServerGC()
	return server.ServeStdio(newMCPServer(st, emb, true))
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

// warmUp runs every toolset's warm-up at once in the background, then hands
// back to the OS what they left behind: loading the vectors alone reads ~650 MB
// of rows that are garbage once copied, and Go keeps freed memory otherwise.
func warmUp(warms []func(), release func()) {
	go func() {
		var wg sync.WaitGroup
		for _, w := range warms {
			wg.Go(w)
		}
		wg.Wait()
		release()
	}()
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
// and the one tests drive in-process.
func newMCPServer(st *store.Store, emb *embed.Embedder, warm bool) *server.MCPServer {
	s := server.NewMCPServer("t7kb", Version(), server.WithToolCapabilities(false),
		server.WithInstructions(mcpInstructions),
		server.WithRecovery()) // a panicking handler must not take the whole stdio server down
	dir := journal.Dir()
	var warms []func()
	for _, ts := range toolsets(st, emb) {
		tools := strictTools(ts.tools)
		if dir != "" { // T7KB_DEBUG: each call recorded, the refused ones too
			tools = journaledTools(dir, tools)
		}
		s.AddTools(tools...)
		if ts.warm != nil {
			warms = append(warms, ts.warm)
		}
	}
	if warm {
		warmUp(warms, debug.FreeOSMemory)
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
