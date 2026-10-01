package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/McReaper/t7_companion/internal/embed"
	"github.com/McReaper/t7_companion/internal/store"
)

// TestRetrievalQuality scores search against judged queries
// (testdata/retrieval_eval.json): for each, the documents that should come up —
// "key" ones (graded 2) and "relevant" ones (graded 1). Rerun it before and
// after changing ranking: a change that helps one query can hurt ten. Opt-in:
//
//	T7KB_BENCH_DB=<t7kb.db> HF_HOME=<models> go test ./internal/cli -run TestRetrievalQuality -v
//
// Metrics, averaged over the queries:
//   - key@5: a key document is in the top 5
//   - recall@10: the share of key + relevant documents in the top 10
//   - MRR: 1 / the rank of the first key or relevant document
//   - nDCG@10: graded ranking quality (key 2, relevant 1), 1 = ideal order

type evalQuery struct {
	Query    string   `json:"query"`
	Key      []string `json:"key"`
	Relevant []string `json:"relevant"`
}

type evalScore struct{ key5, recall10, mrr, ndcg10 float64 }

func TestRetrievalQuality(t *testing.T) {
	db := os.Getenv("T7KB_BENCH_DB")
	if db == "" {
		t.Skip("set T7KB_BENCH_DB to a t7kb.db (and HF_HOME to the model cache)")
	}
	var set struct {
		Queries []evalQuery `json:"queries"`
	}
	b, err := os.ReadFile("testdata/retrieval_eval.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &set); err != nil {
		t.Fatal(err)
	}
	checkJudgedDocsExist(t, db, set.Queries)

	emb, err := embed.New()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	var sum evalScore
	var report strings.Builder
	fmt.Fprintf(&report, "\n%-46s key@5 rec@10  MRR  nDCG@10  top-10 sources\n", "query")
	for _, q := range set.Queries {
		v, err := emb.Embed(q.Query)
		if err != nil {
			t.Fatal(err)
		}
		hits, err := st.SearchHybrid(context.Background(), q.Query, v, 10)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(hits))
		for i, h := range hits {
			ids[i] = h.DocID
		}
		s := scoreRanking(ids, q)
		sum.key5 += s.key5
		sum.recall10 += s.recall10
		sum.mrr += s.mrr
		sum.ndcg10 += s.ndcg10
		fmt.Fprintf(&report, "%-46.46s %5.0f %6.2f %4.2f %8.2f  %s\n", q.Query, s.key5, s.recall10, s.mrr, s.ndcg10, sourceMix(ids))
	}
	n := float64(len(set.Queries))
	fmt.Fprintf(&report, "%-46s %5.2f %6.2f %4.2f %8.2f\n", fmt.Sprintf("MEAN over %d queries", len(set.Queries)), sum.key5/n, sum.recall10/n, sum.mrr/n, sum.ndcg10/n)
	t.Log(report.String())
}

// scoreRanking scores one ranked list of doc_ids against a judged query.
func scoreRanking(ids []string, q evalQuery) evalScore {
	grade := map[string]float64{}
	for _, d := range q.Relevant {
		grade[d] = 1
	}
	for _, d := range q.Key {
		grade[d] = 2
	}
	var s evalScore
	found, dcg := 0, 0.0
	for i, d := range ids {
		g := grade[d]
		if g == 0 {
			continue
		}
		if i < 10 {
			found++
			dcg += (math.Pow(2, g) - 1) / math.Log2(float64(i)+2)
		}
		if s.mrr == 0 {
			s.mrr = 1 / float64(i+1)
		}
		if g == 2 && i < 5 {
			s.key5 = 1
		}
	}
	s.recall10 = float64(found) / float64(len(grade))
	// ideal: every judged document, key ones first
	var ideal []float64
	for range q.Key {
		ideal = append(ideal, 2)
	}
	for range q.Relevant {
		ideal = append(ideal, 1)
	}
	idcg := 0.0
	for i, g := range ideal {
		if i >= 10 {
			break
		}
		idcg += (math.Pow(2, g) - 1) / math.Log2(float64(i)+2)
	}
	if idcg > 0 {
		s.ndcg10 = dcg / idcg
	}
	return s
}

// sourceMix summarises where the top results come from: "discord 7, api 2, ugx 1".
func sourceMix(ids []string) string {
	count := map[string]int{}
	var order []string
	for _, id := range ids {
		src, _, _ := strings.Cut(id, "::")
		src = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(src, "forums-"), "wiki-"), "-bo3modtools")
		if count[src] == 0 {
			order = append(order, src)
		}
		count[src]++
	}
	parts := make([]string, len(order))
	for i, s := range order {
		parts[i] = fmt.Sprintf("%s %d", s, count[s])
	}
	return strings.Join(parts, ", ")
}

// checkJudgedDocsExist fails on a judged doc_id the db doesn't have: a typo, or
// a rebuilt corpus that renamed documents — either would silently score zero.
func checkJudgedDocsExist(t *testing.T, db string, qs []evalQuery) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+db+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	for _, q := range qs {
		for _, id := range append(append([]string{}, q.Key...), q.Relevant...) {
			var n int
			if err := raw.QueryRow(`SELECT count(*) FROM documents WHERE doc_id = ?`, id).Scan(&n); err != nil || n == 0 {
				t.Errorf("%q: judged doc %q is not in the db", q.Query, id)
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func TestScoreRanking(t *testing.T) {
	q := evalQuery{Key: []string{"k"}, Relevant: []string{"r1", "r2"}}
	if s := scoreRanking([]string{"k", "r1", "r2"}, q); s.key5 != 1 || s.recall10 != 1 || s.mrr != 1 || math.Abs(s.ndcg10-1) > 1e-9 {
		t.Fatalf("the ideal ranking scores 1 everywhere: %+v", s)
	}
	if s := scoreRanking([]string{"x", "x", "x", "x", "x", "k"}, q); s.key5 != 0 || s.mrr != 1.0/6 || s.recall10 != 1.0/3 {
		t.Fatalf("a key at rank 6 misses key@5: %+v", s)
	}
	if s := scoreRanking([]string{"x"}, q); s != (evalScore{}) {
		t.Fatalf("nothing found scores 0: %+v", s)
	}
}
