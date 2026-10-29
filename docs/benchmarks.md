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

What each map tool's call costs, measured on the stock ZM Mod Level template's map: [map-tools-tokens.md](map-tools-tokens.md).

## Retrieval quality

How well search ranks, and every ranking change tried and rejected: [retrieval-quality.md](retrieval-quality.md).

## Latency and memory

| Benchmark | Result |
|---|---|
| Search, real corpus (embed + search), p50 / max over 16 queries | 2.01 s / 2.17 s |
| Search in the MCP server, vectors in memory (embed excluded), p50 / p90 over 200 searches | 68 ms / 71 ms |
| `search` through the MCP server over stdio (embed included), p50 / p90 over 100 calls | 143 ms / 326 ms |
| Vector index load, MCP start (445k chunks) | 2.2 s, ~650 MB |
| MCP server memory (private bytes): after start / peak over 100 `search` calls | 1.14 GB / 1.43 GB |
| `vectorRank`, synthetic | ~5.3 µs and ~1.8 KB allocated per chunk (4k and 40k chunks) |
| `bm25Rank`, 10k docs | 41 ms |
| `rrfFuse`, 2 × 50 candidates | 11 µs |
| `Get`, one document | 0.12 ms |
| Embedding model load | 121 ms, 307 MB allocated |
| Embed one query | ~90 ms |

The vector scan dominates: ~445k chunks per query. It reads only each chunk's rowid, doc_id and vector and scores them in place, then reads the text of the winners alone for their snippets; what remains is the SQLite driver reading every row. The MCP server keeps every vector in memory instead (`vecindex.go`), scored on every CPU with the scan's results (`TestVectorIndexMatchesScan`); the CLI keeps the scan.

The MCP server's memory is the vectors (~650 MB), the GDT caches (~220 MB, with `TA_TOOLS_PATH` set) and the embedding model; at Go's default `GOGC` of 100 it held ~2.2 GB, twice what is live. It runs at `GOGC` 25 (`serverGCPercent`, `mcp.go`), which leaves search times unchanged since the vectors hold no pointers for the collector to read, and hands back to the OS what the warm-ups leave behind once they finish. A `GOGC` in the environment still wins; `T7KB_VECTOR_INDEX=0` keeps the vectors out of memory (~650 MB less, ~2 s a search).
