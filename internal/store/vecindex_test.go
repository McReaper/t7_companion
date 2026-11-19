package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

// The in-memory index must rank exactly as the SQLite scan: same documents,
// same order, same snippets, with and without a source filter.
func TestVectorIndexMatchesScan(t *testing.T) {
	c := corpus(t, 1000, 4)
	scan, err := Open(c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer scan.Close()
	mem, err := Open(c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	mem.EnableVectorIndex()
	if mem.vectorIndex() == nil {
		t.Fatal("the index did not load")
	}

	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))
	for q := range 20 {
		qvec := unitVec(rng)
		for _, only := range []map[string]bool{nil, {"bench": true}, {"other": true}} {
			want, err := scan.vectorRank(ctx, qvec, Pool, only)
			if err != nil {
				t.Fatal(err)
			}
			got, err := mem.vectorRank(ctx, qvec, Pool, only)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("query %d, filter %v: the index ranks differently from the scan", q, only)
			}
		}
	}
	if got, _ := mem.vectorRank(ctx, make([]float32, 10), Pool, nil); len(got) != 0 {
		t.Error("a query of another dimension finds nothing, as with the scan")
	}
}

func BenchmarkVectorIndexRank(b *testing.B) {
	for _, sz := range benchSizes {
		st, c := openBench(b, sz.docs, sz.chunks)
		st.EnableVectorIndex()
		st.vectorIndex()
		b.Run(fmt.Sprintf("chunks=%d", sz.docs*sz.chunks), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := st.vectorRank(context.Background(), c.qvec, Pool, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The index loads on the first search, not before; ReleaseVectors drops it,
// says it did, and the next search loads it again and ranks as before.
func TestReleasedVectorsLoadAgainOnTheNextSearch(t *testing.T) {
	c := corpus(t, 100, 2)
	s, err := Open(c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	qvec := unitVec(rand.New(rand.NewSource(3)))

	plain, err := s.vectorRank(ctx, qvec, Pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.vecIx != nil {
		t.Fatal("loaded without EnableVectorIndex")
	}
	s.EnableVectorIndex()
	if s.vecIx != nil {
		t.Fatal("loaded before any search")
	}
	first, err := s.vectorRank(ctx, qvec, Pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.vecIx == nil {
		t.Fatal("the first search didn't load the index")
	}
	if !s.ReleaseVectors() || s.vecIx != nil {
		t.Fatal("ReleaseVectors kept the index, or said there was none")
	}
	if s.ReleaseVectors() {
		t.Error("a second ReleaseVectors said it dropped an index")
	}
	again, err := s.vectorRank(ctx, qvec, Pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.vecIx == nil {
		t.Fatal("the search after a release didn't load the index again")
	}
	if !reflect.DeepEqual(first, plain) || !reflect.DeepEqual(again, plain) {
		t.Error("the index ranks differently from the scan, before or after a release")
	}
}

// A store whose vectors can't load (no embeddings table) answers with the
// scan, and doesn't retry the load on each search until it is released.
func TestVectorIndexWithoutEmbeddingsLoadsOnce(t *testing.T) {
	c := corpus(t, 10, 1)
	rw, err := sql.Open("sqlite", "file:"+c.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rw.Exec(`DROP TABLE embeddings`); err != nil {
		t.Fatal(err)
	}
	_ = rw.Close()
	s, err := Open(c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.EnableVectorIndex()
	if s.vectorIndex() != nil || !s.vecLoaded {
		t.Fatal("no embeddings: want no index, and the load marked as tried")
	}
	if s.ReleaseVectors() {
		t.Error("ReleaseVectors said it dropped an index that never loaded")
	}
	if s.vecLoaded {
		t.Error("ReleaseVectors left the load marked as tried")
	}
}
