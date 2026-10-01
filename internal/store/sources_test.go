package store

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
)

// sourcesDB is a small corpus over several sources; every doc mentions
// "clientfield" so BM25 alone could return any of them.
func sourcesDB(t *testing.T) (*Store, []float32) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
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
			t.Fatal(err)
		}
	}
	rng := rand.New(rand.NewSource(3))
	for _, src := range []string{"gscode-api", "discord-bo3modtools", "wiki-modme", "wiki-t7", "forums-ugx"} {
		for i := 0; i < 5; i++ {
			id := fmt.Sprintf("%s::d%d", src, i)
			if _, err := db.Exec(`INSERT INTO documents(doc_id,source,title,body,reliability) VALUES(?,?,?,?,?)`,
				id, src, "clientfield "+src, "how to register a clientfield", 0.5); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO embeddings(doc_id,chunk_index,chunk_text,embedding) VALUES(?,0,?,?)`,
				id, "chunk", encode(unitVec(rng))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT INTO docs_fts(docs_fts) VALUES('rebuild')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, unitVec(rng)
}

func TestResolveSources(t *testing.T) {
	st, _ := sourcesDB(t)
	ctx := context.Background()
	for in, want := range map[string]string{
		"api":            "gscode-api",
		"wiki":           "wiki-modme,wiki-t7", // a prefix group takes every wiki
		"API, discord":   "discord-bo3modtools,gscode-api",
		"wiki-t7":        "wiki-t7", // an exact source name
		"forums,wiki-t7": "forums-ugx,wiki-t7",
	} {
		got, err := st.ResolveSources(ctx, strings.Split(in, ","))
		if err != nil || strings.Join(got, ",") != want {
			t.Errorf("%q -> %v %v, want %s", in, got, err, want)
		}
	}
	if got, err := st.ResolveSources(ctx, nil); got != nil || err != nil {
		t.Errorf("no names means no filter: %v %v", got, err)
	}
	_, err := st.ResolveSources(ctx, []string{"apis"})
	if err == nil || !strings.Contains(err.Error(), "api, discord, docs") || !strings.Contains(err.Error(), "gscode-api") {
		t.Errorf("a typo lists the groups and the sources: %v", err)
	}
}

func TestSearchHybridFiltersBothRetrievers(t *testing.T) {
	st, qvec := sourcesDB(t)
	ctx := context.Background()
	only, err := st.ResolveSources(ctx, []string{"wiki"})
	if err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() ([]Hit, error){
		"hybrid":      func() ([]Hit, error) { return st.SearchHybrid(ctx, "clientfield", qvec, 20, only...) },
		"bm25 only":   func() ([]Hit, error) { return st.SearchHybrid(ctx, "clientfield", nil, 20, only...) },
		"vector only": func() ([]Hit, error) { return st.SearchHybrid(ctx, "zzznotaword", qvec, 20, only...) },
	} {
		hits, err := run()
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 10 {
			t.Errorf("%s: want the 10 wiki docs, got %d", name, len(hits))
		}
		for _, h := range hits {
			if !strings.HasPrefix(h.DocID, "wiki-") {
				t.Errorf("%s: %s is outside the filter", name, h.DocID)
			}
		}
	}
	all, _ := st.SearchHybrid(ctx, "clientfield", qvec, 50)
	if len(all) != 25 {
		t.Errorf("without a filter every source is searched: %d", len(all))
	}
}
