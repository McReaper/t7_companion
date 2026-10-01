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
		fmt.Fprintf(&b, "\n[truncated: characters %d–%d of %d shown. For the next part, call get with doc_id %q and offset %d.]\n",
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
