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
| `gdt_schema` | material + material_type lit | 1 | 1223 | 1223 | 1223 | 4670 |
| `gdt_schema` | xmodel | 1 | 2446 | 2446 | 2446 | 9956 |
| `zone_explain` | the asset with the longest chain | 1 | 337 | 337 | 337 | 1123 |
| `zone_contents` | every line of the zone | 1 | 1243 | 1243 | 1243 | 4019 |
| `zone_contents` | its heaviest line | 1 | 972 | 972 | 972 | 3071 |
| `zone_check` | the map | 1 | 1879 | 1879 | 1879 | 6392 |
| `create` | dry run, ZM Advanced Level | 1 | 873 | 873 | 873 | 3017 |
| `create` | dry run, ZM Mod Level | 1 | 228 | 228 | 228 | 907 |

`get` serves long bodies a page at a time (16,000 characters by default, `max_chars` up to 64,000, `offset` for the next part). Corpus bodies: median 1.2k characters, p99 275k, maximum 98 MB (`source-workspace` indexes whole GDTs and scripts). Over 90% of documents fit in one page. A full page of a GDT is ~5.3k tokens rather than ~4k: paths and numbers tokenize denser than prose.

Paging costs answer quality only when the matched passage sits past the first page: of the bench queries' 48 top-3 hits, 41 fit in one page, and of the 7 paged ones the snippet's passage was on page 1 for 4. With `find` set to a phrase from the snippet, it is in the page returned for all 7.

## Retrieval quality

`TestRetrievalQuality` scores search against 14 judged queries (`internal/cli/testdata/retrieval_eval.json`), one or more per skill domain. Each lists its **key** documents (the API doc, the reference tutorial, the thread with the fix) and **relevant** ones, judged by reading the candidates from three independent paths: the hybrid top 30, the BM25 top 15, and a title search over the non-Discord sources. A judged doc_id the db lacks fails the test.

```
T7KB_BENCH_DB=<t7kb.db> HF_HOME=<model cache> go test ./internal/cli -run TestRetrievalQuality -v
```

Two judged sets, scored by the same test:

- **tuning** (`retrieval_eval.json`, 14 queries) — the ranking weights were chosen on it;
- **held-out** (`retrieval_holdout.json`, 18 queries) — other themes and phrasings, candidates pooled from both rankings and judged blind (sorted by doc_id, without knowing which ranking proposed which). It checks that a choice generalises.

| key@5 / recall@10 / MRR / nDCG@10 | tuning | held-out |
|---|---|---|
| v2.2 ranking | 0.71 / 0.66 / 1.00 / 0.69 | 0.83 / 0.44 / 0.82 / 0.50 |
| + title ×10 (**current**) | 0.79 / 0.67 / 1.00 / 0.70 | 0.83 / 0.43 / 0.82 / 0.50 |
| + reliability weight 0.25 alone | 0.86 / 0.68 / 1.00 / 0.72 | 0.78 / 0.39 / 0.79 / 0.47 |
| title ×10 + reliability 0.25 (shipped in 2.3.0) | 0.86 / 0.68 / 1.00 / 0.72 | 0.78 / 0.39 / 0.79 / 0.47 |

**Decision.** The title weight and a reliability weight together lift tuning key@5 from 0.71 to 0.86 but lose on the held-out set (0.83 → 0.78). Nearly all the tuning gain is the reliability weight, which favours API pages and Treyarch's scripts — right for the tuning set's "what's the API for…" questions, wrong for easter-egg songs, water, localization or devgui, where the best answers are community threads it pushed down. The title weight alone leaves the held-out set where it was and still brings `PlayFXOnTag`'s API page into the tuning top 5, so it stays; reliability is back to a tiebreak. **The held-out set has been used once, for this decision**: a later ranking decision needs a fresh blind set.

**Why the title weight.** Two failures on tuning key documents: BM25 required every query word (an AND, stopwords included — `sound alias plays silently` matched no document), and on the vector side reference pages ranked far down (the API page for `RegisterClientField` 45th–89th, the modme LUI tutorials 270th–587th) because a chunk of code no longer resembles the question. Reference pages are short with precise titles, so a title match counts 10× a body one (summary 5×); 5, 10 and 20 score the same.

**Tried and rejected** on the tuning set: OR instead of AND terms (key@5 0.50 — long stock scripts that repeat the words fill BM25's pool), dropping stopwords, joined compound terms (`clientfield register` → `registerclientfield`), a per-source cap, and per-document title vectors as a third fused list (no gain, so no database change).

The tuning judgments are pooled: the top 10 of each ranking variant was judged too (as relevant only), so a ranking that surfaces relevant documents the baseline missed isn't scored as noise.

**Still failing unfiltered**: `clientfield register set lua` and `custom lua hud widget` come back all Discord, with the API page and the LUI tutorials absent from the top 10 — no ranking weight reaches them. The `source` filter does: the eval's `filtered` checks (an agent passing the kind of answer it wants) put a key document at rank 3 for both (`source: api`, `source: wiki`), and at rank 1 for `PlayFXOnTag` (`api`) and the wallbuy fix (`wiki,forums`). The test fails if one drops out of the top 5.

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
