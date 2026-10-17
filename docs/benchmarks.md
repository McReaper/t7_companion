# Benchmarks

What a call to `t7kb` costs: time and memory for the retrieval path, and **tokens** for every MCP tool's answer, since that is what an agent actually pays for. Baseline: v2.2.2, Windows, 16 threads, NVMe, real `t7kb.db` (66,023 documents, ~350k chunks) and a full BO3 mod-tools install.

## How to run

Synthetic benchmarks need neither the database nor the model, and run anywhere:

```
go test ./internal/store -run '^$' -bench . -benchmem
HF_HOME=<model cache> go test ./internal/embed -run '^$' -bench . -benchmem
```

The token report runs against the real corpus and install, so it is opt-in:

```
T7KB_BENCH_DB=<t7kb.db> HF_HOME=<model cache> TA_TOOLS_PATH=<bo3 root> \
  T7KB_TOKEN_REPORT=tokens.md go test ./internal/cli -run TestTokenReport -v
```

It builds each answer with the same functions the MCP handlers use and counts tokens with `o200k_base`, an offline BPE tokenizer. Claude's tokenizer isn't public, so read the counts as relative: they compare formats and catch regressions, not bill.

## Tokens per tool answer

16 queries phrased the way a modder asks (`benchQueries` in `internal/cli/tokens_report_test.go`); the `gdt_*` rows use one real material, its image and its GDT; the `zone_*` rows the first linked map of the install (12k packed assets).

| Tool | Call | Samples | Tokens p50 | p90 | max | Bytes p50 |
|---|---|---:|---:|---:|---:|---:|
| `search` | 10 results (default) | 16 | 725 | 791 | 842 | 2726 |
| `search` | 5 results | 16 | 345 | 381 | 439 | 1331 |
| `get` | top result | 16 | 664 | 3447 | 5256 | 2194 |
| `get` | each of the top 3 | 48 | 493 | 3944 | 5256 | 1893 |
| `gdt_find` | a material | 1 | 52 | 52 | 52 | 159 |
| `gdt_get` | a material (defaults hidden) | 1 | 219 | 219 | 219 | 660 |
| `gdt_get` | a material, all=true | 1 | 240 | 240 | 240 | 742 |
| `gdt_refs` | its color image | 1 | 102 | 102 | 102 | 338 |
| `gdt_refs` | the material (GDT fields and LOD files) | 1 | 245 | 245 | 245 | 786 |
| `gdt_check` | one asset | 1 | 38 | 38 | 38 | 124 |
| `gdt_check` | the whole GDT | 1 | 2005 | 2005 | 2005 | 6878 |
| `gdt_schema` | material (names only) | 1 | 2894 | 2894 | 2894 | 11408 |
| `gdt_schema` | material + material_type lit | 1 | 1241 | 1241 | 1241 | 4715 |
| `gdt_schema` | xmodel | 1 | 2446 | 2446 | 2446 | 9956 |
| `zone_explain` | the asset with the longest chain | 1 | 337 | 337 | 337 | 1123 |
| `zone_contents` | every line of the zone | 1 | 1317 | 1317 | 1317 | 4280 |
| `zone_contents` | its heaviest line | 1 | 972 | 972 | 972 | 3071 |
| `zone_contents` | images: the map | 1 | 1320 | 1320 | 1320 | 3839 |
| `zone_contents` | images: its heaviest line | 1 | 1185 | 1185 | 1185 | 3497 |
| `zone_check` | the map | 1 | 1899 | 1899 | 1899 | 6509 |
| `create` | dry run, ZM Advanced Level | 1 | 873 | 873 | 873 | 3017 |
| `create` | dry run, ZM Mod Level | 1 | 228 | 228 | 228 | 907 |

`get` serves long bodies a page at a time (16,000 characters by default, `max_chars` up to 64,000, `offset` for the next part). Corpus bodies: median 1.2k characters, p99 275k, maximum 98 MB (`source-workspace` indexes whole GDTs and scripts). Over 90% of documents fit in one page. A full page of a GDT is ~5.3k tokens rather than ~4k: paths and numbers tokenize denser than prose.

Paging costs answer quality only when the matched passage sits past the first page: of the bench queries' 48 top-3 hits, 41 fit in one page, and of the 7 paged ones the snippet's passage was on page 1 for 4. With `find` set to a phrase from the snippet, it is in the page returned for all 7.

### The map tools

Measured on the stock ZM Mod Level template's map (33 entities, 20 world brushes), what a map made by `create` holds, served by `radianttest`'s fake Radiant with the views of a real one: a 1648×869 3D view, a 693×1165 2D view (`mapSamples` in `internal/cli/tokens_report_map_test.go`). An image counts as Claude prices it, about width × height / 750 tokens; the JSON beside it, and the ops an agent writes, as text.

| Tool | Call | Tokens | Bytes |
|---|---|---:|---:|
| `radiant_open` | the map already open | 53 | 183 |
| `map_find` | every entity (33) | 1598 | 5334 |
| `map_find` | class `script_struct` (15) | 766 | 2842 |
| `map_find` | near the player start, 256 units | 690 | 2319 |
| `map_find` | worldspawn's 20 brushes | 667 | 1716 |
| `map_find` | one entity, all keys | 85 | 265 |
| `map_view` | top, framed on the start zone: JSON + 457×768 image | 48 + 467 | 117 + 3477 |
| `map_view` | 3D, framed, `render fake`: JSON + 768×405 image | 52 + 414 | 129 + 2610 |
| `map_view` | 3D as it is, `size 1568`: JSON + 1568×827 image | 84 + 1728 | 190 + 8402 |
| `map_edit` | the ops an agent writes for a room (floor, 4 walls, ceiling, a light) | 254 | 643 |
| `map_edit` | the answer to them, with each material's tile | 269 | 699 |
| `map_edit` | the ops for a floor, two walls caulked top and bottom, a pillar copied three times | 238 | 556 |
| `map_edit` | the answer to them (7 made, 5 materials' tiles) | 375 | 949 |
| `map_edit` | a `room` op: a room with a door | 76 | 178 |
| `map_edit` | the answer to it (8 pieces) | 343 | 846 |
| `map_edit` | moving a point entity | 49 | 140 |
| `map_command` | listing `Selection/CSG` | 32 | 111 |
| `map_command` | `CSG Hollow` on a box | 206 | 579 |
| `map_materials` | brick walls, 20 listed | 656 | 2122 |
| `map_materials` | six wood floors shown: JSON + 768×405 image | 362 + 414 | 1077 + 3585 |
| `map_check` | the template and the rooms above, 50 findings listed | 1618 | 5158 |
| `map_check` | seams, stretch and unknown materials only | 210 | 608 |

A room costs about a thousand tokens: its ops (254), the answer (269), a top view to check it (515); as one `room` op, door and caulk included, the ops are 76. The whole map's `map_find` is the dearest call, about 48 tokens an entity, so `class`, `near` or `ids` narrow it; the default image, 768 pixels on the long side, stays near 450 tokens, and `size 1568` quadruples it. A list of materials costs about 33 tokens a material, a finding of `map_check` about 32: `kinds` narrows a long one.

## Retrieval quality

How well search ranks, and every ranking change tried and rejected: [retrieval-quality.md](retrieval-quality.md).

## Latency and memory

| Benchmark | Result |
|---|---|
| Search, real corpus (embed + search), p50 / max over 16 queries | 2.01 s / 2.17 s |
| Search in the MCP server, vectors in memory (embed excluded), p50 / max | 41 ms / 258 ms |
| Vector index load, MCP start | 4.6 s, ~566 MB |
| `vectorRank`, synthetic | ~5.3 µs and ~1.8 KB allocated per chunk (4k and 40k chunks) |
| `bm25Rank`, 10k docs | 41 ms |
| `rrfFuse`, 2 × 50 candidates | 11 µs |
| `Get`, one document | 0.12 ms |
| Embedding model load | 121 ms, 307 MB allocated |
| Embed one query | ~90 ms |

The vector scan dominates: ~380k chunks per query. It reads only each chunk's rowid, doc_id and vector and scores them in place, then reads the text of the winners alone for their snippets; what remains is the SQLite driver reading every row. The MCP server keeps every vector in memory instead (`vecindex.go`), scored on every CPU with the scan's results (`TestVectorIndexMatchesScan`); the CLI keeps the scan.
