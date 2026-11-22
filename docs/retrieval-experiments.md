# Retrieval experiments

Ranking and database changes measured with `TestRetrievalQuality` since 2026-11, in the order they ran: what each one did to the three judged sets, and what was kept. The sets and the metrics are described in [retrieval-quality.md](retrieval-quality.md).

**Database rebuild, 2026-11.** This is not a ranking change. All three rows were measured the same day with the same ranking and model:

| key@5 / recall@10 / MRR / nDCG@10 | tuning | held-out |
|---|---|---|
| shipped database (`t7kb.db.zip`) | 0.71 / 0.68 / 1.00 / 0.70 | 0.83 / 0.42 / 0.78 / 0.48 |
| rebuilt, 256-token embedding cut | 0.71 / 0.57 / 0.93 / 0.60 | 0.83 / 0.37 / 0.76 / 0.43 |
| rebuilt, 128-token cut + wiki summary fixes (**kept**) | 0.71 / 0.68 / 1.00 / 0.71 | 0.83 / 0.42 / 0.78 / 0.48 |

- **Token cut.** The shipped database embeds only each chunk's first 128 tokens. That was fastembed's default when it was built, never a choice. fastembed 0.8 defaults to 256, so a plain rebuild changed every long chunk's vector, and it scored worse on both sets (the summary fixes on top didn't change that). `t7_knowledge` now pins the cut at 128 (`corpus.EMBEDDING_MAX_TOKENS`, enforced by `build_kb.py`). Queries stay well under 128 tokens, so go-sentex's own 256 cut still matches.
- **Wiki summaries.** The wiki sources write their lede as `description`, which `build_kb.py` used to drop, so those docs had no summary. wiki-modme also took 17 ledes from inside code blocks. Only those docs changed: 442,089 of the 445,231 chunk vectors are identical. Only `custom lua hud widget` moved (nDCG@10 0.48 → 0.58).
- **Held-out reuse.** The held-out set was read again for these two database choices, and it moved with the tuning set both times. Both sets' judgments were pooled from rankings on the 128-token database, which works against any vector change: a document only the 256 cut surfaces is unjudged and scores as noise. Part of the 256 loss may therefore be unjudged documents rather than worse ones.
- **Baseline.** Until a release ships this rebuild, keep comparing against the shipped database. Once it ships, its row is the baseline.

**Fusion and summary experiments, 2026-11-22.** These ran on the rebuilt database (D, the **kept** row above) with a third judged set, `testdata/retrieval_fresh.json`: 23 queries on new themes, written before any ranking ran. Its candidates were pooled from the top 10 of every variant below, on both builds, so none of them is favoured. The ideas came from the "a doc must be in both lists" effect: with k = 60 and 50 candidates per list, the weakest doc in both lists (2/110 = 0.0182) outranks the best doc in one list only (1/61 = 0.0164).

| nDCG@10 (key@5) | tuning | held-out | fresh |
|---|---|---|---|
| shipped fusion (50 + 50, equal weights) | 0.71 (0.71) | 0.48 (0.83) | 0.49 (0.74) |
| pools of 100 per retriever | 0.68 (0.64) | 0.47 (0.83) | 0.49 (0.74) |
| BM25 topped up with any-word (OR) hits when every-word finds < 10 | 0.63 (0.64) | 0.50 (0.89) | 0.51 (0.78) |
| weighted RRF, BM25 × 0.75 | 0.69 (0.79) | 0.53 (0.89) | 0.54 (0.83) |
| weighted RRF, vector × 0.75 | 0.65 (0.64) | 0.42 (0.78) | 0.44 (0.61) |
| BM25 × 0.75 + OR top-up | 0.63 (0.71) | 0.53 (0.94) | 0.55 (0.83) |
| vector only | 0.59 (0.71) | 0.53 (0.72) | 0.55 (0.87) |
| BM25 only | 0.36 (0.29) | 0.24 (0.33) | 0.27 (0.48) |
| build E2 (question-style summaries on ~700 wiki/docs/tools/video docs), shipped fusion | 0.70 (0.71) | 0.48 (0.83) | 0.49 (0.74) |

- **Larger pools** change nothing on the fresh set and lose on tuning.
- **OR top-up** helps where every-word BM25 finds little ("power switch turns on perks and doors" 0.47 → 0.77) and breaks queries it already served ("sound alias plays silently" 0.82 → 0.25): the extra BM25 hits become two-list docs and push out vector-only answers.
- **BM25 × 0.75** is the only change that raises key@5 on all three sets. Its nDCG gains on held-out and fresh (+0.05) outweigh the loss on tuning (−0.02). With it, a doc ranked first by the vector alone (0.0164) beats the weakest two-list doc (0.0159); a BM25-only doc still cannot.
- **Question-style summaries (E2)** neither help nor hurt here. The first pilot, with long identifier-heavy summaries, lost 0.07 key@5 on tuning.

**Chunk sizes, 2026-11-22 (later).** The vector reads only a chunk's first 128 tokens, and the title + summary head takes ~48 of them, so 40–70% of the prose in 700-character chunks reached no vector. Three re-chunkings of build E2 (same summaries, only the vectors changed) were tested: 300-character chunks (871k vectors; 79% still overflow 128 tokens, because code is token-dense), token-sized chunks that fit 128 whole with the head on each (1.50M vectors, every body token embedded), and a head-only vector plus title-prefixed token-sized chunks (854k). The top 10 of each was pooled into all three sets. The 251 never-judged docs among them were judged blind, which added 134 grades. Every score below and from now on uses the extended grades. Paired per-query nDCG@10 against E2, BM25 × 0.75 fusion:

| variant | vectors | mean change | 95% bootstrap interval |
|---|---|---|---|
| token-sized chunks | 1.50M | +0.017 | −0.010 to +0.045 |
| 300-character chunks | 871k | +0.016 | −0.007 to +0.039 |
| head-only vector + title-prefixed chunks | 854k | −0.007 | −0.043 to +0.028 |

- **Not shipped.** The gain stays inside the noise of 55 queries (22 better, 17 worse), and key@5 does not rise. The full-coverage build costs 2.2 GB of in-memory vectors instead of 663 MB, 84 ms per vector rank instead of 36, and a 5.7 GB database instead of 3.6 GB. Revisit with int8 vectors, which would make the memory cost neutral, and with more queries.
- **The head matters more than the body.** Taking the summary off the body chunks erased the gain.
- **Pool bias was large.** Before the blind judging, the head-only variant looked 0.06 worse on the fresh set. "Setting Up Water" had never been judged for `water volume swimming`, because no 128-token ranking put it in a top 10.
- **Extended-grade baseline**, E2 (and E3: E2 with the 20 binary `.tga` bodies dropped and 197 missing summaries filled, identical scores), as nDCG@10 (key@5):

  | fusion | tuning | held-out | fresh |
  |---|---|---|---|
  | shipped | 0.67 (0.71) | 0.47 (0.83) | 0.48 (0.74) |
  | BM25 × 0.75 | 0.66 (0.79) | 0.51 (0.89) | 0.52 (0.78) |
