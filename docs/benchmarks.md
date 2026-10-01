# Benchmarks

What a call to `t7kb` costs: time and memory for the retrieval path, and **tokens** for every MCP tool's answer, since that is what an agent actually pays for. Baseline taken on 2026-10-01 (v2.2.2), Windows, 16 threads, NVMe, real `t7kb.db` (66,023 documents, ~350k chunks) and a full BO3 mod-tools install.

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

16 queries phrased the way a modder asks (`benchQueries` in `internal/cli/tokens_report_test.go`); the `gdt_*` rows use one real material, its image and its GDT.

| Tool | Call | Samples | Tokens p50 | p90 | max | Bytes p50 |
|---|---|---:|---:|---:|---:|---:|
| `search` | 10 results (default) | 16 | 725 | 791 | 842 | 2726 |
| `search` | 5 results | 16 | 345 | 381 | 439 | 1331 |
| `get` | top result | 16 | 664 | 3447 | 5256 | 2194 |
| `get` | each of the top 3 | 48 | 493 | 3944 | 5256 | 1893 |
| `gdt_find` | a material | 1 | 52 | 52 | 52 | 159 |
| `gdt_get` | a material (defaults hidden) | 1 | 219 | 219 | 219 | 660 |
| `gdt_get` | a material, all=true | 1 | 240 | 240 | 240 | 742 |
| `gdt_refs` | its color image | 1 | 102 | 102 | 102 | 352 |
| `gdt_check` | one asset | 1 | 38 | 38 | 38 | 124 |
| `gdt_check` | the whole GDT | 1 | 2005 | 2005 | 2005 | 6878 |
| `gdt_schema` | material (names only) | 1 | 2894 | 2894 | 2894 | 11408 |
| `gdt_schema` | material + material_type lit | 1 | 1223 | 1223 | 1223 | 4670 |
| `gdt_schema` | xmodel | 1 | 2446 | 2446 | 2446 | 9956 |

`get` serves long bodies a page at a time (16,000 characters by default, `max_chars` up to 64,000, `offset` for the next part). Corpus bodies have a median of 1.2k characters but a p99 of 275k and a maximum of 98 MB (`source-workspace` indexes whole GDTs and scripts; 1,347 documents exceed 100k characters). Before paging, the same `get` rows had a p90 of 3,790 / 5,597 and a max of **38,278 / 41,308** tokens; the median is unchanged, since 90%+ of documents fit in one page. A full page of a GDT is ~5.3k tokens rather than ~4k: paths and numbers tokenize denser than prose.

## Latency and memory

| Benchmark | Result |
|---|---|
| Search, real corpus (embed + search), p50 / max over 16 queries | 2.52 s / 2.82 s |
| `vectorRank`, synthetic | ~7.8 µs and ~8 KB allocated per chunk (4k and 40k chunks) |
| `bm25Rank`, 10k docs | 41 ms |
| `rrfFuse`, 2 × 50 candidates | 11 µs |
| `Get`, one document | 0.12 ms |
| Embedding model load | 121 ms, 307 MB allocated |
| Embed one query | ~90 ms |

The vector scan dominates: at ~350k chunks, ~7.8 µs/chunk is ~2.7 s per query, matching the measured search time. It reads every chunk's text along with its vector, though only the best chunks' text becomes a snippet.
