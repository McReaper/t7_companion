# The code part of search

Why search ranks the code and data files apart from the pages, and lists them as the last 3 of 10 results. Measured with `TestRetrievalQuality` on 2026-11-23. The judged sets and the builds named here (E3, F) are described in [retrieval-quality.md](retrieval-quality.md) and [retrieval-experiments.md](retrieval-experiments.md).

The four code sources (source-scripts, source-dump, source-workspace and defs-bo3) hold 15,206 docs and 53% of the vectors. They still had template summaries: `GSC script — namespace zm_playerhealth (19 fns): …`, `map — elevator.map`, `6 assets (3 xmodel, 3 material)`. Build F replaced them with question-style summaries, using E2's prompt (15,178 generated, 4 failed). The never-judged docs in every variant's top 10 were judged blind and added to the three sets (178 docs, 42 relevant, none key).

Counts over the 55 questions, with BM25 × 0.75. "Useful code" counts the questions with a code doc graded key or relevant among the 10 results:

| build, ranking | key in the top 5 | key among the 10 results | useful code among the 10 |
|---|---|---|---|
| E3, one list | 46 | 49 | 6 |
| F, one list | 44 | 49 | 11 |
| E3, code apart (7 pages, then 3 code files) | 44 | 49 | 18 |
| F, code apart (**shipped**) | 44 | 49 | 25 |

- **Why the new summaries lost in one list.** The loss is all on the vector side. Swapping the embeddings table between the two builds showed it: F's text with E3's vectors scores 46, and E3's text with F's vectors scores 44.
  - Every chunk carries its doc's title and summary. The templates had kept the code chunks away from plain-English questions, by accident.
  - With question-style summaries, the code docs in the vector's top 10 went from 31 to 45 over the 55 questions.
  - In the fused top 10, useful code docs went from 6 to 17, and the other code docs from 30 to 43. Those took the slots of tutorials and threads.
- **Ways to keep code in one list, all rejected** (key in the top 5):

  | variant | result |
  |---|---|
  | the summary on a code doc's first chunk only, its title alone on the others | 45: title + raw code still sits closer to the questions than the templates did |
  | E3's vectors, plus one title + summary vector per code doc | 44 |
  | code down-weighted in the vector list (RRF weight 0.5, or its rank ×2, ×3 or ×5) | 44 or 45 |
  | code dropped from the vector list | 46, but the useful code goes with it |
- **Code apart (shipped).** `store.Search` ranks the pages and the code files separately, each with the shipped fusion and from one pass over the vectors. It lists 7 pages, then 3 code files, and a part with fewer hits leaves its slots to the other. A source filter still ranks one list.
- **The first 5 results are now pages only.** Of the 46 questions that had a key document there:
  - `custom powerup drop` now finds its key, `_zm_powerups`, in the code part;
  - two lost a place to fusion shifts;
  - `weapon recoil and spread settings` gained one.

  Among the 10 results, nothing is lost.
- **BM25 weight within each part.** At × 0.5, 0.75 and 1: 43, 44 and 42 questions have a key in the top 5; 49 for all three among the 10 results; 26, 25 and 24 have useful code. 0.75 stays.
- **Caveat.** One judge graded the code docs, blind to which ranking proposed them. Read the useful-code counts as an order of magnitude.
- **Speed.** Over 165 searches with the vectors in memory, excluding the query embedding (p50 / p90): one list 53 / 124 ms, and the code part 75 / 288 ms, because BM25 runs once per part. Reading BM25 snippets for the shown hits only, instead of for every matching doc, brought these to 47 / 65 ms and 61 / 100 ms, with the same hits and snippets.
- **Baseline.** The full test on F with this search, as key@5 / key@10 / recall@10 / MRR / nDCG@10: tuning 0.64 / 0.86 / 0.48 / 1.00 / 0.64, held-out 0.83 / 0.94 / 0.34 / 0.92 / 0.49, fresh 0.87 / 0.87 / 0.35 / 0.84 / 0.48. That is 44 of 55 questions with a key document in the top 5, and 49 among the 10 results.
