# The t7kb CLI

Every MCP tool is also a command, with the same behaviour (`gdt_edit` is `t7kb gdt edit`). `t7kb <command> --help` gives every flag. From a terminal:

```
t7kb                         browse: type a query, pick a hit, read it
t7kb search <query>...       search  (--source api,wiki… · -n N · --bm25)
t7kb get <doc_id>            read a document  (--find · --offset · --all)
t7kb create <name>           new map or mod from a template  (--write)
t7kb build <name>            compile/light/link/run  (--stages · --mod · --json)
t7kb gdt <command>           find | get | schema | edit | check | refs
t7kb zone <command>          explain | contents | check
t7kb shader decompile <name> a stock shader's HLSL  (--stage · --json)
t7kb radiant open <map>      open a map in Radiant, or start it  (--pid · --discard)
t7kb map find [id…]          the open map's entities, ids and bounds  (--class · --near · --brushes)
t7kb map view [id…]          an image of a view, ids on it  (--view top · --render fake · --out)
t7kb map edit --ops <file|->  a batch of edits, one undo step  (--label)
t7kb map command <path> [id…] a Radiant menu entry on what the ids name  (--menu)
t7kb map materials           materials to texture with, most used first  (--usage · --surface · --name · --show)
t7kb mcp                     run the MCP server
t7kb update-check            is a newer release out? is the plugin behind?
```
