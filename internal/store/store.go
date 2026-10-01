// Package store opens a t7kb.db and serves hybrid retrieval over it:
// FTS5 BM25 + cosine over precomputed embeddings, fused with RRF.
package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo); FTS5 compiled in
)

// RRFK is the Reciprocal Rank Fusion constant (matches query_kb.py).
const RRFK = 60

// Pool is the per-retriever candidate count fused before truncating to limit.
const Pool = 50

// titleWeight: a BM25 match in the title counts this much over one in the
// body (the summary half as much). Reference pages are short with precise
// titles ("PlayFXOnTag (GSC)", "Wallbuy 0 Fix") and lost to long threads and
// scripts repeating the words. Scored against internal/cli TestRetrievalQuality
// (docs/benchmarks.md): it brings one more tuning query's key into the top 5
// and leaves the blind held-out set unchanged.
//
// Reliability stays a tiebreak only. Weighting the fused score by it
// (1 + w*(reliability - 0.5), w = 0.25) won on the tuning set — which leans on
// "what is the API for…" questions — and lost on the held-out one (key@5 0.83
// -> 0.78), where the best answers are often community threads it pushed down.
const titleWeight = 10.0

// Store is a read-only handle to a t7kb.db.
type Store struct {
	db *sql.DB

	srcOnce sync.Once // sources(): the distinct sources, read once
	srcList []string
	srcErr  error
}

// Hit is one fused search result.
type Hit struct {
	DocID       string
	Title       string
	Source      string
	Score       float64 // fused RRF score (higher = better)
	Reliability float64
	Snippet     string
}

// Doc is a full document body.
type Doc struct {
	DocID       string
	Source      string
	Title       string
	Summary     string
	Body        string
	URL         string
	ContentPath string
	Themes      string
	Metadata    string
	SourceType  string
	Reliability float64
}

// Open opens the database at path in read-only mode.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close releases the handle.
func (s *Store) Close() error { return s.db.Close() }

// SearchHybrid fuses BM25 and vector rankings with RRF, applies reliability as
// a tiebreak, and returns the top `limit` hits. qvec may be nil/empty (or
// a different dimension than the db) — then it's BM25-only.
//
// sources, if given, restricts the search to those sources (exact names, see
// ResolveSources): both retrievers then rank within them, rather than the top
// of the whole corpus being filtered afterwards.
func (s *Store) SearchHybrid(ctx context.Context, query string, qvec []float32, limit int, sources ...string) ([]Hit, error) {
	only := sourceSet(sources)
	bm25, err := s.bm25Rank(ctx, query, Pool, only)
	if err != nil {
		return nil, err
	}
	vec, err := s.vectorRank(ctx, qvec, Pool, only)
	if err != nil {
		return nil, err
	}

	fused, snippet := rrfFuse(bm25, vec)
	if len(fused) == 0 {
		return nil, nil
	}

	docIDs := make([]string, 0, len(fused))
	for d := range fused {
		docIDs = append(docIDs, d)
	}
	rel, title, source, err := s.meta(ctx, docIDs)
	if err != nil {
		return nil, err
	}
	sortByFusedScore(docIDs, fused, rel)
	if limit > 0 && len(docIDs) > limit {
		docIDs = docIDs[:limit]
	}

	hits := make([]Hit, len(docIDs))
	for i, d := range docIDs {
		hits[i] = Hit{DocID: d, Title: title[d], Source: source[d], Score: fused[d], Reliability: rel[d], Snippet: snippet[d]}
	}
	return hits, nil
}

// rankItem is one retriever's ranked candidate (already in rank order).
type rankItem struct {
	docID   string
	snippet string
}

// rrfFuse combines ranked lists by Reciprocal Rank Fusion and keeps the first
// non-empty snippet seen per doc.
func rrfFuse(rankings ...[]rankItem) (fused map[string]float64, snippet map[string]string) {
	fused = make(map[string]float64)
	snippet = make(map[string]string)
	for _, items := range rankings {
		for rank, it := range items {
			fused[it.docID] += 1.0 / float64(RRFK+rank+1)
			if snippet[it.docID] == "" && it.snippet != "" {
				snippet[it.docID] = it.snippet
			}
		}
	}
	return fused, snippet
}

// sortByFusedScore orders docIDs by fused score, with reliability as tiebreak.
func sortByFusedScore(docIDs []string, fused, rel map[string]float64) {
	sort.Slice(docIDs, func(i, j int) bool {
		di, dj := docIDs[i], docIDs[j]
		switch {
		case fused[di] != fused[dj]:
			return fused[di] > fused[dj]
		case rel[di] != rel[dj]:
			return rel[di] > rel[dj]
		default:
			return di < dj
		}
	})
}

var ftsToken = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// ftsQuery turns free text into a safe FTS5 MATCH expression: each word becomes
// a quoted term, joined implicitly (AND). Empty if there are no usable tokens.
func ftsQuery(q string) string {
	toks := ftsToken.FindAllString(q, -1)
	if len(toks) == 0 {
		return ""
	}
	for i, t := range toks {
		toks[i] = `"` + t + `"`
	}
	return strings.Join(toks, " ")
}

// bm25Rank returns up to limit doc_ids ranked by FTS5 BM25, with a snippet.
func (s *Store) bm25Rank(ctx context.Context, query string, limit int, only map[string]bool) ([]rankItem, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	args := []any{match}
	filter := ""
	if len(only) > 0 {
		marks := make([]string, 0, len(only))
		for src := range only {
			marks = append(marks, "?")
			args = append(args, src)
		}
		filter = "AND d.source IN (" + strings.Join(marks, ",") + ")"
	}
	q := `
		SELECT d.doc_id, snippet(docs_fts, 3, '', '', ' … ', 12) AS snip
		FROM docs_fts
		JOIN documents d ON d.rowid = docs_fts.rowid
		WHERE docs_fts MATCH ? ` + filter + `
		ORDER BY bm25(docs_fts, 0.0, ?, ?, 1.0)
		LIMIT ?`
	args = append(args, titleWeight, titleWeight/2, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}
	defer rows.Close()

	var out []rankItem
	for rows.Next() {
		var it rankItem
		var snip sql.NullString
		if err := rows.Scan(&it.docID, &snip); err != nil {
			return nil, err
		}
		it.snippet = snip.String
		out = append(out, it)
	}
	return out, rows.Err()
}

// scoredDoc is a doc with its best vector score and that chunk's snippet.
type scoredDoc struct {
	docID   string
	score   float64
	snippet string
}

// vectorRank streams the embeddings table, scoring each chunk by cosine (dot,
// since vectors are L2-normalized) against qvec and keeping the best chunk per
// doc. Chunks whose dimension differs from qvec are skipped — so the tool
// degrades to BM25-only against a db embedded with a different model.
func (s *Store) vectorRank(ctx context.Context, qvec []float32, limit int, only map[string]bool) ([]rankItem, error) {
	if len(qvec) == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT doc_id, chunk_text, embedding FROM embeddings`)
	if err != nil {
		return nil, nil // no embeddings table → vector disabled, not fatal
	}
	defer rows.Close()

	best := make(map[string]scoredDoc)
	for rows.Next() {
		var docID string
		var chunkText sql.NullString
		var blob []byte
		if err := rows.Scan(&docID, &chunkText, &blob); err != nil {
			return nil, err
		}
		if len(only) > 0 && !only[sourceOf(docID)] {
			continue
		}
		v := decodeVec(blob)
		if len(v) != len(qvec) {
			continue
		}
		score := dot(qvec, v)
		if cur, ok := best[docID]; !ok || score > cur.score {
			best[docID] = scoredDoc{docID, score, snippetOf(chunkText.String)}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return topRankItems(best, limit), nil
}

// topRankItems sorts scored docs by score (descending), truncates to limit, and
// projects to rankItems.
func topRankItems(byDoc map[string]scoredDoc, limit int) []rankItem {
	docs := make([]scoredDoc, 0, len(byDoc))
	for _, d := range byDoc {
		docs = append(docs, d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].score > docs[j].score })
	if limit > 0 && len(docs) > limit {
		docs = docs[:limit]
	}
	items := make([]rankItem, len(docs))
	for i, d := range docs {
		items[i] = rankItem{docID: d.docID, snippet: d.snippet}
	}
	return items
}

func dot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func decodeVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func snippetOf(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:160] + " …"
	}
	return s
}

// meta fetches title/source/reliability for a set of doc_ids.
func (s *Store) meta(ctx context.Context, docIDs []string) (rel map[string]float64, title, source map[string]string, err error) {
	rel = make(map[string]float64)
	title = make(map[string]string)
	source = make(map[string]string)
	if len(docIDs) == 0 {
		return rel, title, source, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(docIDs)), ",")
	args := make([]any, len(docIDs))
	for i, d := range docIDs {
		args[i] = d
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT doc_id, title, source, reliability FROM documents WHERE doc_id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d, t, src string
		var r float64
		if err := rows.Scan(&d, &t, &src, &r); err != nil {
			return nil, nil, nil, err
		}
		title[d], source[d], rel[d] = t, src, r
	}
	return rel, title, source, rows.Err()
}

// Get returns the full document for a doc_id, or (nil, nil) if absent.
func (s *Store) Get(ctx context.Context, docID string) (*Doc, error) {
	const q = `
		SELECT doc_id, source, title, summary, body, url, content_path,
		       themes, metadata, source_type, reliability
		FROM documents WHERE doc_id = ?`
	var d Doc
	var summary, url, contentPath, themes, metadata, sourceType sql.NullString
	err := s.db.QueryRowContext(ctx, q, docID).Scan(
		&d.DocID, &d.Source, &d.Title, &summary, &d.Body, &url, &contentPath,
		&themes, &metadata, &sourceType, &d.Reliability,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", docID, err)
	}
	d.Summary = summary.String
	d.URL = url.String
	d.ContentPath = contentPath.String
	d.Themes = themes.String
	d.Metadata = metadata.String
	d.SourceType = sourceType.String
	return &d, nil
}
