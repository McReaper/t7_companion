package level

import (
	"fmt"
	"sort"
	"strings"
)

// minID is a short id's length: a GUID's first group, which Radiant's
// time-based GUIDs change at every new one.
const minID = 8

// IDs gives each GUID of a map a short id: its first 8 hex digits, more when
// another GUID starts the same, so an id stays the same from one call to the
// next while the map grows.
type IDs struct {
	sorted []string          // the GUIDs' hex digits, sorted
	guid   map[string]string // hex -> the GUID as the map writes it
	short  map[string]string // hex -> short id
}

// NewIDs indexes a map's GUIDs (entities' and brushes'); a repeated one is
// indexed once.
func NewIDs(guids []string) *IDs {
	x := &IDs{guid: map[string]string{}, short: map[string]string{}}
	for _, g := range guids {
		h := hexOf(g)
		if h == "" {
			continue
		}
		if _, seen := x.guid[h]; !seen {
			x.guid[h] = g
			x.sorted = append(x.sorted, h)
		}
	}
	sort.Strings(x.sorted)
	for i, h := range x.sorted {
		n := minID
		for _, j := range []int{i - 1, i + 1} {
			if j >= 0 && j < len(x.sorted) {
				n = max(n, commonPrefix(h, x.sorted[j])+1)
			}
		}
		x.short[h] = h[:min(n, len(h))]
	}
	return x
}

// Short is a GUID's short id; a GUID the map doesn't have gets its hex digits.
func (x *IDs) Short(guid string) string {
	h := hexOf(guid)
	if s, ok := x.short[h]; ok {
		return s
	}
	return h
}

// Resolve finds the GUID an id names: a short id, a longer start of the
// GUID, or the GUID itself, in any case and with or without its braces and
// dashes.
func (x *IDs) Resolve(id string) (string, error) {
	h := hexOf(id)
	if h == "" {
		return "", fmt.Errorf("%q is no id: an id is a GUID's first hex digits (8 or more)", id)
	}
	i := sort.SearchStrings(x.sorted, h)
	var matches []string
	for ; i < len(x.sorted) && strings.HasPrefix(x.sorted[i], h); i++ {
		matches = append(matches, x.sorted[i])
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no entity or brush %s in the map", id)
	case 1:
		return x.guid[matches[0]], nil
	}
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = x.short[m]
	}
	return "", fmt.Errorf("%s names %d entities or brushes (%s): give more of it", id, len(matches), strings.Join(ids, ", "))
}

// hexOf is a GUID's hex digits, lower case, "" when anything else is in it.
func hexOf(s string) string {
	s = strings.ToLower(strings.Trim(strings.TrimSpace(s), "{}"))
	s = strings.ReplaceAll(s, "-", "")
	if s == "" || strings.Trim(s, "0123456789abcdef") != "" {
		return ""
	}
	return s
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
