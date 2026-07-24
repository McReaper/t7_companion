package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/t7-reapy/t7_companion/internal/embed"
	"github.com/t7-reapy/t7_companion/internal/store"
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

// runMCP opens the db and loads the embedder once, then serves search/get over
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

	s := server.NewMCPServer("t7kb", Version(), server.WithToolCapabilities(false))
	s.AddTool(searchToolDef(), searchToolHandler(st, emb))
	s.AddTool(getToolDef(), getToolHandler(st))
	s.AddTool(buildToolDef(), buildToolHandler())
	return server.ServeStdio(s)
}

func searchToolDef() mcp.Tool {
	return mcp.NewTool("search",
		mcp.WithDescription("Search the Black Ops 3 modding knowledge base with hybrid "+
			"(keyword + semantic) retrieval. Returns ranked results — doc_id, title, "+
			"source, reliability, and a snippet. Use `get` to fetch a full body."),
		mcp.WithString("query", mcp.Required(),
			mcp.Description("Natural-language question or keywords.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results (default 10).")),
	)
}

func searchToolHandler(st *store.Store, emb *embed.Embedder) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		limit := req.GetInt("limit", 10)

		qvec, err := emb.Embed(query)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("embed query", err), nil
		}
		hits, err := st.SearchHybrid(ctx, query, qvec, limit)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("search", err), nil
		}
		return mcp.NewToolResultText(formatHits(hits)), nil
	}
}

func formatHits(hits []store.Hit) string {
	if len(hits) == 0 {
		return "No results."
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s  (source: %s, reliability: %.2f)\n", i+1, h.DocID, h.Source, h.Reliability)
		fmt.Fprintf(&b, "   %s\n", h.Title)
		if h.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", h.Snippet)
		}
	}
	return b.String()
}

func getToolDef() mcp.Tool {
	return mcp.NewTool("get",
		mcp.WithDescription("Fetch a document's full body by its doc_id (from a search result)."),
		mcp.WithString("doc_id", mcp.Required(),
			mcp.Description("The doc_id to fetch, e.g. \"gscode-api::api.gsc.setclientfield\".")),
	)
}

func getToolHandler(st *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docID, err := req.RequireString("doc_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		doc, err := st.Get(ctx, docID)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("get", err), nil
		}
		if doc == nil {
			return mcp.NewToolResultError("no such doc_id: " + docID), nil
		}
		return mcp.NewToolResultText(formatDoc(doc)), nil
	}
}

func formatDoc(d *store.Doc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", d.Title)
	fmt.Fprintf(&b, "doc_id: %s\nsource: %s  ·  reliability: %.2f\n", d.DocID, d.Source, d.Reliability)
	if d.URL != "" {
		fmt.Fprintf(&b, "url: %s\n", d.URL)
	}
	fmt.Fprintf(&b, "\n%s\n", d.Body)
	return b.String()
}

func buildToolDef() mcp.Tool {
	return mcp.NewTool("build",
		mcp.WithDescription("Headlessly compile/light/link a Black Ops 3 map or mod by driving the "+
			"mod-tools pipeline (gdtdb, cod2map64, radiant light, linker), returning a compact "+
			"per-stage JSON report with the first actionable error of any failing stage. Requires a "+
			"Windows BO3 mod-tools install (path from $TA_TOOLS_PATH or the tools_path arg). "+
			"NOTE: runs synchronously and can take minutes (link) up to 20-30 min (full compile+light) "+
			"— set a long client timeout. For a script-only change, pass stages=\"link\"."),
		mcp.WithString("name", mcp.Required(),
			mcp.Description("Map or mod name, e.g. \"zm_mymap\".")),
		mcp.WithString("stages",
			mcp.Description("Comma list of stages to run: compile,light,link,run (default \"compile,light,link\"). Use \"link\" alone for a script-only change.")),
		mcp.WithBoolean("mod",
			mcp.Description("Target is a mod (mods/<name>) instead of a usermap; skips compile+light (default false).")),
		mcp.WithString("light",
			mcp.Description("Light bake quality: low|medium|high (default \"medium\").")),
		mcp.WithBoolean("onlyents",
			mcp.Description("Fast entity-only compile (-onlyents); invalid after brush edits (default false).")),
		mcp.WithString("language",
			mcp.Description("Linker language (default \"english\").")),
		mcp.WithBoolean("skip_gdt",
			mcp.Description("Skip the gdtdb /update pass before building (default false).")),
		mcp.WithString("tools_path",
			mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH).")),
		mcp.WithString("game_path",
			mcp.Description("BO3 game root (default $TA_GAME_PATH, then tools_path).")),
	)
}

func buildToolHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		o := &buildOpts{
			toolsPath: req.GetString("tools_path", ""),
			gamePath:  req.GetString("game_path", ""),
			isMod:     req.GetBool("mod", false),
			stages:    req.GetString("stages", "compile,light,link"),
			onlyEnts:  req.GetBool("onlyents", false),
			light:     req.GetString("light", "medium"),
			language:  req.GetString("language", "english"),
			skipGDT:   req.GetBool("skip_gdt", false),
		}
		rep, err := runBuildReport(o, name, io.Discard)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("marshal build report", err), nil
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}
