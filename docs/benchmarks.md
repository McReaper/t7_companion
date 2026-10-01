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

Paging only costs answer quality when the passage the search matched sits past the first page: of the bench queries' 48 top-3 hits, 41 fit in one page, and of the 7 paged ones the snippet's passage was on page 1 for 4. With `find` set to a phrase from the snippet, it is in the page returned for all 7.

## Retrieval quality

`TestRetrievalQuality` scores search against 14 judged queries (`internal/cli/testdata/retrieval_eval.json`), one or more per skill domain. Each lists its **key** documents (the API doc, the reference tutorial, the thread with the fix) and **relevant** ones, judged by reading the candidates from three independent paths: the hybrid top 30, the BM25 top 15, and a title search over the non-Discord sources — so the judgments don't simply echo what search already returns. A judged doc_id the db lacks fails the test.

```
T7KB_BENCH_DB=<t7kb.db> HF_HOME=<model cache> go test ./internal/cli -run TestRetrievalQuality -v
```

| Metric (mean over 14 queries) | v2.2 ranking | Title-weighted + reliability (current) |
|---|---:|---:|
| key@5 — a key document in the top 5 | 0.71 | **0.86** |
| recall@10 — key + relevant documents in the top 10 | 0.66 | 0.68 |
| MRR — 1 / rank of the first useful document | 1.00 | 1.00 |
| nDCG@10 — graded ranking quality (key 2, relevant 1) | 0.69 | 0.72 |

(Scored on the pooled judgments, so both columns use the same set; before pooling the v2.2 ranking scored 0.71 / 0.64 / 0.87 / 0.58.)

**What changed the ranking.** Diagnosing where each key document ranked showed two separate failures. BM25 required every query word (an AND, stopwords included): `sound alias plays silently` matched no document at all, `custom wallbuy shows cost 0` one; and on the vector side, reference pages ranked far down (the API page for `RegisterClientField` 45th–89th, the modme LUI tutorials 270th–587th) because a chunk of code no longer resembles the question. The fix that measured best is two weights in `internal/store`: a BM25 match in the **title counts 10×** one in the body (summary 5×) — reference pages are short with precise titles — and the fused score is scaled by **1 + 0.25 × (reliability − 0.5)**, so an API page edges out a Discord thread on a near tie. `PlayFXOnTag`'s API page and the wiki's "Wallbuy 0 Fix" move into the top 5; `SetHintString` and `GiveWeapon` go 0.65 → 0.77 and 0.66 → 0.80 in nDCG; the queries whose answers are Discord threads stay where they were. The one dip, `sound alias plays silently` (0.82 → 0.69), is the same ten documents reordered: two relevant forum threads now come before two key Discord ones.

**Tried and rejected**, on the same queries: OR instead of AND terms (key@5 0.50 — long stock scripts that repeat the words fill BM25's pool), dropping stopwords, joined compound terms (`clientfield register` → `registerclientfield`), a per-source cap, a reliability weight of 0.75 or more (stock scripts crowd out the answers), and per-document title vectors as a third fused list — prototyped with the corpus's own model, they changed nothing over the two weights above, so no database change was needed for this.

**Still failing unfiltered**: `clientfield register set lua` and `custom lua hud widget` come back all Discord, with the API page and the LUI tutorials absent from the top 10 — no ranking weight reaches them. The `source` filter does: the eval's `filtered` checks (an agent passing the kind of answer it wants) put a key document at rank 3 for both (`source: api`, `source: wiki`), and at rank 1 for `PlayFXOnTag` (`api`) and the wallbuy fix (`wiki,forums`). The test fails if one drops out of the top 5; the knowledge-base skill and AGENTS.md tell the agent when to pass it.

The judgments were pooled: after each round of variants, the top 10 of the variants was judged too (56 documents added, as relevant only), so a ranking that surfaces relevant documents the baseline missed isn't scored as if it surfaced noise.

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
