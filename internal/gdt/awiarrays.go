package gdt

import (
	"regexp"
	"strconv"
	"strings"
)

// A deffile may build field names from a constant array of strings walked with
// a counter — `gibPrefix + GIB_KEYS[keyIndex++]` after `uint keyIndex = 0;`
// (gibcharacterdef.awi) — which leaves the static parse no literal to read.
// expandArrays replaces each such element, in code order, by its literal, so
// the name reads as `gibPrefix + "gibmodel"`.

var (
	constArrayRE = regexp.MustCompile(`(?s)const\s+array\s*<\s*string\s*>\s+(\w+)\s*=\s*\{(.*?)\}\s*;`)
	stringLitRE  = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// expandArrays inlines the elements of constant string arrays indexed by a
// literal or by a counter set by `int|uint name = N;` (`name++` advancing it).
func expandArrays(code string) string {
	arrays := map[string][]string{}
	var names []string
	for _, m := range constArrayRE.FindAllStringSubmatch(code, -1) {
		var elems []string
		for _, lit := range stringLitRE.FindAllStringSubmatch(m[2], -1) {
			elems = append(elems, lit[1])
		}
		arrays[m[1]] = elems
		names = append(names, regexp.QuoteMeta(m[1]))
	}
	if len(arrays) == 0 {
		return code
	}
	re := regexp.MustCompile(`\b(?:int|uint)\s+(\w+)\s*=\s*(\d+)\s*;|\b(` + strings.Join(names, "|") + `)\[\s*(\w+)\s*(\+\+)?\s*\]`)
	counters := map[string]int{}
	return re.ReplaceAllStringFunc(code, func(s string) string {
		m := re.FindStringSubmatch(s)
		if m[1] != "" { // a counter (re)set
			counters[m[1]], _ = strconv.Atoi(m[2])
			return s
		}
		i, err := strconv.Atoi(m[4])
		if err != nil {
			c, ok := counters[m[4]]
			if !ok {
				return s
			}
			i = c
			if m[5] != "" {
				counters[m[4]] = c + 1
			}
		}
		if elems := arrays[m[3]]; i >= 0 && i < len(elems) {
			return `"` + elems[i] + `"`
		}
		return s
	})
}
