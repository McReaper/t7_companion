package shader

import (
	"strings"
	"testing"
)

// FuzzParseName holds ParseName to what it promises for any string: a name
// it accepts has a source, a stage the cache uses and a hash, and writes back
// as itself.
func FuzzParseName(f *testing.F) {
	for _, s := range []string{"techsetdef_unlit.hlsl_ps_main_CJK6WNS46DJOAEU6RKXSYFZY5D", "fx_count.hlsl_cs_main_D",
		"x.hlsl_ps_H", "_ps_main_H", "a_b_c_main_d", "", "_", "__main_"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n, ok := ParseName(s)
		if !ok {
			if n != (Name{}) {
				t.Fatalf("%q: rejected with %+v", s, n)
			}
			return
		}
		if n.Source == "" || n.Hash == "" || !stages[n.Stage] || strings.Contains(n.Hash, "_") || n.File() != s {
			t.Fatalf("%q: %+v", s, n)
		}
	})
}
