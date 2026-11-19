# t7kb.db data model

A single SQLite file, queried with hybrid retrieval: FTS5 BM25 + vector cosine, fused with Reciprocal Rank Fusion. Three objects.

## `documents` — one row per entry

| column | notes |
|---|---|
| `doc_id` | `<source>::<local-id>`, primary key |
| `source` | e.g. `gscode-api`, `source-scripts`, `discord-bo3modtools` |
| `title`, `summary`, `body` | content; `body` is the full text |
| `url` | upstream link when public (attribution) |
| `content_path` | local-install path when applicable |
| `themes`, `metadata` | JSON arrays of tags |
| `source_type` | `api`, `script`, `transcript`, … |
| `reliability` | `[0, 1]`, soft ranking tiebreak |

## `docs_fts`

FTS5 virtual table over `title + summary + body` (external-content mode over `documents`), providing BM25 ranking.

## `embeddings` — precomputed vectors, one row per chunk

| column | notes |
|---|---|
| `doc_id` | references `documents` |
| `chunk_index` | chunk number within the doc |
| `chunk_text` | the chunk's text |
| `embedding` | BLOB: 384 little-endian float32, L2-normalized (all-MiniLM-L6-v2) |

At query time the tool embeds the query with the **same** model, takes the cosine (a dot product, since vectors are normalized) against `embedding`, keeps the best chunk per `doc_id`, and RRF-fuses that ranking with BM25.
