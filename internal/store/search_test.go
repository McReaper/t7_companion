package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// splitDB holds pages (wiki, discord) and code files (source-scripts,
// defs-bo3). Every code file beats every page on both halves of the search:
// it repeats the query word more, and its vector is closer to the query
// vector (the first axis). In one list, the code fills the top.
func splitDB(t *testing.T, pages, code int) (*Store, []float32) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "split.db")
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
	add := func(src string, i int, body string, cos float64) {
		id := fmt.Sprintf("%s::d%02d", src, i)
		if _, err := db.Exec(`INSERT INTO documents(doc_id,source,title,body,reliability) VALUES(?,?,?,?,?)`,
			id, src, id, body, 0.5); err != nil {
			t.Fatal(err)
		}
		v := make([]float32, 8)
		v[0], v[1] = float32(cos), float32(math.Sqrt(1-cos*cos))
		if _, err := db.Exec(`INSERT INTO embeddings(doc_id,chunk_index,chunk_text,embedding) VALUES(?,0,?,?)`,
			id, "chunk of "+id, encode(v)); err != nil {
			t.Fatal(err)
		}
	}
	srcs := []string{"wiki-modme", "discord-bo3modtools"}
	for i := 0; i < pages; i++ {
		add(srcs[i%2], i, "a spawner, and other words around it", 0.80-float64(i)*0.01)
	}
	srcs = []string{"source-scripts", "defs-bo3"}
	for i := 0; i < code; i++ {
		add(srcs[i%2], i, "spawner spawner spawner", 0.95-float64(i)*0.01)
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
	q := make([]float32, 8)
	q[0] = 1
	return st, q
}

func ids(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.DocID
	}
	return out
}

// Code files that would fill one list keep to the last 3 of 10 results; the
// 7 pages above them are the pages' own best, in their own order.
func TestSearchListsPagesThenCode(t *testing.T) {
	st, q := splitDB(t, 12, 12)
	ctx := context.Background()
	one, err := st.SearchHybrid(ctx, "spawner", q, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(one[0].DocID, "source-scripts::") {
		t.Fatalf("the fixture should let code lead one list: %v", ids(one))
	}

	hits, err := st.Search(ctx, "spawner", q, 10)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := st.SearchHybrid(ctx, "spawner", q, 7, "discord-bo3modtools", "wiki-modme")
	code, _ := st.SearchHybrid(ctx, "spawner", q, 3, "defs-bo3", "source-scripts")
	if want := append(ids(pages), ids(code)...); !reflect.DeepEqual(ids(hits), want) {
		t.Fatalf("got %v\nwant the 7 best pages, then the 3 best code files: %v", ids(hits), want)
	}
	for i, h := range hits {
		if h.Code != (i >= 7) {
			t.Errorf("hit %d (%s): Code = %v", i+1, h.DocID, h.Code)
		}
	}
}

// A part with fewer hits than its slots leaves them to the other.
func TestSearchGivesUnfilledSlotsToTheOtherPart(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		pages, code, wantPages, wantCode int
	}{
		{pages: 2, code: 12, wantPages: 2, wantCode: 8},
		{pages: 12, code: 1, wantPages: 9, wantCode: 1},
		{pages: 3, code: 2, wantPages: 3, wantCode: 2},
	} {
		st, q := splitDB(t, c.pages, c.code)
		hits, err := st.Search(ctx, "spawner", q, 10)
		if err != nil {
			t.Fatal(err)
		}
		var p, k int
		for _, h := range hits {
			if h.Code {
				k++
			} else if k > 0 {
				t.Fatalf("%d pages + %d code: a page after the code: %v", c.pages, c.code, ids(hits))
			} else {
				p++
			}
		}
		if p != c.wantPages || k != c.wantCode {
			t.Errorf("%d pages + %d code: got %d pages + %d code, want %d + %d", c.pages, c.code, p, k, c.wantPages, c.wantCode)
		}
	}
}

// A source filter asks for one kind of answer: Search then ranks one list.
func TestSearchWithSourcesIsOneList(t *testing.T) {
	st, q := splitDB(t, 12, 12)
	ctx := context.Background()
	only := []string{"defs-bo3", "wiki-modme"}
	got, err := st.Search(ctx, "spawner", q, 10, only...)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := st.SearchHybrid(ctx, "spawner", q, 10, only...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want SearchHybrid's %v", ids(got), ids(want))
	}
}

// One pass over the vectors ranks each part as a filtered pass would, with
// the in-memory index and with the scan.
func TestVectorRankSplitMatchesFilteredRanks(t *testing.T) {
	ctx := context.Background()
	for _, indexed := range []bool{false, true} {
		st, q := splitDB(t, 12, 12)
		if indexed {
			st.EnableVectorIndex()
		}
		code := map[string]bool{"source-scripts": true, "defs-bo3": true}
		others, inCode, err := st.vectorRankSplit(ctx, q, 5, code)
		if err != nil {
			t.Fatal(err)
		}
		wantOthers, _ := st.vectorRank(ctx, q, 5, map[string]bool{"wiki-modme": true, "discord-bo3modtools": true})
		wantCode, _ := st.vectorRank(ctx, q, 5, code)
		if !reflect.DeepEqual(others, wantOthers) || !reflect.DeepEqual(inCode, wantCode) {
			t.Errorf("index %v: split %v | %v\nwant %v | %v", indexed, others, inCode, wantOthers, wantCode)
		}
	}
}

// A hit BM25 found shows the passage around the words it matched; a hit only
// the vector found shows its best chunk.
func TestHitsShowTheMatchedPassageElseTheChunk(t *testing.T) {
	st, q := splitDB(t, 4, 4)
	ctx := context.Background()
	for _, c := range []struct {
		query string
		want  func(Hit) string
	}{
		{"spawner", func(h Hit) string { return "spawner" }},
		{"zzznotaword", func(h Hit) string { return "chunk of " + h.DocID }},
	} {
		hits, err := st.SearchHybrid(ctx, c.query, q, 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 8 {
			t.Fatalf("%q: %d hits, want 8", c.query, len(hits))
		}
		for _, h := range hits {
			if !strings.Contains(h.Snippet, c.want(h)) {
				t.Errorf("%q: %s shows %q, want %q in it", c.query, h.DocID, h.Snippet, c.want(h))
			}
		}
	}
}
