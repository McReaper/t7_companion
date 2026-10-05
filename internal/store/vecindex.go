package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"runtime"
	"sort"
	"sync"
)

// vecIndex holds every chunk vector in memory, for a process that searches many
// times (the MCP server): a search then scores memory instead of reading ~380k
// SQLite rows. It holds the same float32 vectors, scored with the same
// arithmetic, chunk by chunk in the order the scan reads them, so a search
// returns what the scan returns. A full corpus takes ~580 MB.
type vecIndex struct {
	dim    int
	vecs   []float32 // chunk i is vecs[i*dim : (i+1)*dim]
	rowid  []int64   // chunk i's rowid, to read its text for a snippet
	doc    []int32   // chunk i's document, an index into docs
	docs   []string
	docSrc []string // sourceOf(docs[d]), for the source filter
}

// loadVecIndex reads the embeddings table once. It returns nil when the table
// is absent or mixes dimensions: the scan's per-chunk dimension check then
// stays the one rule.
func loadVecIndex(ctx context.Context, db *sql.DB) (*vecIndex, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM embeddings`).Scan(&n); err != nil {
		return nil, nil // no embeddings table: vector search is off anyway
	}
	rows, err := db.QueryContext(ctx, `SELECT rowid, doc_id, embedding FROM embeddings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ix := &vecIndex{rowid: make([]int64, 0, n), doc: make([]int32, 0, n)}
	docAt := map[string]int32{}
	for rows.Next() {
		var rowid int64
		var docID, blob sql.RawBytes
		if err := rows.Scan(&rowid, &docID, &blob); err != nil {
			return nil, err
		}
		if ix.dim == 0 {
			ix.dim = len(blob) / 4
			ix.vecs = make([]float32, 0, n*ix.dim)
		}
		if len(blob) != 4*ix.dim {
			return nil, nil // mixed dimensions: leave it to the scan
		}
		d, ok := docAt[string(docID)]
		if !ok {
			d = int32(len(ix.docs))
			id := string(docID)
			docAt[id] = d
			ix.docs = append(ix.docs, id)
			ix.docSrc = append(ix.docSrc, sourceOf(id))
		}
		for i := 0; i < ix.dim; i++ {
			ix.vecs = append(ix.vecs, math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:])))
		}
		ix.rowid = append(ix.rowid, rowid)
		ix.doc = append(ix.doc, d)
	}
	return ix, rows.Err()
}

// rank scores every chunk on every CPU, then keeps each document's best chunk
// (the first of equal scores, in scan order) and the limit best documents.
func (ix *vecIndex) rank(qvec []float32, limit int, only map[string]bool) []scoredDoc {
	if len(qvec) != ix.dim {
		return nil // the scan skips every chunk of another dimension
	}
	scores := ix.scores(qvec)
	best := make([]int32, len(ix.docs)) // best chunk + 1 per document; 0 = none yet
	for c, s := range scores {
		d := ix.doc[c]
		if len(only) > 0 && !only[ix.docSrc[d]] {
			continue
		}
		if b := best[d]; b == 0 || s > scores[b-1] {
			best[d] = int32(c) + 1
		}
	}
	docs := make([]scoredDoc, 0, len(ix.docs))
	for d, b := range best {
		if b > 0 {
			docs = append(docs, scoredDoc{docID: ix.docs[d], score: scores[b-1], chunk: ix.rowid[b-1]})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].score > docs[j].score })
	if limit > 0 && len(docs) > limit {
		docs = docs[:limit]
	}
	return docs
}

// scores is dot(qvec, chunk) for every chunk, split across the CPUs.
func (ix *vecIndex) scores(qvec []float32) []float64 {
	n := len(ix.rowid)
	out := make([]float64, n)
	workers := runtime.NumCPU()
	per := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for lo := 0; lo < n; lo += per {
		hi := min(lo+per, n)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := lo; c < hi; c++ {
				out[c] = dot(qvec, ix.vecs[c*ix.dim:(c+1)*ix.dim])
			}
		}()
	}
	wg.Wait()
	return out
}

// EnableVectorIndex makes vector search score an in-memory copy of the
// vectors, loaded on first use (or by WarmVectors). For a long-lived process:
// a one-shot search is faster with the plain scan than with loading the index.
func (s *Store) EnableVectorIndex() { s.vecOn = true }

// WarmVectors loads the vector index in the background.
func (s *Store) WarmVectors() {
	if s.vecOn {
		go s.vectorIndex()
	}
}

// vectorIndex returns the in-memory index, loading it once; nil means scan.
func (s *Store) vectorIndex() *vecIndex {
	s.vecOnce.Do(func() {
		s.vecIx, _ = loadVecIndex(context.Background(), s.db) // on failure, the scan still answers
	})
	return s.vecIx
}
