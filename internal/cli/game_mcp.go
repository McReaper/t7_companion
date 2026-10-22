package cli

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// gameToolset reads what the game said while it ran.
func gameToolset() toolset {
	return toolset{name: "game", tools: []server.ServerTool{gameLogTool()}}
}

func gameLogTool() server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("game_log",
		mcp.WithDescription("Investigate what the game printed while it ran, from its console_mp.log: use it when the user "+
			"reports something going wrong in game, or after a build's run. Finds the current log (the newest of the game root's "+
			"and each mod's; t7kb:build's run makes the game write one), sorts its last launch into script errors (message "+
			"and call stack), the map's own problems (assets its zone lists that the game didn't find, Error lines of other "+
			"kinds) and a count of the stock lookups every load fails, each with its line number; and gives the raw lines "+
			"asked for (grep, tail), and the dialogs the running game shows. The answer names the file and the other logs "+
			"beside it (crashes.log, older ones): read around a line yourself when the sorting misses something. In Claude "+
			"Code the t7kb plugin's monitor (t7kb game watch) tells you each problem as it happens; elsewhere, after reads "+
			"only what is new."),
		mcp.WithString("name", mcp.Description("The map or mod the run was of: picks its log and the zone to sort against. Omit for the newest log.")),
		mcp.WithString("path", mcp.Description("A log file to read instead of finding the current one.")),
		mcp.WithString("grep", mcp.Description("A regular expression (RE2, case-insensitive): the last launch's lines that match, with context.")),
		mcp.WithNumber("context", mcp.Description("Lines before and after each grep match (default 2, at most 10).")),
		mcp.WithNumber("max", mcp.Description("Grep matches given at most (default 50, at most 200); the rest are counted.")),
		mcp.WithNumber("tail", mcp.Description("The last launch's last lines (at most 500).")),
		mcp.WithNumber("after", mcp.Description("Read only the lines after this line of the file: what is new since an earlier answer's lines.")),
		mcp.WithString("tools_path", mcp.Description("BO3 mod-tools root (default $TA_TOOLS_PATH; $TA_GAME_PATH wins for the game root)."))),
		Handler: jsonHandler(func(_ context.Context, r mcp.CallToolRequest) (any, error) {
			return gameLog(gameLogArgs{ToolsPath: r.GetString("tools_path", ""), Name: r.GetString("name", ""), Path: r.GetString("path", ""),
				Grep: r.GetString("grep", ""), Context: r.GetInt("context", 0), Max: r.GetInt("max", 0), Tail: r.GetInt("tail", 0),
				After: r.GetInt("after", 0)})
		})}
}
