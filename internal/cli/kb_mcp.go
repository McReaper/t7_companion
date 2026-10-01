package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

// kbToolset is the knowledge base: hybrid search and full-document get over t7kb.db.
func kbToolset(st *store.Store, emb *embed.Embedder) toolset {
	return toolset{name: "kb", tools: []server.ServerTool{
		{Tool: searchToolDef(), Handler: searchToolHandler(st, emb)},
		{Tool: getToolDef(), Handler: getToolHandler(st)},
	}}
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
		mcp.WithDescription("Fetch a document's body by its doc_id (from a search result). Long documents "+
			"(whole scripts, GDTs, transcripts) come a page at a time — about 16k characters by default — "+
			"ending with the offset to pass for the next part; most documents fit in one page."),
		mcp.WithString("doc_id", mcp.Required(),
			mcp.Description("The doc_id to fetch, e.g. \"gscode-api::api.gsc.setclientfield\".")),
		mcp.WithNumber("offset", mcp.Description("Where to start in the body, from a previous page's truncation note (default 0).")),
		mcp.WithNumber("max_chars", mcp.Description("Page size in characters (default 16000, at most 64000).")),
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
		page, err := renderDoc(doc, req.GetInt("offset", 0), pageSize(req.GetInt("max_chars", 0)))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(page), nil
	}
}
