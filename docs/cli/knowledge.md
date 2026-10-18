# Knowledge: look it up

The knowledge base gathers Black Ops III modding's sources in one local index: Treyarch's docs, scripts and GSC/CSC API, the asset and entity schemas, wikis, forums, the mod-tools Discord and video transcripts. Search matches your words and their meaning together, so a question phrased your own way still finds the page that uses other words. [All commands](../cli.md).

## `t7kb`

With no command, an interactive search: type a query, then a result's number to read it whole. An empty line or `q` quits.

## `t7kb search <query>…`

Ranked results, each with its `doc_id` (what `get` takes), title, source and a snippet.

```
t7kb search zombie barrier window
t7kb search get_player_out_of_playable_area_monitor_wait_time --source scripts --bm25 -n 5
```

| Option | What it does |
| --- | --- |
| `--source` | only these sources, comma-separated: a group (`api`, `scripts`, `docs`, `wiki`, `forums`, `discord`, `video`, `workspace`) or an exact source name; a name that doesn't exist is an error listing those that do |
| `-n`, `--num` | how many results (default 10) |
| `--bm25` | keywords only: fast, and best for an exact function or error string |
| `--vector` | meaning only; run it beside `--bm25` to compare what each half finds |
| `--scores` | each hit's fused score and its source's reliability |

The groups: `api` is the GSC/CSC function reference, `scripts` Treyarch's shipped scripts and data, `docs` the official docs, schemas and tool docs, `workspace` the files of a mod-tools install.

## `t7kb get <doc_id>`

A document's body, a page at a time: long ones (whole scripts, GDTs, transcripts) end with the offset of the next page.

```
t7kb get discord-bo3modtools::general_1224885285746380800
t7kb get <doc_id> --find "zone_init"
```

| Option | What it does |
| --- | --- |
| `--find` | start the page at this word or phrase, ignoring case; with `--offset`, the next match after it |
| `--offset` | where to start, as the note ending the previous page gives it |
| `--max-chars` | page size (default 16,000 characters, at most 64,000) |
| `--all` | the whole body, however long |
