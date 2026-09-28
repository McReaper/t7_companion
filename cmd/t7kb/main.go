// Command t7kb is the agent-first Black Ops 3 modding knowledge tool: an MCP
// server (primary) plus a small CLI, querying a local t7kb.db with hybrid
// retrieval (BM25 + vector).
package main

import "github.com/McReaper/t7_companion/internal/cli"

func main() {
	cli.Execute()
}
