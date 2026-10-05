# Retrieval quality

How well search ranks, and every ranking change tried: what was kept and what was rejected. Read it before tuning ranking. Run it against the real corpus (opt-in):

```
T7KB_BENCH_DB=<t7kb.db> HF_HOME=<model cache> go test ./internal/cli -run TestRetrievalQuality -v
```

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
