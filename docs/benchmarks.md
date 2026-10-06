# Benchmarks

What a call to `t7kb` costs: time and memory for the retrieval path, and **tokens** for every MCP tool's answer, since that is what an agent actually pays for. Baseline: v2.2.2, Windows, 16 threads, NVMe, real `t7kb.db` (66,023 documents, ~350k chunks) and a full BO3 mod-tools install.

## How to run

Synthetic benchmarks need neither the database nor the model, and run anywhere (the `bench` workflow runs them on demand):

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
| `gdt_schema` | material + material_type lit | 1 | 1226 | 1226 | 1226 | 4673 |
| `gdt_schema` | xmodel | 1 | 2446 | 2446 | 2446 | 9956 |
| `zone_explain` | the asset with the longest chain | 1 | 337 | 337 | 337 | 1123 |
| `zone_contents` | every line of the zone | 1 | 1243 | 1243 | 1243 | 4019 |
| `zone_contents` | its heaviest line | 1 | 972 | 972 | 972 | 3071 |
| `zone_check` | the map | 1 | 1899 | 1899 | 1899 | 6509 |
| `zone_predict` | the map | 1 | 3313 | 3313 | 3313 | 10459 |
| `zone_predict` | its heaviest zone line | 1 | 549 | 549 | 549 | 1800 |
| `zone_predict` | why: the asset with the longest chain | 1 | 111 | 111 | 111 | 341 |
| `create` | dry run, ZM Advanced Level | 1 | 873 | 873 | 873 | 3017 |
| `create` | dry run, ZM Mod Level | 1 | 228 | 228 | 228 | 907 |

`get` serves long bodies a page at a time (16,000 characters by default, `max_chars` up to 64,000, `offset` for the next part). Corpus bodies: median 1.2k characters, p99 275k, maximum 98 MB (`source-workspace` indexes whole GDTs and scripts). Over 90% of documents fit in one page. A full page of a GDT is ~5.3k tokens rather than ~4k: paths and numbers tokenize denser than prose.

Paging costs answer quality only when the matched passage sits past the first page: of the bench queries' 48 top-3 hits, 41 fit in one page, and of the 7 paged ones the snippet's passage was on page 1 for 4. With `find` set to a phrase from the snippet, it is in the page returned for all 7.

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
