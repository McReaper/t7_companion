package store_test

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

// doc topics that are semantically distinct so vector ranking is unambiguous.
var docs = []struct{ id, title, body string }{
	{"x::respawn", "Player respawn points in co-op", "Where a downed player re-enters the map after going down in co-op zombies."},
	{"x::weapon", "Custom weapon attachments", "Use weaponfull instead of weapon in the zone to load attachment unique files."},
	{"x::light", "Lighting and atmosphere", "Reflection probes, vision sets and sun volumes control the lighting mood of a map."},
	{"x::clientfield", "Clientfield networking", "How many bits a clientfield needs to transmit a float over the network."},
}

func encodeVec(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func buildDB(t *testing.T, path string, emb *embed.Embedder) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE documents (doc_id TEXT PRIMARY KEY, source TEXT NOT NULL, title TEXT NOT NULL,
			summary TEXT, body TEXT NOT NULL, url TEXT, content_path TEXT, themes TEXT, metadata TEXT,
			source_type TEXT, reliability REAL NOT NULL)`,
		`CREATE VIRTUAL TABLE docs_fts USING fts5(doc_id UNINDEXED, title, summary, body,
			content='documents', content_rowid='rowid', tokenize='unicode61 remove_diacritics 2')`,
		`CREATE TABLE embeddings (doc_id TEXT NOT NULL, chunk_index INTEGER NOT NULL DEFAULT 0,
			chunk_text TEXT, embedding BLOB NOT NULL, PRIMARY KEY(doc_id, chunk_index))`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	for _, d := range docs {
		if _, err := db.Exec(
			`INSERT INTO documents(doc_id,source,title,body,reliability) VALUES(?,?,?,?,?)`,
			d.id, "x", d.title, d.body, 0.8); err != nil {
			t.Fatal(err)
		}
		vec, err := emb.Embed(d.title + " " + d.body)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			`INSERT INTO embeddings(doc_id,chunk_index,chunk_text,embedding) VALUES(?,0,?,?)`,
			d.id, d.body, encodeVec(vec)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO docs_fts(docs_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("fts rebuild: %v", err)
	}
}

func TestHybridSearch(t *testing.T) {
	emb, err := embed.New()
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	buildDB(t, path, emb)

	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	// Paraphrase query with NO shared keywords with the target doc body —
	// only vectors can connect "make zombies bring the player back" to the
	// respawn doc. Proves the vector path is doing real work.
	qvec, err := emb.Embed("make zombies bring the player back after they bleed out")
	if err != nil {
		t.Fatal(err)
	}
	hits, err := st.SearchHybrid(ctx, "make zombies bring the player back after they bleed out", qvec, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].DocID != "x::respawn" {
		t.Fatalf("vector ranking wrong: got %q first, want x::respawn\nhits: %+v", hits[0].DocID, hits)
	}

	// BM25 exact-term query still works (keyword "weaponfull").
	bm, err := st.SearchHybrid(ctx, "weaponfull attachment", nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(bm) == 0 || bm[0].DocID != "x::weapon" {
		t.Fatalf("bm25 ranking wrong: %+v", bm)
	}
}
