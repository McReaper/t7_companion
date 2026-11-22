package store

import (
	"context"
	"fmt"
	"math"
	"testing"
)

// At equal weights a document last in both lists outranked the vector's first
// hit alone; with BM25 at bm25Weight the vector's first hit wins.
func TestHybridFusionWeighsBM25(t *testing.T) {
	bm, vec := make([]rankItem, Pool), make([]rankItem, Pool)
	for i := range bm {
		bm[i] = rankItem{docID: fmt.Sprintf("b%d", i)}
		vec[i] = rankItem{docID: fmt.Sprintf("v%d", i)}
	}
	bm[Pool-1].docID, vec[Pool-1].docID = "both", "both"

	equal, _ := rrfFuse(ranking{bm, 1}, ranking{vec, 1})
	if equal["both"] <= equal["v0"] {
		t.Fatalf("equal weights: last in both lists %.5f should beat the vector's first %.5f", equal["both"], equal["v0"])
	}
	weighted, _ := rrfFuse(ranking{bm, bm25Weight}, ranking{vec, 1})
	if weighted["v0"] <= weighted["both"] {
		t.Fatalf("BM25 x %.2f: the vector's first %.5f should beat last in both lists %.5f", bm25Weight, weighted["v0"], weighted["both"])
	}
	if weighted["b0"] >= weighted["v0"] {
		t.Fatalf("BM25 x %.2f: the vector's first %.5f should beat BM25's first alone %.5f", bm25Weight, weighted["v0"], weighted["b0"])
	}
}

// A keyword-only search (t7kb search --bm25 passes no query vector) has nothing
// to balance: its scores stay plain RRF, 1 / (RRFK + rank).
func TestKeywordOnlySearchKeepsPlainRRFScores(t *testing.T) {
	c := corpus(t, 200, 1)
	s, err := Open(c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for name, search := range map[string]func() ([]Hit, error){
		"hybrid, no vector": func() ([]Hit, error) { return s.SearchHybrid(ctx, "zombie", nil, 10) },
		"any word":          func() ([]Hit, error) { return s.SearchAnyWord(ctx, "zombie nothinghere", 10) },
	} {
		hits, err := search()
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatalf("%s: no hits for a word the corpus holds", name)
		}
		for i, h := range hits {
			if want := 1.0 / float64(RRFK+i+1); math.Abs(h.Score-want) > 1e-12 {
				t.Fatalf("%s: hit %d scores %.6f, want plain RRF %.6f", name, i+1, h.Score, want)
			}
		}
	}
}
