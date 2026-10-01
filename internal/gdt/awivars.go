package gdt

import (
	"regexp"
	"strings"
)

// Combo options are often a string variable rather than a literal:
//
//	string TileModes = "tile both* | tile horizontal | tile vertical | no tile";
//	string SurfaceTypeEntries = """ <error> | <none> | asphalt | … """;
//	string compressionMethodStr = compMethodDefault; if (…) compressionMethodStr = compMethodSRGB;
//	AddEntry_Combo( "tileColor", TileModes )
//
// awiVars reads every assignment in a deffile and evaluates the ones built only
// from literals and other such variables. A variable assigned in several places
// (if/else branches) can hold any of those values; one assigned from anything
// else — a function call, an array element, a += in a loop — is not static, and a
// combo built from it keeps no options (so it isn't checked, as before).

// maxValues caps the values a variable can hold (branches multiply under +).
const maxValues = 64

var assignRE = regexp.MustCompile(`(?:^|[^\w.!<>=+\-*/])([A-Za-z_]\w*)\s*(\+?=)[^=]`)

type awiVars struct {
	rhs     map[string][]string // variable -> right-hand sides, in source order
	dynamic map[string]bool     // assigned from something not static
	memo    map[string][]string
	busy    map[string]bool
}

func parseAwiVars(code string) *awiVars {
	v := &awiVars{rhs: map[string][]string{}, dynamic: map[string]bool{}, memo: map[string][]string{}, busy: map[string]bool{}}
	for _, m := range assignRE.FindAllStringSubmatchIndex(code, -1) {
		if inString(code, m[2]) {
			continue
		}
		name, op := code[m[2]:m[3]], code[m[4]:m[5]]
		end := statementEnd(code, m[5])
		if end < 0 {
			v.dynamic[name] = true
			continue
		}
		if op == "+=" {
			v.dynamic[name] = true
			continue
		}
		v.rhs[name] = append(v.rhs[name], code[m[5]:end])
	}
	return v
}

// values returns every string the variable can hold, or false if it isn't static.
func (v *awiVars) values(name string) ([]string, bool) {
	if v.dynamic[name] || v.busy[name] { // busy: x = x + "…" refers to itself
		return nil, false
	}
	if out, ok := v.memo[name]; ok {
		return out, out != nil
	}
	rhs, ok := v.rhs[name]
	if !ok {
		return nil, false // a parameter, or declared without a value
	}
	v.busy[name] = true
	defer delete(v.busy, name)
	var out []string
	for _, r := range rhs {
		vals, ok := v.eval(r)
		if !ok {
			v.memo[name] = nil
			return nil, false
		}
		for _, s := range vals {
			if !contains(out, s) {
				out = append(out, s)
			}
		}
	}
	if len(out) > maxValues {
		out = nil
	}
	v.memo[name] = out
	return out, out != nil
}

// eval evaluates an expression of string literals (adjacent ones concatenate,
// as in AngelScript), """heredocs""", static variables and +.
func (v *awiVars) eval(expr string) ([]string, bool) {
	acc := []string{""}
	s, terms := strings.TrimSpace(expr), 0
	for s != "" {
		if s[0] == '+' {
			s = strings.TrimSpace(s[1:])
			continue
		}
		vals, rest, ok := v.term(s)
		if !ok {
			return nil, false
		}
		if acc = product(acc, vals); acc == nil {
			return nil, false
		}
		s = strings.TrimSpace(rest)
		terms++
	}
	return acc, terms > 0
}

// term reads the literal, heredoc or variable at the start of s: its values and
// what follows it.
func (v *awiVars) term(s string) ([]string, string, bool) {
	switch {
	case strings.HasPrefix(s, `"""`):
		end := strings.Index(s[3:], `"""`)
		if end < 0 {
			return nil, "", false
		}
		return []string{s[3 : 3+end]}, s[3+end+3:], true
	case s[0] == '"':
		lit, n, ok := stringLiteral(s)
		return []string{lit}, s[n:], ok
	}
	id := identRE.FindString(s)
	if id == "" {
		return nil, "", false
	}
	rest := strings.TrimSpace(s[len(id):])
	if rest != "" && rest[0] != '+' && rest[0] != '"' { // a call, an index, an operator
		return nil, "", false
	}
	vals, ok := v.values(id)
	return vals, rest, ok
}

// stringLiteral reads the "…" at the start of s (escapes resolved) and its length.
func stringLiteral(s string) (string, int, bool) {
	var b strings.Builder
	i := 1
	for ; i < len(s) && s[i] != '"'; i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	if i >= len(s) {
		return "", 0, false
	}
	return b.String(), i + 1, true
}

// product concatenates every a with every b; nil past maxValues.
func product(as, bs []string) []string {
	var out []string
	for _, a := range as {
		for _, b := range bs {
			out = append(out, a+b)
		}
	}
	if len(out) > maxValues {
		return nil
	}
	return out
}

var identRE = regexp.MustCompile(`^[A-Za-z_]\w*`)

// statementEnd finds the ";" ending a statement that starts at i, skipping strings
// and heredocs; -1 if a brace closes first (not a plain assignment).
func statementEnd(code string, i int) int {
	depth := 0
	for ; i < len(code); i++ {
		switch code[i] {
		case '"':
			if strings.HasPrefix(code[i:], `"""`) {
				end := strings.Index(code[i+3:], `"""`)
				if end < 0 {
					return -1
				}
				i += 3 + end + 2
				continue
			}
			for i++; i < len(code) && code[i] != '"'; i++ {
				if code[i] == '\\' {
					i++
				}
			}
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '{', '}':
			return -1
		case ';':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// inString reports whether position p is inside a string literal on its line —
// enough to skip "a = b" text inside tooltips.
func inString(code string, p int) bool {
	start := strings.LastIndexByte(code[:p], '\n') + 1
	return strings.Count(code[start:p], `"`)%2 == 1
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
