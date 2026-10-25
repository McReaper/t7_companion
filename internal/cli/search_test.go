package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// kbDB writes a keyword-only corpus (no embeddings) and points T7KB_DB at it.
func kbDB(t *testing.T, docs map[string]string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kb.db")
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
	for id, body := range docs {
		src, _, _ := strings.Cut(id, "::")
		if _, err := db.Exec(`INSERT INTO documents(doc_id,source,title,body,reliability) VALUES(?,?,?,?,?)`, id, src, id, body, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO docs_fts(docs_fts) VALUES('rebuild')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("T7KB_DB", path)
}

// A keyword-only search of a question no one document holds every word of
// ranks those holding some, and says so; one that finds every word stays as
// it was, and a query of no word at all still finds nothing.
func TestSearchBM25AnswersAQuestionWithTheDocumentsHoldingSomeOfItsWords(t *testing.T) {
	kbDB(t, map[string]string{
		"wiki::light":  "Place an omni light in Radiant and set its radius.",
		"wiki::caulk":  "Caulk the faces nobody sees.",
		"forum::other": "Nothing about it here.",
	})
	run := func(args ...string) (string, string) {
		cmd := newRootCmd()
		var out, errs strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&errs)
		cmd.SetArgs(append([]string{"search", "--bm25"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%q: %v", args, err)
		}
		return out.String(), errs.String()
	}
	out, errs := run("placing", "a", "light", "entity", "in", "Radiant", "for", "zombies")
	if !strings.Contains(out, "wiki::light") || strings.Contains(out, "forum::other") || !strings.Contains(errs, "no document holds every word") {
		t.Errorf("a question:\n%s\nstderr: %s", out, errs)
	}
	if out, errs := run("omni", "light"); !strings.Contains(out, "wiki::light") || strings.Contains(out, "wiki::caulk") || errs != "" {
		t.Errorf("every word found:\n%s\nstderr: %s", out, errs)
	}
	if out, errs := run("--source", "forum", "omni", "light"); out != "" || !strings.Contains(errs, "no results") {
		t.Errorf("outside the filter:\n%s\nstderr: %s", out, errs)
	}
	if out, errs := run("zzz", "qqq"); out != "" || strings.TrimSpace(errs) != "no results" {
		t.Errorf("no word found:\n%s\nstderr: %s", out, errs)
	}
}
