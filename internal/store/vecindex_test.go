package store

import (
	"context"
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
