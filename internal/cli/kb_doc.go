package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/McReaper/t7_companion/internal/store"
)

// A get answer lands in the agent's context, and the corpus has a long tail:
// bodies have a median of 1.2k characters but a p99 of 275k and a maximum of
// 98 MB (source-workspace indexes whole GDTs and scripts), so one uncapped get
// could cost ~40k tokens (docs/benchmarks.md). The body is served a page at a
// time: the default page holds 90%+ of documents whole, and a longer one ends
// with the offset to read on from.
const (
	defaultPageChars = 16000 // ~4k tokens of prose, ~5k of a GDT (paths and numbers tokenize denser)
	maxPageChars     = 64000
)

// renderDoc renders one page of a document: the header, then the body from
// offset (a byte offset into the body) for up to maxChars bytes, cut at a line
// or word break and never inside a UTF-8 character. maxChars <= 0 renders the
// whole body.
func renderDoc(d *store.Doc, offset, maxChars int) (string, error) {
	body := d.Body
	if offset < 0 || (offset > 0 && offset >= len(body)) {
		return "", fmt.Errorf("offset %d is outside the document (%d characters)", offset, len(body))
	}
	for offset < len(body) && !utf8.RuneStart(body[offset]) {
		offset++
	}
	end := len(body)
	if maxChars > 0 && offset+maxChars < len(body) {
		end = pageEnd(body, offset, offset+maxChars)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", d.Title)
	fmt.Fprintf(&b, "doc_id: %s\nsource: %s  ·  reliability: %.2f\n", d.DocID, d.Source, d.Reliability)
	if d.URL != "" {
		fmt.Fprintf(&b, "url: %s\n", d.URL)
	}
	if offset > 0 {
		fmt.Fprintf(&b, "(continued from offset %d)\n", offset)
	}
	fmt.Fprintf(&b, "\n%s\n", body[offset:end])
	if end < len(body) {
		fmt.Fprintf(&b, "\n[truncated: characters %d–%d of %d shown. For the next part, call get with doc_id %q and offset %d; to jump to a passage, pass find with a word or phrase from it.]\n",
			offset, end, len(body), d.DocID, end)
	}
	return b.String(), nil
}

// pageEnd picks where a page ending near limit should stop: the last line break
// in its final quarter, else the last space there, else limit itself — moved
// back to the start of a UTF-8 character.
func pageEnd(body string, offset, limit int) int {
	floor := offset + (limit-offset)*3/4
	if i := strings.LastIndexByte(body[floor:limit], '\n'); i >= 0 {
		return floor + i + 1
	}
	if i := strings.LastIndexByte(body[floor:limit], ' '); i >= 0 {
		return floor + i + 1
	}
	for limit > offset && !utf8.RuneStart(body[limit]) {
		limit--
	}
	if limit == offset { // a page always moves on by at least one character
		_, size := utf8.DecodeRuneInString(body[offset:])
		limit = offset + size
	}
	return limit
}

// findContext is how far before a match a page may start when the match's line
// is longer than that (a minified file): the page starts at the line, else here.
const findContext = 300

// findOffset returns where a page showing the first occurrence of text at or
// after from should start: the start of the match's line, or findContext
// before the match on a very long line. The search folds ASCII case only, so
// byte offsets stay exact (full Unicode folding can change a string's length).
func findOffset(body, text string, from int) (int, error) {
	if text == "" {
		return from, nil
	}
	if from < 0 || from > len(body) {
		return 0, fmt.Errorf("offset %d is outside the document (%d characters)", from, len(body))
	}
	i := indexFoldASCII(body[from:], text)
	if i < 0 {
		where := "the document"
		if from > 0 {
			where = fmt.Sprintf("the document after offset %d", from)
		}
		return 0, fmt.Errorf("%q is not in %s", text, where)
	}
	match := from + i
	start := strings.LastIndexByte(body[:match], '\n') + 1
	if match-start > findContext {
		start = match - findContext
		for start < match && !utf8.RuneStart(body[start]) {
			start++
		}
	}
	return start, nil
}

// indexFoldASCII is strings.Index with ASCII letters compared case-insensitively.
func indexFoldASCII(s, sub string) int {
	n := len(sub)
	if n > len(s) {
		return -1
	}
	first := lowerASCII(sub[0])
	for i := 0; i+n <= len(s); i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		j := 1
		for j < n && lowerASCII(s[i+j]) == lowerASCII(sub[j]) {
			j++
		}
		if j == n {
			return i
		}
	}
	return -1
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// pageSize applies the default and the ceiling to a requested page size.
func pageSize(requested int) int {
	switch {
	case requested <= 0:
		return defaultPageChars
	case requested > maxPageChars:
		return maxPageChars
	}
	return requested
}

// docPage is a get answer: the page at offset or, with find, the page holding
// find's first occurrence at or after offset.
func docPage(d *store.Doc, offset int, find string, maxChars int) (string, error) {
	if find != "" {
		var err error
		if offset, err = findOffset(d.Body, find, offset); err != nil {
			return "", err
		}
	}
	return renderDoc(d, offset, maxChars)
}
