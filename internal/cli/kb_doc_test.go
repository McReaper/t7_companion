package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/McReaper/t7_companion/internal/store"
)

var nextOffsetRE = regexp.MustCompile(`and offset (\d+);[^\]]*\]\n$`)

// readAll follows a document's pages the way an agent does and returns the
// body they add up to and how many pages it took.
func readAll(t *testing.T, d *store.Doc, maxChars int) (string, int) {
	t.Helper()
	var got strings.Builder
	offset, pages := 0, 0
	for {
		page, err := renderDoc(d, offset, maxChars)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if pages > len(d.Body)+1 { // every page moves on by at least one character
			t.Fatal("paging doesn't terminate")
		}
		m := nextOffsetRE.FindStringSubmatch(page)
		got.WriteString(pageBody(page))
		if m == nil {
			return got.String(), pages
		}
		next, _ := strconv.Atoi(m[1])
		if next <= offset {
			t.Fatalf("page at %d points back to %d", offset, next)
		}
		offset = next
	}
}

// pageBody is the slice of the document a page carries: after the header (the
// title line, then doc_id/source/url/continued lines, then a blank line) and
// before the truncation note.
func pageBody(page string) string {
	rest := page[strings.Index(page, "doc_id:"):]
	body := rest[strings.Index(rest, "\n\n")+2:]
	if i := strings.LastIndex(body, "\n\n[truncated:"); i >= 0 {
		return body[:i]
	}
	return strings.TrimSuffix(body, "\n")
}

func TestRenderDocPages(t *testing.T) {
	short := &store.Doc{DocID: "x::short", Title: "Short", Source: "x", Body: "one line\nand another"}
	page, err := renderDoc(short, 0, pageSize(0))
	if err != nil || strings.Contains(page, "truncated") || !strings.HasSuffix(page, "and another\n") {
		t.Fatalf("a document under the page size comes whole, with no note: %q %v", page, err)
	}

	var lines []string
	for i := 0; i < 4000; i++ {
		lines = append(lines, "line "+strconv.Itoa(i)+" — zombies spawn here ✓ «ok»")
	}
	long := &store.Doc{DocID: "x::long", Title: "Long", Source: "x", Body: strings.Join(lines, "\n")}
	for _, size := range []int{pageSize(0), 1000, 7, 1} {
		got, pages := readAll(t, long, size)
		if got != long.Body {
			t.Fatalf("size %d: the pages don't add up to the body (%d pages, %d of %d bytes)", size, pages, len(got), len(long.Body))
		}
		if size == pageSize(0) && pages < 2 {
			t.Fatalf("a %d-byte body needs several default pages", len(long.Body))
		}
	}

	first, _ := renderDoc(long, 0, 1000)
	if !utf8.ValidString(first) {
		t.Fatal("a page never splits a UTF-8 character")
	}
	if body := pageBody(first); !strings.HasSuffix(body, "»\n") {
		t.Fatalf("a page ends at a line break when one is near the limit: …%q", body[len(body)-20:])
	}
	if !strings.Contains(first, `call get with doc_id "x::long" and offset `) {
		t.Fatalf("the note names the doc and the offset: %s", first[len(first)-160:])
	}

	cont, _ := renderDoc(long, 1000, 1000)
	if !strings.Contains(cont, "(continued from offset ") {
		t.Fatal("a later page says where it starts")
	}
	if _, err := renderDoc(long, len(long.Body)+5, 1000); err == nil {
		t.Fatal("an offset past the end is an error")
	}
	if whole, _ := renderDoc(long, 0, 0); strings.Contains(whole, "truncated") {
		t.Fatal("maxChars 0 renders the whole body (CLI --all, browse)")
	}
	if pageSize(0) != defaultPageChars || pageSize(1e9) != maxPageChars || pageSize(500) != 500 {
		t.Fatal("pageSize: default, ceiling, and the requested size in between")
	}
}

// On the real corpus (opt-in, T7KB_BENCH_DB): the largest documents — whole
// GDTs and scripts, up to ~98 MB — page back to their exact body.
func TestPagingOnTheRealCorpus(t *testing.T) {
	db := os.Getenv("T7KB_BENCH_DB")
	if db == "" {
		t.Skip("set T7KB_BENCH_DB to a t7kb.db")
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	raw, err := sql.Open("sqlite", "file:"+db+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	rows, err := raw.Query(`SELECT doc_id FROM documents ORDER BY length(body) DESC LIMIT 8`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	for _, id := range ids {
		d, err := st.Get(context.Background(), id)
		if err != nil || d == nil {
			t.Fatalf("%s: %v", id, err)
		}
		got, pages := readAll(t, d, pageSize(0))
		if got != d.Body {
			t.Fatalf("%s: %d pages give %d of %d bytes", id, pages, len(got), len(d.Body))
		}
		t.Logf("%s: %d bytes in %d pages", id, len(d.Body), pages)
	}
}

func TestGetFindJumpsToThePassage(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "filler line %d about nothing in particular — ✓\n", i)
	}
	b.WriteString("the hintstring uses ^3 for yellow and ^7 to reset\n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "more filler %d\n", i)
	}
	b.WriteString("a second HINTSTRING note\n")
	b.WriteString(strings.Repeat("x", 5000) + " needle far into a very long minified line\n")
	d := &store.Doc{DocID: "x::thread", Title: "Thread", Source: "x", Body: b.String()}
	hint := strings.Index(d.Body, "the hintstring")

	page, err := docPage(d, 0, "HintString uses", pageSize(0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pageBody(page), "the hintstring uses ^3") {
		t.Fatalf("find starts the page at the matching line, case-insensitively: %.80q", pageBody(page))
	}
	if !strings.Contains(page, fmt.Sprintf("(continued from offset %d)", hint)) {
		t.Fatal("the page says where it starts")
	}

	next, err := docPage(d, hint+len("the hintstring"), "hintstring", pageSize(0))
	if err != nil || !strings.HasPrefix(pageBody(next), "a second HINTSTRING note") {
		t.Fatalf("find with offset finds the next occurrence: %v %.60q", err, pageBody(next))
	}

	far, err := docPage(d, 0, "needle", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if body := pageBody(far); !strings.Contains(body, "needle") || strings.Count(body, "x") > findContext+1 {
		t.Fatalf("on a very long line the page starts just before the match: %d x's", strings.Count(body, "x"))
	}

	if _, err := docPage(d, 0, "not in there", 1000); err == nil || !strings.Contains(err.Error(), "is not in the document") {
		t.Fatalf("a find that matches nothing is an error, not a silent first page: %v", err)
	}
	if _, err := docPage(d, len(d.Body)+1, "x", 1000); err == nil {
		t.Fatal("find from an offset past the end is an error")
	}
	if i := indexFoldASCII("Ünïcode ÉCOLE then école", "école"); i != strings.Index("Ünïcode ÉCOLE then école", "école") {
		t.Fatalf("only ASCII is folded, so byte offsets stay exact: %d", i)
	}
}
