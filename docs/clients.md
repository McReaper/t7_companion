# Connecting an MCP client to t7kb

After installing (see the [README](../README.md)), every client runs the same thing: **`t7kb mcp`** as a stdio MCP server, pointed at your installed binary. Replace `/path/to/t7kb` below (Windows: `C:\path\to\t7kb.exe`).

**Claude Code** — the plugin does this for you (`/t7kb:setup-or-update`); to wire it manually:
```bash
claude mcp add t7kb -- "/path/to/t7kb" mcp
```
Always quote the path, even on one line — on Windows this command runs through a POSIX-style shell, which silently drops backslashes before non-special letters in an unquoted argument (`C:\Users\you\t7kb.exe` becomes `C:Usersyout7kb.exe`), registering a broken command. If `t7kb` ever shows as failed/disconnected, check the registered command (`claude mcp list`) for missing backslashes and re-add it quoted.

**Codex** — `~/.codex/config.toml`:
```toml
[mcp_servers.t7kb]
command = "/path/to/t7kb"
args = ["mcp"]
```

**OpenCode** — `opencode.json`:
```json
{ "mcp": { "t7kb": { "type": "local", "command": ["/path/to/t7kb", "mcp"], "enabled": true } } }
```

**Copilot (VS Code)** — `.vscode/mcp.json`:
```json
{ "servers": { "t7kb": { "type": "stdio", "command": "/path/to/t7kb", "args": ["mcp"] } } }
```

**Cursor** — `.cursor/mcp.json`:
```json
{ "mcpServers": { "t7kb": { "command": "/path/to/t7kb", "args": ["mcp"] } } }
```
