package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Synthetic-corpus benchmarks: no real t7kb.db and no embedding model needed,
// so they run anywhere (CI included). Vectors are random unit vectors of the
// real dimension (384); bodies are drawn from a small BO3 vocabulary so FTS5 has
// realistic postings. Run: go test ./internal/store -run '^$' -bench . -benchmem
//
// The real corpus is ~66k documents and ~350k chunks; sizes here scale towards
// it without making the fixture slow to build.

const benchDim = 384

var benchWords = strings.Fields(`zombie spawner perk machine clientfield register lua widget linker error
	scriptparsetree material surfacetype wallbuy cost platform elevator notetrack xanim fog volume
	easter egg step bsp leak sealing hintstring color radiant prefab brush patch light probe sun
	vision set reverb ambient sound alias weapon attachment camo model export gdt ape image texture`)

type benchCorpus struct {
	path string
	qvec []float32
}

var (
	benchMu      sync.Mutex
	benchCorpora = map[string]benchCorpus{}
)

// corpus builds (once per size, per process) a db of docs documents with chunks
// embeddings each.
func corpus(b *testing.B, docs, chunks int) benchCorpus {
	b.Helper()
	key := fmt.Sprintf("%dx%d", docs, chunks)
	benchMu.Lock()
	defer benchMu.Unlock()
	if c, ok := benchCorpora[key]; ok {
		return c
	}
	dir, err := tempDir()
	if err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(dir, key+".db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	for _, s := range []string{
		`CREATE TABLE documents (doc_id TEXT PRIMARY KEY, source TEXT NOT NULL, title TEXT NOT NULL,
			summary TEXT, body TEXT NOT NULL, url TEXT, content_path TEXT, themes TEXT, metadata TEXT,
			source_type TEXT, reliability REAL NOT NULL)`,
		`CREATE VIRTUAL TABLE docs_fts USING fts5(doc_id UNINDEXED, title, summary, body,
			content='documents', content_rowid='rowid', tokenize='unicode61 remove_diacritics 2')`,
		`CREATE TABLE embeddings (doc_id TEXT NOT NULL, chunk_index INTEGER NOT NULL DEFAULT 0,
			chunk_text TEXT, embedding BLOB NOT NULL, PRIMARY KEY(doc_id, chunk_index))`,
	} {
		if _, err := db.Exec(s); err != nil {
			b.Fatal(err)
		}
	}
	rng := rand.New(rand.NewSource(1))
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	for d := 0; d < docs; d++ {
		id := fmt.Sprintf("bench::doc%06d", d)
		body := words(rng, 200) // ~1.2k chars, the corpus median
		if _, err := tx.Exec(`INSERT INTO documents(doc_id,source,title,body,reliability) VALUES(?,?,?,?,?)`,
			id, "bench", words(rng, 6), body, rng.Float64()); err != nil {
			b.Fatal(err)
		}
		for c := 0; c < chunks; c++ {
			if _, err := tx.Exec(`INSERT INTO embeddings(doc_id,chunk_index,chunk_text,embedding) VALUES(?,?,?,?)`,
				id, c, words(rng, 150), encode(unitVec(rng))); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO docs_fts(docs_fts) VALUES('rebuild')`); err != nil {
		b.Fatal(err)
	}
	c := benchCorpus{path: path, qvec: unitVec(rng)}
	benchCorpora[key] = c
	return c
}

// tempDir is a per-process directory for the fixtures (b.TempDir would rebuild
// them for every benchmark).
var tempDir = sync.OnceValues(func() (string, error) { return mkTemp() })

func words(rng *rand.Rand, n int) string {
	w := make([]string, n)
	for i := range w {
		w[i] = benchWords[rng.Intn(len(benchWords))]
	}
	return strings.Join(w, " ")
}

func unitVec(rng *rand.Rand) []float32 {
	v := make([]float32, benchDim)
	var n float64
	for i := range v {
		v[i] = float32(rng.NormFloat64())
		n += float64(v[i]) * float64(v[i])
	}
	n = math.Sqrt(n)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v
}

func encode(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

// sizes are (documents, chunks per document).
var benchSizes = []struct{ docs, chunks int }{{1000, 4}, {10000, 4}}

func openBench(b *testing.B, docs, chunks int) (*Store, benchCorpus) {
	b.Helper()
	c := corpus(b, docs, chunks)
	st, err := Open(c.path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	return st, c
}

func BenchmarkBM25Rank(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(fmt.Sprintf("docs=%d", sz.docs), func(b *testing.B) {
			st, _ := openBench(b, sz.docs, sz.chunks)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := st.bm25Rank(ctx, "zombie spawner perk", Pool); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVectorRank is the full scan of the embeddings table every query does.
func BenchmarkVectorRank(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(fmt.Sprintf("chunks=%d", sz.docs*sz.chunks), func(b *testing.B) {
			st, c := openBench(b, sz.docs, sz.chunks)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := st.vectorRank(ctx, c.qvec, Pool); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(sz.docs*sz.chunks), "ns/chunk")
		})
	}
}

// BenchmarkSearchHybrid is one search end to end, minus the query embedding
// (measured in internal/embed): BM25 + vector + RRF + metadata.
func BenchmarkSearchHybrid(b *testing.B) {
	for _, sz := range benchSizes {
		b.Run(fmt.Sprintf("docs=%d", sz.docs), func(b *testing.B) {
			st, c := openBench(b, sz.docs, sz.chunks)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := st.SearchHybrid(ctx, "zombie spawner perk", c.qvec, 10); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkGet(b *testing.B) {
	st, _ := openBench(b, benchSizes[len(benchSizes)-1].docs, benchSizes[len(benchSizes)-1].chunks)
	ctx := context.Background()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := st.Get(ctx, fmt.Sprintf("bench::doc%06d", i%1000)); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkRRFFuse(b *testing.B) {
	rng := rand.New(rand.NewSource(2))
	mk := func() []rankItem {
		out := make([]rankItem, Pool)
		for i := range out {
			out[i] = rankItem{docID: fmt.Sprintf("d%d", rng.Intn(Pool*2)), snippet: "s"}
		}
		return out
	}
	a, c := mk(), mk()
	b.ReportAllocs()
	for b.Loop() {
		rrfFuse(a, c)
	}
}

var benchDir string

func mkTemp() (string, error) {
	d, err := os.MkdirTemp("", "t7kb-bench-")
	benchDir = d
	return d, err
}

// TestMain removes the benchmark fixtures (~100 MB at 10k docs) when the run ends.
func TestMain(m *testing.M) {
	code := m.Run()
	if benchDir != "" {
		_ = os.RemoveAll(benchDir)
	}
	os.Exit(code)
}
